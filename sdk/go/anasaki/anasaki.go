// Package anasaki is the SDK for Anasaki Studio service plugins: the server
// program of a plugin that runs next to the CMS (see docs/plugins.md §2).
//
//	p := anasaki.New(anasaki.Config{Name: "Hello", Version: "1.0.0"}) // reads ANASAKI_* env
//	p.Public("GET /hello", func(w http.ResponseWriter, r *http.Request) {
//		anasaki.Page(w, http.StatusOK, "Hello", "<h1>Hello!</h1>")
//	})
//	p.Admin("GET /dashboard", dashboard)
//	p.Block("greeting", func(in anasaki.BlockInput) anasaki.BlockOutput { … })
//	p.Collection("quotes", func(q map[string]any) []map[string]any { … })
//	p.OnEvent(func(e anasaki.Event) { … })
//	log.Fatal(http.ListenAndServe(":9300", p))
//
// Every request from the CMS is checked against the shared secret before your
// handlers run; p.CMS calls the CMS back with the same signature.
package anasaki

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Protocol headers (same as the CMS).
const (
	HeaderPlugin    = "X-Anasaki-Plugin"
	HeaderTimestamp = "X-Anasaki-Timestamp"
	HeaderSignature = "X-Anasaki-Signature"
	HeaderSigV2     = "X-Anasaki-Signature-V2"
	HeaderNonce     = "X-Anasaki-Nonce"
	HeaderUser      = "X-Anasaki-User"
	HeaderSiteURL   = "X-Anasaki-Site-Url"
	HeaderAdminBase = "X-Anasaki-Admin-Base"
	HeaderLayout    = "X-Anasaki-Layout"
	HeaderTitle     = "X-Anasaki-Title"
)

// Config configures a plugin service. Empty fields come from the environment:
// ANASAKI_PLUGIN_ID, ANASAKI_PLUGIN_SECRET, ANASAKI_URL (the CMS, for p.CMS).
type Config struct {
	ID      string
	Secret  string
	CMSURL  string
	Name    string // shown by the CMS connection test
	Version string
	// MaxBody limits request bodies from the CMS (default 20 MB).
	MaxBody int64
	// RequireV2 refuses requests without the v2 signature (CMS versions that
	// only sign with v1). Without it, v1-only requests are accepted until the
	// first v2 request arrives, and refused from then on.
	RequireV2 bool
}

// Plugin is an http.Handler serving the CMS protocol.
type Plugin struct {
	cfg         Config
	public      *http.ServeMux
	admin       *http.ServeMux
	blocks      map[string]func(BlockInput) BlockOutput
	collections map[string]func(map[string]any) []map[string]any
	events      []func(Event)
	// trusted skips the signature check: in WASM mode the CMS runs the
	// module itself, so requests can't come from anywhere else.
	trusted bool
	// CMS calls the CMS service API (/api/service/...).
	CMS *Client
}

// New creates a plugin service.
func New(cfg Config) *Plugin {
	if cfg.ID == "" {
		cfg.ID = os.Getenv("ANASAKI_PLUGIN_ID")
	}
	if cfg.Secret == "" {
		cfg.Secret = os.Getenv("ANASAKI_PLUGIN_SECRET")
	}
	if cfg.CMSURL == "" {
		cfg.CMSURL = os.Getenv("ANASAKI_URL")
	}
	if cfg.MaxBody == 0 {
		cfg.MaxBody = 20 << 20
	}
	if cfg.Secret == "" && !inWasm {
		log.Print("anasaki: ANASAKI_PLUGIN_SECRET is empty — every request will be refused")
	}
	return &Plugin{cfg: cfg, public: http.NewServeMux(), admin: http.NewServeMux(),
		blocks: map[string]func(BlockInput) BlockOutput{}, collections: map[string]func(map[string]any) []map[string]any{},
		CMS: newClient(cfg)}
}

// Public handles a public site path ("GET /shop/{slug}"); it must be under a
// prefix listed in the manifest's service.public.
func (p *Plugin) Public(pattern string, h http.HandlerFunc) { p.public.HandleFunc(pattern, h) }

// Admin handles an admin screen path ("GET /dashboard", "POST /api/save").
// Use AdminURL(r, "…") for links: screens live under a per-session prefix.
func (p *Plugin) Admin(pattern string, h http.HandlerFunc) { p.admin.HandleFunc(pattern, h) }

// Block renders a designer block declared with "render": "service".
func (p *Plugin) Block(typ string, fn func(BlockInput) BlockOutput) { p.blocks[typ] = fn }

// Collection serves a collection source declared in service.collections.
func (p *Plugin) Collection(source string, fn func(query map[string]any) []map[string]any) {
	p.collections[source] = fn
}

