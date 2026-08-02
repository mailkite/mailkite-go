// Package mailkite is the official MailKite SDK for Go.
//
// Shape shared by every MailKite SDK: one low-level Request plus one thin method
// per API endpoint. Bodies and responses are plain interface{} values decoded
// with encoding/json, so there are no external dependencies. (Go exports methods
// in PascalCase; the method set is otherwise identical to the other SDKs.)
//
//	mk := mailkite.New(os.Getenv("MAILKITE_API_KEY"))
//	res, err := mk.Send(mailkite.Message{
//		From: "hello@app.mailkite.dev", To: "ada@example.com",
//		Subject: "Hi", Text: "It works.",
//	})
package mailkite

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API base URL.
const DefaultBaseURL = "https://api.mailkite.dev"

// DefaultToleranceMs rejects webhook events older than this (ms) to block
// replays. Pass 0 to VerifyWebhookWithTolerance to disable the freshness check.
const DefaultToleranceMs int64 = 5 * 60 * 1000

// Error is returned for any non-2xx response.
type Error struct {
	Status  int
	Message string
	Body    interface{}
}

func (e *Error) Error() string { return e.Message }

// generated-from: sdks/spec/schemas/send-request.json#attachments.items — edit the schema, then `node sdks/gen/types-codegen.mjs`.
// Attachment — One attachment on a send: a filename plus the file as a URL (preferred) or inline
// base64.
type Attachment struct {
	Filename string `json:"filename"`
	// Fetch the file from this URL at send time and attach it. Any URL works; an uploadAttachment()
	// URL is the secure, recommended choice. Prefer this over `content` for large files.
	URL string `json:"url,omitempty"`
	// Inline file bytes as base64. Simple for tiny files, but re-uploaded on every send — use `url`
	// (see uploadAttachment) for anything large.
	Content string `json:"content,omitempty"`
	// MIME type; defaults to application/octet-stream.
	ContentType string `json:"contentType,omitempty"`
}

