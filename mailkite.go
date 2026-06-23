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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Attachment is one item in Message.Attachments.
type Attachment struct {
	Filename    string `json:"filename"`
	URL         string `json:"url,omitempty"`
	Content     string `json:"content,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

// Message is the body of a Send call. (You may also pass any map/struct.)
type Message struct {
	From        string       `json:"from"`
	To          interface{}  `json:"to"`
	Subject     string       `json:"subject"`
	HTML        string       `json:"html,omitempty"`
	Text        string       `json:"text,omitempty"`
	CC          interface{}  `json:"cc,omitempty"`
	BCC         interface{}  `json:"bcc,omitempty"`
	ReplyTo     string       `json:"replyTo,omitempty"`
	InReplyTo   string       `json:"inReplyTo,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Client talks to the MailKite API.
type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for the production API.
func New(apiKey string) *Client {
	return &Client{APIKey: apiKey, BaseURL: DefaultBaseURL, HTTP: http.DefaultClient}
}

// NewWithBaseURL returns a client pointed at a custom base URL.
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{APIKey: apiKey, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: http.DefaultClient}
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
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
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

func (c *Client) TestWebhook(id string) (interface{}, error) {
	return c.Request("POST", "/api/domains/"+id+"/webhook/test", nil)
}

func (c *Client) CheckDomainAvailability(domain string) (interface{}, error) {
	return c.Request("GET", "/api/domains/register/check?domain="+url.QueryEscape(domain), nil)
}

func (c *Client) RegisterDomain(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/domains/register", body)
}

// --- Routes -----------------------------------------------------------------

func (c *Client) ListRoutes() (interface{}, error) {
	return c.Request("GET", "/api/routes", nil)
}

func (c *Client) CreateRoute(body interface{}) (interface{}, error) {
	return c.Request("POST", "/api/routes", body)
}

// --- Messages & deliveries --------------------------------------------------

func (c *Client) ListMessages() (interface{}, error) {
	return c.Request("GET", "/api/messages", nil)
}

func (c *Client) GetMessage(id string) (interface{}, error) {
	return c.Request("GET", "/api/messages/"+id, nil)
}

func (c *Client) RetryDelivery(id string) (interface{}, error) {
	return c.Request("POST", "/api/deliveries/"+id+"/retry", nil)
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