// OnEvent receives the core events listed in service.events.
func (p *Plugin) OnEvent(fn func(Event)) { p.events = append(p.events, fn) }

// BlockInput is what the CMS sends to render a block.
type BlockInput struct {
	Values   map[string]any    `json:"values"`
	Settings map[string]any    `json:"settings"`
	Site     map[string]string `json:"site"`
	Post     map[string]string `json:"post"`
	Item     map[string]string `json:"item"`
}

// BlockOutput is the block's inner HTML and optional CSS (scoped by the CMS).
type BlockOutput struct {
	HTML string `json:"html"`
	CSS  string `json:"css"`
}

// Event is a core event ("post.published", "user.created", …).
type Event struct {
	Event  string         `json:"event"`
	Data   map[string]any `json:"data"`
	SentAt string         `json:"sentAt"`
}

// User is the admin who opened an admin screen.
type User struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Role   string   `json:"role"`
	Caps   []string `json:"caps"`
	Locale string   `json:"locale"`
}

// Can reports whether the user has a capability.
func (u *User) Can(cap string) bool {
	if u == nil {
		return false
	}
	if u.Role == "administrator" {
		return true
	}
	for _, c := range u.Caps {
		if c == cap {
			return true
		}
	}
	return false
}

type userKey struct{}

// UserFrom returns the admin user of an admin-screen request (nil elsewhere).
func UserFrom(r *http.Request) *User {
	u, _ := r.Context().Value(userKey{}).(*User)
	return u
}

// AdminURL builds a link to another admin screen of this plugin ("orders/12").
func AdminURL(r *http.Request, path string) string {
	return r.Header.Get(HeaderAdminBase) + strings.TrimPrefix(path, "/")
}

// SiteURL is the public base URL of the site (public requests).
func SiteURL(r *http.Request) string { return r.Header.Get(HeaderSiteURL) }

// Page answers a public request with HTML that the CMS wraps in the site's
// header and footer (so the page looks like the rest of the site).
func Page(w http.ResponseWriter, status int, title, html string) {
	w.Header().Set(HeaderLayout, "page")
	w.Header().Set(HeaderTitle, url.QueryEscape(title))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, html)
}

// JSON writes v as JSON.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------- serving

// ServeHTTP verifies the CMS signature, then routes the request.
func (p *Plugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, p.cfg.MaxBody+1))
	if err != nil || int64(len(body)) > p.cfg.MaxBody {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !p.trusted {
		err := Verify(p.cfg.Secret, r, body)
		if err == nil && p.cfg.RequireV2 && r.Header.Get(HeaderSigV2) == "" {
			err = ErrBadSignature
		}
		if err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	path := r.URL.Path
	switch {
	case path == "/health":
		JSON(w, http.StatusOK, map[string]string{"name": p.cfg.Name, "version": p.cfg.Version, "id": p.cfg.ID})
	case strings.HasPrefix(path, "/public/"):
		p.serveSub(w, r, p.public, "/public")
	case path == "/admin" || strings.HasPrefix(path, "/admin/"):
		u := &User{}
		if raw, err := base64.StdEncoding.DecodeString(r.Header.Get(HeaderUser)); err == nil {
			_ = json.Unmarshal(raw, u)
		}
		p.serveSub(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)), p.admin, "/admin")
	case strings.HasPrefix(path, "/blocks/") && r.Method == http.MethodPost:
		fn := p.blocks[strings.TrimPrefix(path, "/blocks/")]
		if fn == nil {
			http.NotFound(w, r)
			return
		}
		var in BlockInput
		_ = json.Unmarshal(body, &in)
		JSON(w, http.StatusOK, fn(in))
	case strings.HasPrefix(path, "/collections/") && r.Method == http.MethodPost:
		fn := p.collections[strings.TrimPrefix(path, "/collections/")]
		if fn == nil {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Query map[string]any `json:"query"`
		}
		_ = json.Unmarshal(body, &in)
		items := fn(in.Query)
		if items == nil {
			items = []map[string]any{}
		}
		JSON(w, http.StatusOK, map[string]any{"items": items})
	case path == "/events" && r.Method == http.MethodPost:
		var e Event
		_ = json.Unmarshal(body, &e)
		for _, fn := range p.events {
			fn(e)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// serveSub strips prefix and dispatches to mux.
func (p *Plugin) serveSub(w http.ResponseWriter, r *http.Request, mux *http.ServeMux, prefix string) {
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Path = strings.TrimPrefix(r.URL.Path, prefix)
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawPath = ""
	r2.URL = &u
	r2.RequestURI = u.RequestURI()
	mux.ServeHTTP(w, r2)
}

// ---------------------------------------------------------------- signing

// ErrBadSignature is returned for an unsigned, stale or forged request.
var ErrBadSignature = errors.New("anasaki: bad signature")

// Sign computes the protocol signature.
func Sign(secret string, ts int64, method, requestURI string, body []byte) string {
	sum := sha256.Sum256(body)
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "%d\n%s\n%s\n%x", ts, strings.ToUpper(method), requestURI, sum)
	return hex.EncodeToString(h.Sum(nil))
}

// SignV2 computes the v2 signature: it also covers a per-request nonce and
// the X-Anasaki-Plugin, X-Anasaki-User and X-Anasaki-Admin-Base headers.
func SignV2(secret string, ts int64, nonce, method, requestURI string, body []byte, plugin, user, adminBase string) string {
	sum := sha256.Sum256(body)
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "v2\n%d\n%s\n%s\n%s\n%x\n%s\n%s\n%s", ts, nonce, strings.ToUpper(method), requestURI, sum,
		plugin, user, adminBase)
	return hex.EncodeToString(h.Sum(nil))
}