// generated-from: sdks/spec/schemas/send-request.json — edit the schema, then `node sdks/gen/types-codegen.mjs`.
// Message — Body of Send(). (You may also pass any map/struct to the low-level Request.)
type Message struct {
	// An address on a verified domain.
	From string `json:"from"`
	// One recipient or a list.
	To interface{} `json:"to"`
	// Required unless supplied by a template.
	Subject string `json:"subject,omitempty"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
	// Send using a saved template — a user template (tpl_…) or a base template (base_…). Its
	// subject/html/text seed the message; explicit subject/html/text here override them.
	TemplateID string `json:"templateId,omitempty"`
	// Values substituted into the template's {{merge_tags}} (e.g. {"name":"Ann"} fills {{name}}).
	// HTML values are auto-escaped.
	TemplateData map[string]any `json:"templateData,omitempty"`
	CC           interface{}    `json:"cc,omitempty"`
	BCC          interface{}    `json:"bcc,omitempty"`
	ReplyTo      string         `json:"replyTo,omitempty"`
	InReplyTo    string         `json:"inReplyTo,omitempty"`
	// Extra raw MIME headers, applied after threading headers (caller wins). Use for what the
	// structured fields can't express — e.g. `List-Unsubscribe`, a dedup/idempotency key
	// (`X-Entity-Ref-ID`), or a tag header (`X-Tag`). Carried on both immediate and scheduled sends.
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	// Send later: ISO 8601, simple relative natural language ("in 2 hours"), or a ms-epoch. A future
	// time parks the message with the scheduler — the response carries an ssnd_… id and status
	// "scheduled", cancelable via DELETE /v1/scheduled/{id}. Omitted or past sends now.
	ScheduledAt string `json:"scheduledAt,omitempty"`
	// Open-tracking override for this send (HTML only). Omitted → the from-domain's default applies.
	TrackOpens bool `json:"trackOpens,omitempty"`
	// Click-tracking override for this send (HTML only): http(s) links are rewritten to a signed
	// redirect that records the click, then 302s to the destination. Omitted → the from-domain's
	// default applies.
	TrackClicks bool `json:"trackClicks,omitempty"`
}

// generated-from: sdks/spec/schemas/batch-send-request.json#recipients.items — edit the schema, then `node sdks/gen/types-codegen.mjs`.
// BatchRecipient — One batch recipient: exactly one address, plus optional merge values / headers
// that win over the shared ones.
type BatchRecipient struct {
	// Exactly one recipient address (bare or `Name <addr>`). This message is sent to this address
	// only.
	To string `json:"to"`
	// Per-recipient merge values, merged over the shared templateData (this wins on conflicts). Fills
	// {{merge_tags}} in the subject/html/text for this recipient's message.
	TemplateData map[string]any `json:"templateData,omitempty"`
	// Per-recipient extra raw MIME headers, merged over the shared headers (this wins on conflicts).
	Headers map[string]string `json:"headers,omitempty"`
}

// generated-from: sdks/spec/schemas/batch-send-request.json — edit the schema, then `node sdks/gen/types-codegen.mjs`.
// BatchMessage — Body of SendBatch(). (You may also pass any map/struct to the low-level Request.)
type BatchMessage struct {
	// An address on a verified domain. Shared by every message in the batch.
	From string `json:"from"`
	// One entry per message. Order is preserved in the response's results[].
	Recipients []BatchRecipient `json:"recipients"`
	// Required unless supplied by a template. May contain {{merge_tags}}, filled per recipient.
	Subject string `json:"subject,omitempty"`
	// May contain {{merge_tags}}, filled per recipient.
	HTML string `json:"html,omitempty"`
	// May contain {{merge_tags}}, filled per recipient.
	Text string `json:"text,omitempty"`
	// Send using a saved template — a user template (tpl_…) or a base template (base_…). Its
	// subject/html/text seed every message; explicit subject/html/text here override them.
	TemplateID string `json:"templateId,omitempty"`
	// Shared default merge values for every recipient; a recipient's own templateData overrides these
	// key-by-key. HTML values are auto-escaped.
	TemplateData map[string]any `json:"templateData,omitempty"`
	// Shared extra raw MIME headers for every message, applied after threading headers (caller wins);
	// a recipient's own headers override these key-by-key.
	Headers map[string]string `json:"headers,omitempty"`
	ReplyTo string            `json:"replyTo,omitempty"`
	// Thread every message under this Message-ID.
	InReplyTo string `json:"inReplyTo,omitempty"`
	// Attached to every message in the batch. Same shape as send()'s attachments.
	Attachments []Attachment `json:"attachments,omitempty"`
	// Send later: ISO 8601, simple relative natural language ("in 2 hours"), or a ms-epoch. A future
	// time parks every message with the scheduler (each gets its own ssnd_… id, individually
	// cancelable); omitted or past sends now.
	ScheduledAt string `json:"scheduledAt,omitempty"`
	// Open-tracking override for every message in the batch (HTML only). Omitted → the from-domain's
	// default applies.
	TrackOpens bool `json:"trackOpens,omitempty"`
	// Click-tracking override for every message in the batch (HTML only): http(s) links are rewritten
	// to a signed redirect that records the click, then 302s to the destination. Omitted → the
	// from-domain's default applies.
	TrackClicks bool `json:"trackClicks,omitempty"`
}

// Client talks to the MailKite API.
type Client struct {
	// APIKey is a static Bearer credential — an API key (mk_live_…) OR an OAuth access token.
	APIKey string
	// TokenProvider, if set, is called before each request to fetch a fresh Bearer token; it takes
	// precedence over APIKey. Use it with short-lived OAuth access tokens.
	TokenProvider func() (string, error)
	BaseURL       string
	HTTP          *http.Client
}

// New returns a client for the production API. apiKey is a Bearer credential — an API key
// (mk_live_…) or an OAuth access token.
func New(apiKey string) *Client {
	return &Client{APIKey: apiKey, BaseURL: DefaultBaseURL, HTTP: http.DefaultClient}
}

// NewWithBaseURL returns a client pointed at a custom base URL.
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{APIKey: apiKey, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: http.DefaultClient}
}

// NewWithToken returns a client that calls getToken before each request to obtain a fresh Bearer
// token — use it with short-lived OAuth access tokens so the SDK always sends a valid one.
func NewWithToken(getToken func() (string, error)) *Client {
	return &Client{TokenProvider: getToken, BaseURL: DefaultBaseURL, HTTP: http.DefaultClient}
}

// token resolves the Bearer token for one request: from TokenProvider if set, else APIKey.
func (c *Client) token() (string, error) {
	if c.TokenProvider != nil {
		return c.TokenProvider()
	}
	return c.APIKey, nil
}

// Request is the low-level call. Every method below is a one-liner on top of it.
func (c *Client) Request(method, path string, body interface{}) (interface{}, error) {
	var reader io.Reader
	hasBody := body != nil
	if hasBody {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	tok, err := c.token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)
	var data interface{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", res.StatusCode)
		if m, ok := data.(map[string]interface{}); ok {
			if e, ok := m["error"].(string); ok {
				msg = e
			}
		}
		return nil, &Error{Status: res.StatusCode, Message: msg, Body: data}
	}
	return data, nil
}

// --- Sending ----------------------------------------------------------------

func (c *Client) Send(message interface{}) (interface{}, error) {
	return c.Request("POST", "/v1/send", message)
}

// SendBatch sends one personalized message per recipient (up to 50) in a single
// call. Pass a BatchMessage (or any map/struct shaped like one); each
// recipients[] entry gets its own message and id, and the response reports each
// recipient's outcome in order.
func (c *Client) SendBatch(batch interface{}) (interface{}, error) {
	return c.Request("POST", "/v1/send/batch", batch)
}

// AttachmentUpload describes a file to upload via UploadAttachment. Provide the
// file ONE of four ways (checked in this order): Url (MailKite fetches &
// re-hosts), Bytes (raw binary upload), Path (read off disk, raw binary upload),
// or Content (base64). RetentionDays optionally overrides how long the hosted
// file lives. Path and Bytes are never serialized to JSON.
type AttachmentUpload struct {
	Url           string `json:"url,omitempty"`
	Content       string `json:"content,omitempty"`
	Filename      string `json:"filename,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
	RetentionDays int    `json:"retentionDays,omitempty"`
	Path          string `json:"-"`
	Bytes         []byte `json:"-"`
}

