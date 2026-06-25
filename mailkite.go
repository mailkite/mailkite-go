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
	From         string         `json:"from"`
	To           interface{}    `json:"to"`
	Subject      string         `json:"subject,omitempty"`
	HTML         string         `json:"html,omitempty"`
	Text         string         `json:"text,omitempty"`
	CC           interface{}    `json:"cc,omitempty"`
	BCC          interface{}    `json:"bcc,omitempty"`
	ReplyTo      string         `json:"replyTo,omitempty"`
	InReplyTo    string         `json:"inReplyTo,omitempty"`
	Attachments  []Attachment   `json:"attachments,omitempty"`
	TemplateID   string         `json:"templateId,omitempty"`
	TemplateData map[string]any `json:"templateData,omitempty"`
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

// UploadAttachment uploads a file (base64 content) and returns a secure,
// time-limited URL to reference as a Send() attachment ({ filename, url }) or
// link inline — instead of base64-inlining large files on every send.
func (c *Client) UploadAttachment(file interface{}) (interface{}, error) {
	return c.Request("POST", "/v1/attachments", file)
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