// SignRequest sets the timestamp, a fresh nonce and both signatures (v1 for
// older peers, v2) on req. Set every other X-Anasaki-* header first.
func SignRequest(secret string, req *http.Request, body []byte) {
	ts := time.Now().Unix()
	nb := make([]byte, 16)
	_, _ = rand.Read(nb)
	nonce := hex.EncodeToString(nb)
	uri := req.URL.RequestURI()
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, Sign(secret, ts, req.Method, uri, body))
	req.Header.Set(HeaderSigV2, SignV2(secret, ts, nonce, req.Method, uri, body,
		req.Header.Get(HeaderPlugin), req.Header.Get(HeaderUser), req.Header.Get(HeaderAdminBase)))
}

const signatureWindow = 5 * time.Minute

var (
	nonceRe = regexp.MustCompile(`^[0-9a-f]{16,64}$`)
	nonces  = newNonceCache(1 << 20)
	// sawV2: the CMS signs with v2, so a v1-only request is a downgrade.
	sawV2 atomic.Bool
)

// Verify checks a signed request (body already read). Requests more than
// five minutes old (or ahead) are refused. When the request carries the v2
// signature, v2 is checked and its nonce is accepted only once; v1-only
// requests (from CMS versions before v2) are accepted until the first v2
// request has been seen.
func Verify(secret string, r *http.Request, body []byte) error {
	ts, err := strconv.ParseInt(r.Header.Get(HeaderTimestamp), 10, 64)
	if err != nil || secret == "" {
		return ErrBadSignature
	}
	if d := time.Since(time.Unix(ts, 0)); d > signatureWindow || d < -signatureWindow {
		return ErrBadSignature
	}
	if v2 := r.Header.Get(HeaderSigV2); v2 != "" {
		nonce := r.Header.Get(HeaderNonce)
		if !nonceRe.MatchString(nonce) {
			return ErrBadSignature
		}
		want := SignV2(secret, ts, nonce, r.Method, r.URL.RequestURI(), body,
			r.Header.Get(HeaderPlugin), r.Header.Get(HeaderUser), r.Header.Get(HeaderAdminBase))
		if !hmac.Equal([]byte(want), []byte(v2)) {
			return ErrBadSignature
		}
		if !nonces.add(nonce, time.Now()) {
			return ErrBadSignature // replayed
		}
		sawV2.Store(true)
		return nil
	}
	if sawV2.Load() {
		return ErrBadSignature
	}
	want := Sign(secret, ts, r.Method, r.URL.RequestURI(), body)
	if !hmac.Equal([]byte(want), []byte(r.Header.Get(HeaderSignature))) {
		return ErrBadSignature
	}
	return nil
}

// nonceCache remembers the nonces of verified requests for as long as their
// timestamp could be accepted (bounded: past max entries the oldest go).
type nonceCache struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	order []nonceEntry // insertion order = expiry order
	max   int
}

type nonceEntry struct {
	nonce string
	exp   time.Time
}

func newNonceCache(max int) *nonceCache { return &nonceCache{seen: map[string]struct{}{}, max: max} }

// add records nonce; false when it was already seen.
func (c *nonceCache) add(nonce string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.order) > 0 && (now.After(c.order[0].exp) || len(c.order) >= c.max) {
		delete(c.seen, c.order[0].nonce)
		c.order = c.order[1:]
	}
	if _, dup := c.seen[nonce]; dup {
		return false
	}
	c.seen[nonce] = struct{}{}
	c.order = append(c.order, nonceEntry{nonce, now.Add(2 * signatureWindow)})
	return true
}

// ---------------------------------------------------------------- CMS client

// Client calls the CMS service API; each call needs the matching scope in the
// manifest's service.api.
type Client struct {
	id, secret, base string
	http             *http.Client
	transport        func(ctx context.Context, method, uri string, body []byte) (int, []byte, error)
}