// UploadAttachment uploads a file and returns a secure, time-limited URL to
// reference as a Send() attachment ({ filename, url }) or link inline — instead
// of base64-inlining large files on every send. The file may be provided as a
// URL (re-hosted by MailKite), raw bytes, a local file path, or base64 content;
// see AttachmentUpload. Accepts an AttachmentUpload (by value or pointer) or any
// map/struct (passed through as a JSON body, legacy behavior).
func (c *Client) UploadAttachment(file interface{}) (interface{}, error) {
	var f AttachmentUpload
	switch v := file.(type) {
	case AttachmentUpload:
		f = v
	case *AttachmentUpload:
		f = *v
	default:
		// Legacy / passthrough: any map or struct goes straight out as JSON.
		return c.Request("POST", "/v1/attachments", file)
	}

	// 1. URL → JSON body {url, filename?, contentType?, retentionDays?}.
	if f.Url != "" {
		return c.Request("POST", "/v1/attachments", AttachmentUpload{
			Url:           f.Url,
			Filename:      f.Filename,
			ContentType:   f.ContentType,
			RetentionDays: f.RetentionDays,
		})
	}

	// 2. Bytes / 3. Path → raw binary upload.
	if f.Bytes != nil || f.Path != "" {
		data := f.Bytes
		filename := f.Filename
		contentType := f.ContentType
		if f.Path != "" {
			b, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, err
			}
			data = b
			if filename == "" {
				filename = filepath.Base(f.Path)
			}
			if contentType == "" {
				contentType = guessContentType(f.Path)
			}
		}
		if contentType == "" {
			contentType = guessContentType(filename)
		}
		return c.requestBinary("/v1/attachments", data, filename, contentType, f.RetentionDays)
	}

	// 4. Content (base64) → JSON body {content, filename, contentType?, retentionDays?}.
	if f.Content != "" {
		return c.Request("POST", "/v1/attachments", AttachmentUpload{
			Content:       f.Content,
			Filename:      f.Filename,
			ContentType:   f.ContentType,
			RetentionDays: f.RetentionDays,
		})
	}

	return nil, fmt.Errorf("mailkite: UploadAttachment needs one of Url, Bytes, Path, or Content")
}

// requestBinary POSTs raw file bytes (not JSON, not multipart) to path, passing
// filename and retentionDays as query params and the file's media type as the
// Content-Type header. Response is parsed exactly like Request.
func (c *Client) requestBinary(path string, data []byte, filename, contentType string, retentionDays int) (interface{}, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	q := url.Values{}
	if filename != "" {
		q.Set("filename", filename)
	}
	if retentionDays != 0 {
		q.Set("retentionDays", strconv.Itoa(retentionDays))
	}
	u := c.BaseURL + path
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}

	req, err := http.NewRequest("POST", u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tok, err := c.token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", contentType)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)
	var parsed interface{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", res.StatusCode)
		if m, ok := parsed.(map[string]interface{}); ok {
			if e, ok := m["error"].(string); ok {
				msg = e
			}
		}
		return nil, &Error{Status: res.StatusCode, Message: msg, Body: parsed}
	}
	return parsed, nil
}

// extContentTypes maps lowercase file extensions to media types for raw binary
// uploads. Anything not listed falls back to application/octet-stream.
var extContentTypes = map[string]string{
	"pdf":  "application/pdf",
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"webp": "image/webp",
	"svg":  "image/svg+xml",
	"csv":  "text/csv",
	"txt":  "text/plain",
	"html": "text/html",
	"json": "application/json",
	"zip":  "application/zip",
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ics":  "text/calendar",
	"ical": "text/calendar",
}