// Do sends a signed request to /api/service/<path> and decodes JSON into out.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	status, resp, err := c.transport(ctx, method, "/api/service/"+strings.TrimPrefix(path, "/"), body)
	if err != nil {
		return err
	}
	if status >= 300 {
		var e struct{ Error string }
		_ = json.Unmarshal(resp, &e)
		return fmt.Errorf("anasaki: %s %s: %d %s", method, path, status, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp, out)
}

// httpTransport calls the CMS over HTTP (standalone service).
func (c *Client) httpTransport(ctx context.Context, method, uri string, body []byte) (int, []byte, error) {
	if c.base == "" {
		return 0, nil, errors.New("anasaki: ANASAKI_URL is not set")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+uri, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set(HeaderPlugin, c.id)
	SignRequest(c.secret, req, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp.StatusCode, b, err
}

// ---------------------------------------------------------------- storage & fetch

// Get reads a stored value into out (scope "storage"); found is false for a missing key.
func (c *Client) Get(ctx context.Context, key string, out any) (found bool, err error) {
	var r struct {
		Found bool            `json:"found"`
		Value json.RawMessage `json:"value"`
	}
	if err := c.Do(ctx, http.MethodGet, "kv/get?key="+url.QueryEscape(key), nil, &r); err != nil || !r.Found {
		return false, err
	}
	return true, json.Unmarshal(r.Value, out)
}

// Set stores any JSON value under key (scope "storage"; ≤ 1 MB per value).
func (c *Client) Set(ctx context.Context, key string, value any) error {
	return c.Do(ctx, http.MethodPost, "kv/set", map[string]any{"key": key, "value": value}, nil)
}

// Delete removes a key (scope "storage").
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.Do(ctx, http.MethodPost, "kv/delete", map[string]any{"key": key}, nil)
}

// KV is a stored key and its JSON value.
type KV struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// List returns keys starting with prefix, in order (scope "storage"; limit ≤ 500).
func (c *Client) List(ctx context.Context, prefix string, limit int) ([]KV, error) {
	var r struct {
		Items []KV `json:"items"`
	}
	err := c.Do(ctx, http.MethodGet, "kv/list?prefix="+url.QueryEscape(prefix)+"&limit="+strconv.Itoa(limit), nil, &r)
	return r.Items, err
}

// FetchResult is the answer of Fetch.
type FetchResult struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// Fetch calls an external https API listed in the manifest's service.hosts
// (scope "http") — e.g. a payment gateway.
func (c *Client) Fetch(ctx context.Context, method, rawURL string, headers map[string]string, body string) (FetchResult, error) {
	var r FetchResult
	err := c.Do(ctx, http.MethodPost, "fetch", map[string]any{"method": method, "url": rawURL, "headers": headers, "body": body}, &r)
	return r, err
}

// Site is the site's public information (scope "site").
type Site struct {
	Title    string `json:"title"`
	Tagline  string `json:"tagline"`
	URL      string `json:"url"`
	Logo     string `json:"logo"`
	Icon     string `json:"icon"`
	Language string `json:"language"`
	Timezone string `json:"timezone"`
	Theme    struct {
		Colors      map[string]string `json:"colors"`
		FontHeading string            `json:"fontHeading"`
		FontBody    string            `json:"fontBody"`
	} `json:"theme"`
}

// Site reads the site's title, URL, logo and theme (scope "site").
func (c *Client) Site(ctx context.Context) (Site, error) {
	var s Site
	err := c.Do(ctx, http.MethodGet, "site", nil, &s)
	return s, err
}

// SendMail sends an email through the site's mail settings (scope "mail").
func (c *Client) SendMail(ctx context.Context, to []string, subject, html, replyTo string) error {
	return c.Do(ctx, http.MethodPost, "mail", map[string]any{"to": to, "subject": subject, "html": html, "replyTo": replyTo}, nil)
}

// Emit publishes "<plugin id>.<event>" to the CMS and other plugins (scope "events").
func (c *Client) Emit(ctx context.Context, event string, data map[string]any) error {
	return c.Do(ctx, http.MethodPost, "events", map[string]any{"event": event, "data": data}, nil)
}

// Posts lists published posts or pages (scope "content"); q may hold type,
// category, search, order and limit.
func (c *Client) Posts(ctx context.Context, q url.Values) ([]map[string]string, error) {
	var out struct {
		Items []map[string]string `json:"items"`
	}
	err := c.Do(ctx, http.MethodGet, "posts?"+q.Encode(), nil, &out)
	return out.Items, err
}

// User reads one user (scope "users").
func (c *Client) User(ctx context.Context, id int64) (map[string]any, error) {
	var out map[string]any
	err := c.Do(ctx, http.MethodGet, "users/"+strconv.FormatInt(id, 10), nil, &out)
	return out, err
}