// guessContentType returns a media type from name's extension, defaulting to
// application/octet-stream.
func guessContentType(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ct, ok := extContentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

func (c *Client) Agent(message interface{}) (interface{}, error) {
	return c.Request("POST", "/v1/agent", message)
}

func (c *Client) Route(message interface{}) (interface{}, error) {
	return c.Request("POST", "/v1/route", message)
}

// --- Domains ----------------------------------------------------------------

func (c *Client) ListDomains() (interface{}, error) {
	return c.Request("GET", "/api/domains", nil)
}

func (c *Client) CreateDomain(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/domains", body)
}

func (c *Client) GetDomain(id string) (interface{}, error) {
	return c.Request("GET", "/api/domains/"+id, nil)
}

func (c *Client) GetWebhookSecret(id string) (interface{}, error) {
	return c.Request("GET", "/api/domains/"+id+"/webhook/secret", nil)
}

func (c *Client) DeleteDomain(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/domains/"+id, nil)
}

func (c *Client) VerifyDomain(id string) (interface{}, error) {
	return c.Request("POST", "/api/domains/"+id+"/verify", nil)
}

func (c *Client) SetWebhook(id string, body interface{}) (interface{}, error) {
	return c.Request("PUT", "/api/domains/"+id+"/webhook", body)
}

func (c *Client) DeleteWebhook(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/domains/"+id+"/webhook", nil)
}

func (c *Client) SetTrackingWebhook(id string, body interface{}) (interface{}, error) {
	return c.Request("PUT", "/api/domains/"+id+"/tracking-webhook", body)
}

func (c *Client) DeleteTrackingWebhook(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/domains/"+id+"/tracking-webhook", nil)
}

func (c *Client) SetWebhookEvents(id string, body interface{}) (interface{}, error) {
	return c.Request("PUT", "/api/domains/"+id+"/webhook-events", body)
}

func (c *Client) DeleteWebhookEvents(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/domains/"+id+"/webhook-events", nil)
}

func (c *Client) TestWebhook(id string) (interface{}, error) {
	return c.Request("POST", "/api/domains/"+id+"/webhook/test", nil)
}

func (c *Client) CheckDomainAvailability(domain string) (interface{}, error) {
	return c.Request("GET", "/api/domains/register/check?domain="+url.QueryEscape(domain), nil)
}

func (c *Client) RegisterDomain(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/domains/register", body)
}

// --- Docs -------------------------------------------------------------------

func (c *Client) SemanticSearch(query string) (interface{}, error) {
	return c.Request("GET", "/v1/docs/search?query="+url.QueryEscape(query), nil)
}

// --- Routes -----------------------------------------------------------------

func (c *Client) ListRoutes() (interface{}, error) {
	return c.Request("GET", "/api/routes", nil)
}

func (c *Client) CreateRoute(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/routes", body)
}

func (c *Client) DeleteRoute(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/routes/"+id, nil)
}

// --- Templates --------------------------------------------------------------

func (c *Client) ListTemplates() (interface{}, error) {
	return c.Request("GET", "/api/templates", nil)
}

func (c *Client) ListBaseTemplates() (interface{}, error) {
	return c.Request("GET", "/api/templates/base", nil)
}

func (c *Client) GetTemplate(id string) (interface{}, error) {
	return c.Request("GET", "/api/templates/"+id, nil)
}

func (c *Client) CreateTemplate(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/templates", body)
}

// --- Messages & deliveries --------------------------------------------------

// ListOption sets an optional query parameter on a list call.
type ListOption func(url.Values)

// Before pages a list: return only rows older than this cursor (epoch ms).
func Before(v int64) ListOption {
	return func(q url.Values) { q.Set("before", strconv.FormatInt(v, 10)) }
}

// Limit caps the number of rows returned by a list call.
func Limit(v int) ListOption {
	return func(q url.Values) { q.Set("limit", strconv.Itoa(v)) }
}

// Search filters a message list by a substring of the sender, recipient, or subject.
func Search(v string) ListOption {
	return func(q url.Values) { q.Set("search", v) }
}

// listQuery applies opts to a url.Values and returns "?"+encoded (or "" when no
// opts set a value) to append to a list endpoint path.
func listQuery(opts []ListOption) string {
	q := url.Values{}
	for _, opt := range opts {
		opt(q)
	}
	if encoded := q.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

func (c *Client) ListMessages(opts ...ListOption) (interface{}, error) {
	return c.Request("GET", "/api/messages"+listQuery(opts), nil)
}

func (c *Client) GetMessage(id string) (interface{}, error) {
	return c.Request("GET", "/api/messages/"+id, nil)
}

func (c *Client) RetryDelivery(id string) (interface{}, error) {
	return c.Request("POST", "/api/deliveries/"+id+"/retry", nil)
}

// --- Contact lists ----------------------------------------------------------

func (c *Client) ListLists() (interface{}, error) {
	return c.Request("GET", "/api/lists", nil)
}

func (c *Client) CreateList(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/lists", body)
}

func (c *Client) GetList(id string) (interface{}, error) {
	return c.Request("GET", "/api/lists/"+id, nil)
}

func (c *Client) UpdateList(id string, body interface{}) (interface{}, error) {
	return c.Request("PATCH", "/api/lists/"+id, body)
}

func (c *Client) DeleteList(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/lists/"+id, nil)
}

func (c *Client) ListListContacts(id string, opts ...ListOption) (interface{}, error) {
	return c.Request("GET", "/api/lists/"+id+"/contacts"+listQuery(opts), nil)
}

func (c *Client) AddListContacts(id string, body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/lists/"+id+"/contacts", body)
}

func (c *Client) RemoveListContact(id, contactID string) (interface{}, error) {
	return c.Request("DELETE", "/api/lists/"+id+"/contacts/"+contactID, nil)
}

// --- Broadcasts -------------------------------------------------------------

func (c *Client) ListBroadcasts() (interface{}, error) {
	return c.Request("GET", "/api/broadcasts", nil)
}

func (c *Client) CreateBroadcast(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/broadcasts", body)
}

func (c *Client) GetBroadcast(id string) (interface{}, error) {
	return c.Request("GET", "/api/broadcasts/"+id, nil)
}

func (c *Client) UpdateBroadcast(id string, body interface{}) (interface{}, error) {
	return c.Request("PATCH", "/api/broadcasts/"+id, body)
}

func (c *Client) DeleteBroadcast(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/broadcasts/"+id, nil)
}

func (c *Client) SendBroadcast(id string, body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/broadcasts/"+id+"/send", body)
}

// --- Account ------------------------------------------------------------

// RegisterRequest is the body of Register(): email is required; channel,
// ref, and referrer are optional. (You may also pass any map/struct shaped
// like one to the low-level Request.)
type RegisterRequest struct {
	// The account email. A verification link is sent to it — the account
	// cannot send email until the address is verified, which is what makes
	// registering safe without a password.
	Email string `json:"email"`
	// Distribution-channel slug this registration came through (e.g.
	// wordpress-plugin). Invalid values are dropped, never an error.
	Channel string `json:"channel,omitempty"`
	// Referral code of the account that referred this signup, when any.
	Ref string `json:"ref,omitempty"`
	// First-touch landing referrer URL, when known. Invalid values are
	// dropped, never an error.
	Referrer string `json:"referrer,omitempty"`
}

// RegisterResponse is the result of Register().
type RegisterResponse struct {
	// The new account's API key (mk_live_…) — store it now; it is only
	// returned at registration. Sending stays blocked until the email is
	// verified.
	APIKey string `json:"api_key"`
	// The new account id (usr_…).
	UserID string `json:"user_id"`
	// The normalized account email the verification link was sent to.
	Email string `json:"email"`
	// Always false at registration — poll Me() until it flips after the user
	// clicks the link.
	EmailVerified bool `json:"email_verified"`
	// Always true — an existing email returns 409 account_exists (with no
	// credentials) instead.
	IsNew bool `json:"is_new"`
}

// Register creates a MailKite account from just an email — no password. Pass
// a RegisterRequest (or any map/struct shaped like one); email is required,
// channel/ref/referrer are optional. Returns the new account's API key
// immediately; a verification link is emailed, and sending stays blocked
// until the address is verified (poll Me()). An existing email returns 409
// account_exists with no credentials. Public — no auth required.
func (c *Client) Register(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/v1/provision", body)
}

// MeResponse is the result of Me().
type MeResponse struct {
	// The account email behind this credential.
	Email string `json:"email"`
	// Whether the account email is verified. Sending is blocked until true.
	EmailVerified bool `json:"emailVerified"`
	// The account's plan id.
	Plan string `json:"plan"`
}

// Me returns the account behind this credential: email, whether it is
// verified (sending is blocked until it is), and plan. Use to poll
// verification state after Register().
func (c *Client) Me() (interface{}, error) {
	return c.Request("GET", "/v1/me", nil)
}

// APIKeyResponse is the result of GetApiKey() and RotateApiKey().
type APIKeyResponse struct {
	// The account's unrestricted API key (mk_live_…). Read-or-create: the
	// first call mints it. RotateApiKey invalidates the old key and returns
	// a fresh one.
	Key string `json:"key"`
}

// GetApiKey gets the account's unrestricted API key (mk_live_…).
// Read-or-create: the first call mints it.
func (c *Client) GetApiKey() (interface{}, error) {
	return c.Request("GET", "/api/keys", nil)
}

// RotateApiKey rotates the account API key: the old key stops working
// immediately and a fresh one is returned. The plaintext is only shown here.
func (c *Client) RotateApiKey() (interface{}, error) {
	return c.Request("POST", "/api/keys/rotate", nil)
}

// ScopedKey is a domain-scoped API key, as returned by ListScopedKeys and
// CreateScopedKey.
type ScopedKey struct {
	// Key id (key_…).
	ID string `json:"id"`
	// Owning account (usr_…).
	UserID string `json:"user_id"`
	// The one domain this key is scoped to (dom_…). A scoped key can send
	// and manage only this domain — ideal for a single site or CI job.
	DomainID string `json:"domain_id"`
	// Denormalized domain name, for display and fast scope checks.
	Domain string `json:"domain"`
	// User-supplied label (e.g. the site or service using it).
	Name string `json:"name"`
	// The key material (mk_live_…). Returned in full on list/create — treat
	// like a password.
	Key string `json:"key"`
	// Creation time, ms epoch.
	CreatedAt int64 `json:"created_at"`
	// Last authenticated use, ms epoch — nil if never used.
	LastUsedAt *int64 `json:"last_used_at"`
}

// ListScopedKeys lists the account's domain-scoped API keys. A scoped key
// can send and manage only its one domain — hand one to each site or CI job
// so a leak burns only that surface.
func (c *Client) ListScopedKeys() (interface{}, error) {
	return c.Request("GET", "/api/keys/scoped", nil)
}

// CreateScopedKeyRequest is the body of CreateScopedKey(): domainId is
// required; name is optional. (You may also pass any map/struct shaped like
// one to the low-level Request.)
type CreateScopedKeyRequest struct {
	// The domain (dom_…) the new key is limited to. Must belong to the
	// calling account.
	DomainID string `json:"domainId"`
	// Optional label shown in the dashboard (e.g. which site/integration
	// holds this key).
	Name string `json:"name,omitempty"`
}

// CreateScopedKey creates a key scoped to one domain. Ideal for per-site
// installs (e.g. a WordPress plugin) — the site never holds the account
// master key.
func (c *Client) CreateScopedKey(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/keys/scoped", body)
}

// DeleteScopedKey revokes a domain-scoped key. Takes effect immediately.
func (c *Client) DeleteScopedKey(id string) (interface{}, error) {
	return c.Request("DELETE", "/api/keys/scoped/"+id, nil)
}

// UsageEmails is the emails field of UsageResponse.
type UsageEmails struct {
	// Emails counted so far this period.
	Used int `json:"used"`
	// The plan's included monthly emails — nil means unlimited.
	Included *int `json:"included"`
}

// UsageAIActions is the aiActions field of UsageResponse.
type UsageAIActions struct {
	// AI actions counted so far this period.
	Used int `json:"used"`
}

// UsageOverage is the overage field of UsageResponse.
type UsageOverage struct {
	// Past the plan's included email bucket this period.
	Over bool `json:"over"`
	// Overage is being metered and billed normally.
	Metered bool `json:"metered"`
	// Sending is blocked until the payment card is fixed.
	Blocked bool `json:"blocked"`
	// Card-gate status when in overage (ok | failed …) — nil outside
	// overage.
	Status *string `json:"status"`
	// Machine-readable reason code when blocked — nil otherwise.
	Code *string `json:"code"`
}

// UsageResponse is the result of GetUsage(): current billing-period usage.
type UsageResponse struct {
	// The account's plan id.
	Plan string `json:"plan"`
	// Email volume this billing period (inbound + outbound share one
	// bucket).
	Emails UsageEmails `json:"emails"`
	// AI agent actions this period (pay-as-you-go — no included bucket).
	AIActions UsageAIActions `json:"aiActions"`
	// True when overage metering is active (billing configured and a
	// subscription exists).
	MeteringEnabled bool `json:"meteringEnabled"`
	// The usage meters billed on this account.
	Meters []string `json:"meters"`
	// Overage state — drives the dashboard notice and the plugin quota bar.
	Overage UsageOverage `json:"overage"`
}

// GetUsage returns current billing-period usage: emails used vs the plan's
// included bucket (nil = unlimited), AI actions, and the overage state that
// gates sending. Powers quota meters in dashboards and integrations.
func (c *Client) GetUsage() (interface{}, error) {
	return c.Request("GET", "/api/billing/usage", nil)
}

// --- Suppressions ---------------------------------------------------------

// Suppression is one suppressed address, as returned by ListSuppressions.
type Suppression struct {
	// Suppression id (sup_…).
	ID string `json:"id"`
	// Owning account (usr_…).
	UserID string `json:"user_id"`
	// The suppressed address (normalized lowercase).
	Email string `json:"email"`
	// Why the address is suppressed: unsubscribe | hard_bounce |
	// spam_complaint | manual.
	Reason string `json:"reason"`
	// Who created it: system (bounce/complaint webhook) or customer
	// (manual/API).
	Origin string `json:"origin"`
	// Free-text note attached at creation — nil when none.
	Note *string `json:"note"`
	// Creation time, ms epoch.
	CreatedAt int64 `json:"created_at"`
}

// SuppressionsResponse is the result of ListSuppressions().
type SuppressionsResponse struct {
	// Addresses this account will not send to, newest first. Sends to a
	// suppressed address are dropped before delivery.
	Suppressions []Suppression `json:"suppressions"`
}

// ListSuppressions lists suppressed addresses (unsubscribes, hard bounces,
// spam complaints, manual). Sends to a suppressed address are dropped before
// delivery.
func (c *Client) ListSuppressions() (interface{}, error) {
	return c.Request("GET", "/api/contacts/suppressions", nil)
}

// AddSuppressionRequest is the body of AddSuppression(): email is required;
// reason and note are optional. (You may also pass any map/struct shaped
// like one to the low-level Request.)
type AddSuppressionRequest struct {
	// The address to stop sending to.
	Email string `json:"email"`
	// Why — defaults to manual when omitted or unrecognized.
	Reason string `json:"reason,omitempty"`
	// Optional free-text note (who/why), shown alongside the entry.
	Note string `json:"note,omitempty"`
}

// AddSuppressionResponse is the result of AddSuppression().
type AddSuppressionResponse struct {
	// Always true on success.
	Suppressed bool `json:"suppressed"`
	// The normalized address that was suppressed.
	Email string `json:"email"`
}

// AddSuppression suppresses an address so this account never sends to it
// again (reason defaults to manual).
func (c *Client) AddSuppression(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/contacts/suppressions", body)
}

// RemoveSuppressionResponse is the result of RemoveSuppression().
type RemoveSuppressionResponse struct {
	// Always true — removing an address that was not suppressed is a no-op
	// success.
	Removed bool `json:"removed"`
}

// RemoveSuppression removes an address from the suppression list
// (URL-encodes email). Removing an unsuppressed address is a no-op success.
func (c *Client) RemoveSuppression(email string) (interface{}, error) {
	return c.Request("DELETE", "/api/contacts/suppressions/"+url.QueryEscape(email), nil)
}

// --- Webhooks ---------------------------------------------------------------

// VerifyWebhook reports whether signature is a valid x-mailkite-signature header
// for payload, using the default 5-minute replay window. It is a local
// HMAC-SHA256 check — no network call. Pass the raw, unparsed request body.
func (c *Client) VerifyWebhook(signature, payload, secret string) bool {
	return VerifyWebhook(signature, payload, secret)
}

// VerifyWebhook is the package-level form of (*Client).VerifyWebhook; no client
// is required to verify a signature.
func VerifyWebhook(signature, payload, secret string) bool {
	return VerifyWebhookWithTolerance(signature, payload, secret, DefaultToleranceMs)
}

// VerifyWebhookWithTolerance verifies the signature and rejects events older
// than toleranceMs milliseconds (0 disables the freshness check).
func VerifyWebhookWithTolerance(signature, payload, secret string, toleranceMs int64) bool {
	if signature == "" {
		return false
	}
	var t, v1 string
	for _, seg := range strings.Split(signature, ",") {
		i := strings.Index(seg, "=")
		if i < 0 {
			continue
		}
		switch strings.TrimSpace(seg[:i]) {
		case "t":
			t = strings.TrimSpace(seg[i+1:])
		case "v1":
			v1 = strings.TrimSpace(seg[i+1:])
		}
	}
	if t == "" || v1 == "" {
		return false
	}
	ts, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return false
	}
	// The t in the header is milliseconds since the epoch.
	if toleranceMs > 0 {
		diff := time.Now().UnixMilli() - ts
		if diff < 0 {
			diff = -diff
		}
		if diff > toleranceMs {
			return false
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t + "." + payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(v1))
}

// --- Reply helper -----------------------------------------------------------

// ReplyOk returns the exact body a webhook handler can return to acknowledge an
// event: the JSON string {"status":"ok"}. It is purely local — no network call.
func (c *Client) ReplyOk() string { return ReplyOk() }

// ReplyOk is the package-level form of (*Client).ReplyOk.
func ReplyOk() string { return `{"status":"ok"}` }

// ReplySpam returns the control-mode body telling MailKite to mark the message
// as spam: the JSON string {"status":"spam"}. Purely local — no network call.
func (c *Client) ReplySpam() string { return ReplySpam() }

// ReplySpam is the package-level form of (*Client).ReplySpam.
func ReplySpam() string { return `{"status":"spam"}` }

// ReplyDrop returns the control-mode body telling MailKite to drop (discard) the
// message: the JSON string {"status":"drop"}. Purely local — no network call.
func (c *Client) ReplyDrop() string { return ReplyDrop() }

// ReplyDrop is the package-level form of (*Client).ReplyDrop.
func ReplyDrop() string { return `{"status":"drop"}` }

// ReplyBlockSender returns the control-mode body telling MailKite to block the
// sender: {"status":"ok","actions":[{"type":"block-sender"}]}. Purely local.
func (c *Client) ReplyBlockSender() string { return ReplyBlockSender() }

// ReplyBlockSender is the package-level form of (*Client).ReplyBlockSender.
func ReplyBlockSender() string { return `{"status":"ok","actions":[{"type":"block-sender"}]}` }

// --- At-rest encryption -----------------------------------------------------

// envelope is the stored/serialized at-rest encryption envelope. All binary
// fields are base64. It is byte-compatible with MailKite's WebCrypto envelope
// (see api/src/lib/encryption.ts).
type envelope struct {
	V          int    `json:"v"`
	KeyAlg     string `json:"keyAlg"`
	FP         string `json:"fp"`
	Enc        string `json:"enc"`
	IV         string `json:"iv"`
	WrappedKey string `json:"wrappedKey"`
	Ciphertext string `json:"ciphertext"`
}

// Encrypt protects a UTF-8 plaintext to an RSA public key (SPKI PEM), returning
// the at-rest envelope serialized as a compact JSON string. It uses a hybrid
// scheme — a fresh AES-256-GCM content key encrypts the data and is then wrapped
// with RSA-OAEP (SHA-256). Local only — no network call.
func (c *Client) Encrypt(plaintext, publicKeyPEM string) (string, error) {
	return Encrypt(plaintext, publicKeyPEM)
}

// Encrypt is the package-level form of (*Client).Encrypt.
func Encrypt(plaintext, publicKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return "", fmt.Errorf("mailkite: could not decode public key PEM")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("mailkite: parse public key: %w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("mailkite: public key is not RSA")
	}

	// fp = lowercase hex sha256 of the SPKI DER (the PEM block bytes).
	sum := sha256.Sum256(block.Bytes)
	fp := hex.EncodeToString(sum[:])

	// Fresh 32-byte AES-256 content key.
	rawKey := make([]byte, 32)
	if _, err := rand.Read(rawKey); err != nil {
		return "", err
	}
	blk, err := aes.NewCipher(rawKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return "", err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	// gcm.Seal returns ciphertext||tag, matching WebCrypto's AES-GCM output.
	ct := gcm.Seal(nil, iv, []byte(plaintext), nil)

	// Wrap the AES key with RSA-OAEP (SHA-256 for both hash and MGF1).
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, rawKey, nil)
	if err != nil {
		return "", err
	}

	env := envelope{
		V:          1,
		KeyAlg:     "RSA-OAEP-256",
		FP:         fp,
		Enc:        "A256GCM",
		IV:         base64.StdEncoding.EncodeToString(iv),
		WrappedKey: base64.StdEncoding.EncodeToString(wrapped),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}
	out, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Decrypt reverses Encrypt: given an at-rest envelope (JSON string) and an RSA
// private key (PKCS#8 PEM), it returns the original UTF-8 plaintext. Local only.
func (c *Client) Decrypt(envelopeJSON, privateKeyPEM string) (string, error) {
	return Decrypt(envelopeJSON, privateKeyPEM)
}

// Decrypt is the package-level form of (*Client).Decrypt.
func Decrypt(envelopeJSON, privateKeyPEM string) (string, error) {
	var env envelope
	if err := json.Unmarshal([]byte(envelopeJSON), &env); err != nil {
		return "", fmt.Errorf("mailkite: parse envelope: %w", err)
	}

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("mailkite: could not decode private key PEM")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("mailkite: parse private key: %w", err)
	}
	priv, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("mailkite: private key is not RSA")
	}

	wrapped, err := base64.StdEncoding.DecodeString(env.WrappedKey)
	if err != nil {
		return "", fmt.Errorf("mailkite: decode wrappedKey: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(env.IV)
	if err != nil {
		return "", fmt.Errorf("mailkite: decode iv: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("mailkite: decode ciphertext: %w", err)
	}

	rawKey, err := rsa.DecryptOAEP(sha256.New(), nil, priv, wrapped, nil)
	if err != nil {
		return "", fmt.Errorf("mailkite: unwrap content key: %w", err)
	}
	blk, err := aes.NewCipher(rawKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return "", err
	}
	// Go's gcm.Open expects ciphertext||tag, matching WebCrypto.
	pt, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		return "", fmt.Errorf("mailkite: decrypt: %w", err)
	}
	return string(pt), nil
}
