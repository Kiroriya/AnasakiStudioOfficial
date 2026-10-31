package plugin

// WASM plugins: a service plugin whose program ships inside the package as a
// WebAssembly module ("service": {"runtime": "wasm", "module": "plugin.wasm"}).
// It is installed with one click like any package and runs inside the CMS in
// a sandbox (wazero, WASI preview 1): no files, no network, no database —
// only what the host functions below allow, limited by the manifest's
// service.api scopes.
//
// Each request runs the module once: the CMS writes the request to stdin as
// JSON {method, uri, header, body}, the module answers on stdout with
// {status, header, body} (body base64). The SDK (sdk/go, built with
// GOOS=wasip1 GOARCH=wasm) does this, so the same plugin code builds as a
// standalone service or as a .wasm module.
//
// Host functions (module "anasaki"), used by the SDK's p.CMS client:
//
//	call(reqPtr, reqLen u32) u32   request JSON {method, uri, body} for the
//	                               service API (site, mail, posts, users,
//	                               events, kv, fetch) → length of the answer
//	result(ptr u32)                copies the answer {status, body} to ptr

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// Limits of one WASM plugin run.
const (
	wasmMemoryPages   = 4096     // 256 MB max linear memory (64 KiB pages)
	wasmMaxOutput     = 20 << 20 // stdout (the response)
	wasmMaxConcurrent = 8        // simultaneous runs per plugin
)

// wasmEnvelope is the request/response exchanged with a module over stdio.
type wasmEnvelope struct {
	Method string      `json:"method,omitempty"`
	URI    string      `json:"uri,omitempty"`
	Status int         `json:"status,omitempty"`
	Header http.Header `json:"header"`
	Body   []byte      `json:"body"`
}

// wasmHost is the shared wazero runtime (one per process).
type wasmHost struct {
	once    sync.Once
	rt      wazero.Runtime
	initErr error

	mu      sync.Mutex
	modules map[string]*wasmModule // package dir (id/version) → compiled module
}

var wasm = &wasmHost{modules: map[string]*wasmModule{}}

type wasmModule struct {
	id       string
	compiled wazero.CompiledModule
	sem      chan struct{}
}

// callState is what host functions need during one run.
type callState struct {
	m      *Manager
	k      *pkg
	result []byte
}

type callKey struct{}

// init creates the runtime; compiled modules are cached in cacheDir (inside
// PLUGINS_DIR, so the cache survives container upgrades with the volume).
func (h *wasmHost) init(cacheDir string) error {
	h.once.Do(func() {
		ctx := context.Background()
		cfg := wazero.NewRuntimeConfig().WithMemoryLimitPages(wasmMemoryPages).WithCloseOnContextDone(true)
		if cache, err := wazero.NewCompilationCacheWithDir(cacheDir); err == nil {
			cfg = cfg.WithCompilationCache(cache)
		} else {
			log.Printf("plugins: wasm compilation cache disabled: %v", err)
		}
		h.rt = wazero.NewRuntimeWithConfig(ctx, cfg)
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, h.rt); err != nil {
			h.initErr = err
			return
		}
		_, h.initErr = h.rt.NewHostModuleBuilder("anasaki").
			NewFunctionBuilder().WithFunc(hostCall).Export("call").
			NewFunctionBuilder().WithFunc(hostResult).Export("result").
			Instantiate(ctx)
	})
	return h.initErr
}

// module compiles (once per installed copy: see drop) the package's .wasm file.
func (h *wasmHost) module(p *packages, k *pkg) (*wasmModule, error) {
	if err := h.init(filepath.Join(p.root, ".wasm-cache")); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if mod, ok := h.modules[k.dir]; ok {
		return mod, nil
	}
	b, err := os.ReadFile(filepath.Join(k.dir, filepath.FromSlash(k.mf.Service.Module)))
	if err != nil {
		return nil, err
	}
	start := time.Now()
	compiled, err := h.rt.CompileModule(context.Background(), b)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", k.mf.ID, err)
	}
	for dir, old := range h.modules { // drop older versions of the same plugin
		if old.id == k.mf.ID {
			_ = old.compiled.Close(context.Background())
			delete(h.modules, dir)
		}
	}
	mod := &wasmModule{id: k.mf.ID, compiled: compiled, sem: make(chan struct{}, wasmMaxConcurrent)}
	h.modules[k.dir] = mod
	log.Printf("plugin %s: wasm module compiled in %s", k.mf.ID, time.Since(start).Round(time.Millisecond))
	return mod, nil
}

// drop forgets the compiled module of every version of plugin id (after a
// reinstall of the same version, or an uninstall). Runs in progress finish
// with the module they started with.
func (h *wasmHost) drop(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for dir, old := range h.modules {
		if old.id == id {
			_ = old.compiled.Close(context.Background())
			delete(h.modules, dir)
		}
	}
}

// warmUp compiles the active WASM plugins in the background at startup.
func (p *packages) warmUp() {
	for _, k := range p.active() {
		if k.isWasm() {
			if _, err := wasm.module(p, k); err != nil {
				log.Printf("plugin %s: %v", k.mf.ID, err)
			}
		}
	}
}

// limitedBuffer is a stdout sink that refuses to grow past max.
type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errors.New("plugin output too large")
	}
	return b.Buffer.Write(p)
}

type logWriter struct{ id string }

func (l logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			log.Printf("plugin %s: %s", l.id, line)
		}
	}
	return len(p), nil
}

// roundTrip runs the module for one request (the request's context bounds it).
func (h *wasmHost) roundTrip(p *packages, k *pkg, req *http.Request) (*http.Response, error) {
	mod, err := h.module(p, k)
	if err != nil {
		return nil, err
	}
	var body []byte
	if req.Body != nil {
		if body, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
	}
	in, _ := json.Marshal(wasmEnvelope{Method: req.Method, URI: req.URL.RequestURI(), Header: req.Header, Body: body})
	ctx := req.Context()
	select {
	case mod.sem <- struct{}{}:
		defer func() { <-mod.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out := &limitedBuffer{max: wasmMaxOutput}
	cfg := wazero.NewModuleConfig().WithName("").WithStdin(bytes.NewReader(in)).WithStdout(out).
		WithStderr(logWriter{k.mf.ID}).WithSysWalltime().WithSysNanotime().WithSysNanosleep().
		WithRandSource(rand.Reader).WithArgs(k.mf.ID).WithEnv("ANASAKI_PLUGIN_ID", k.mf.ID).WithEnv("ANASAKI_RUNTIME", "wasm")
	st := &callState{m: p.m, k: k}
	inst, err := h.rt.InstantiateModule(context.WithValue(ctx, callKey{}, st), mod.compiled, cfg)
	if inst != nil {
		_ = inst.Close(context.Background())
	}
	var exit *sys.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 0) {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("plugin timed out: %w", ctx.Err())
		}
		return nil, err
	}
	var env wasmEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("plugin answered something that is not a response: %w", err)
	}
	if env.Status == 0 {
		env.Status = http.StatusOK
	}
	// the module's JSON may use any case ("set-cookie"): canonicalize so the
	// header filters (Set-Cookie, X-Anasaki-*, hop-by-hop) always match
	header := http.Header{}
	for k, vs := range env.Header {
		ck := http.CanonicalHeaderKey(k)
		header[ck] = append(header[ck], vs...)
	}
	return &http.Response{StatusCode: env.Status, Header: header, Body: io.NopCloser(bytes.NewReader(env.Body)),
		ContentLength: int64(len(env.Body)), Request: req}, nil
}

// hostCall serves a service-API request from the module (see the package comment).
func hostCall(ctx context.Context, m api.Module, ptr, size uint32) uint32 {
	st, _ := ctx.Value(callKey{}).(*callState)
	raw, ok := m.Memory().Read(ptr, size)
	if st == nil || !ok {
		return 0
	}
	var in struct {
		Method string          `json:"method"`
		URI    string          `json:"uri"`
		Body   json.RawMessage `json:"body"`
	}
	status, body := http.StatusBadRequest, []byte(`{"error":"bad host call"}`)
	if json.Unmarshal(raw, &in) == nil {
		status, body = st.m.serviceCall(st.k, in.Method, in.URI, in.Body)
	}
	st.result, _ = json.Marshal(map[string]any{"status": status, "body": json.RawMessage(validJSON(body))})
	return uint32(len(st.result))
}

// hostResult copies the last hostCall answer into the module's memory.
func hostResult(ctx context.Context, m api.Module, ptr uint32) {
	st, _ := ctx.Value(callKey{}).(*callState)
	if st != nil {
		m.Memory().Write(ptr, st.result)
	}
}

func validJSON(b []byte) []byte {
	if json.Valid(b) {
		return b
	}
	q, _ := json.Marshal(string(b))
	return q
}

// ---------------------------------------------------------------- service API dispatch

// ServiceAPIFunc answers core service-API routes (site, mail, events, posts,
// users) for plugin id; set by the api package.
type ServiceAPIFunc func(id string, scopes map[string]bool, method, uri string, body []byte) (int, []byte)

// SetServiceAPI wires the core service-API routes.
func (m *Manager) SetServiceAPI(fn ServiceAPIFunc) { m.serviceAPI = fn }

// serviceCall answers a service-API call of package k (from a WASM module, or
// over HTTP after VerifyServiceRequest): storage and fetch are handled here,
// the rest by the api package.
func (m *Manager) serviceCall(k *pkg, method, uri string, body []byte) (int, []byte) {
	route, query, _ := strings.Cut(strings.TrimPrefix(uri, "/"), "?")
	route = strings.TrimPrefix(route, "api/service/")
	scopes := map[string]bool{}
	for _, s := range k.mf.Service.API {
		scopes[s] = true
	}
	deny := func(scope string) (int, []byte) {
		return http.StatusForbidden, []byte(`{"error":"the plugin manifest does not request the \"` + scope + `\" api scope"}`)
	}
	switch {
	case strings.HasPrefix(route, "kv/") || route == "kv":
		if !scopes["storage"] {
			return deny("storage")
		}
		return m.kv(k.mf.ID, method, strings.TrimPrefix(route, "kv"), query, body)
	case route == "fetch":
		if !scopes["http"] {
			return deny("http")
		}
		return m.fetch(k, body)
	}
	if m.serviceAPI == nil {
		return http.StatusNotFound, []byte(`{"error":"unknown service api route"}`)
	}
	if query != "" {
		route += "?" + query
	}
	return m.serviceAPI(k.mf.ID, scopes, method, "/"+route, body)
}

// ServiceCall is serviceCall for an HTTP request already verified by
// VerifyServiceRequest (plugin id known).
func (m *Manager) ServiceCall(id, method, uri string, body []byte) (int, []byte) {
	k := m.pkgs.get(id)
	if k == nil || !k.serviceReady() {
		return http.StatusUnauthorized, []byte(`{"error":"plugin not active"}`)
	}
	return m.serviceCall(k, method, uri, body)
}

// ---------------------------------------------------------------- storage (scope "storage")

const kvSchema = `CREATE TABLE IF NOT EXISTS plugin_kv (
  plugin_id  VARCHAR(64)  NOT NULL,
  k          VARCHAR(191) NOT NULL,
  v          MEDIUMTEXT   NOT NULL,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (plugin_id, k)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// kv is a per-plugin key/value store of JSON values:
//
//	GET  kv/get?key=…               → {"key","value","found"}
//	POST kv/set     {key, value}    (value: any JSON, ≤ 1 MB)
//	POST kv/delete  {key}
//	GET  kv/list?prefix=…&limit=…   → {"items":[{"key","value"}]} (keys in order, ≤ 500)
func (m *Manager) kv(id, method, route, query string, body []byte) (int, []byte) {
	db := m.host.Store.DB
	q, _ := parseQuery(query)
	jsonOut := func(v any) (int, []byte) { b, _ := json.Marshal(v); return http.StatusOK, b }
	switch route {
	case "/get":
		var v string
		err := db.QueryRow(`SELECT v FROM plugin_kv WHERE plugin_id=? AND k=?`, id, q["key"]).Scan(&v)
		if err != nil {
			return jsonOut(map[string]any{"key": q["key"], "found": false, "value": nil})
		}
		return jsonOut(map[string]any{"key": q["key"], "found": true, "value": json.RawMessage(v)})
	case "/set":
		var in struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(body, &in) != nil || in.Key == "" || len(in.Key) > 191 || len(in.Value) == 0 || len(in.Value) > 1<<20 {
			return http.StatusBadRequest, []byte(`{"error":"key (≤191) and value (JSON ≤1MB) are required"}`)
		}
		if _, err := db.Exec(`INSERT INTO plugin_kv (plugin_id, k, v, updated_at) VALUES (?,?,?,?)
			ON DUPLICATE KEY UPDATE v=VALUES(v), updated_at=VALUES(updated_at)`, id, in.Key, string(in.Value), time.Now().UTC()); err != nil {
			return http.StatusInternalServerError, []byte(`{"error":"storage failed"}`)
		}
		return http.StatusOK, []byte(`{"ok":true}`)
	case "/delete":
		var in struct {
			Key string `json:"key"`
		}
		_ = json.Unmarshal(body, &in)
		_, _ = db.Exec(`DELETE FROM plugin_kv WHERE plugin_id=? AND k=?`, id, in.Key)
		return http.StatusOK, []byte(`{"ok":true}`)
	case "/list":
		limit, _ := strconv.Atoi(q["limit"])
		if limit < 1 || limit > 500 {
			limit = 100
		}
		rows, err := db.Query(`SELECT k, v FROM plugin_kv WHERE plugin_id=? AND k LIKE ? ORDER BY k LIMIT ?`,
			id, likePrefix(q["prefix"]), limit)
		if err != nil {
			return http.StatusInternalServerError, []byte(`{"error":"storage failed"}`)
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var key, v string
			if rows.Scan(&key, &v) == nil {
				items = append(items, map[string]any{"key": key, "value": json.RawMessage(v)})
			}
		}
		return jsonOut(map[string]any{"items": items})
	}
	return http.StatusNotFound, []byte(`{"error":"unknown kv route"}`)
}

func likePrefix(p string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(p) + "%"
}

func parseQuery(q string) (map[string]string, error) {
	out := map[string]string{}
	vals, err := urlParseQuery(q)
	for k, v := range vals {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, err
}

// ---------------------------------------------------------------- outbound HTTP (scope "http")

// fetch performs an https request to a host listed in service.hosts:
// {method, url, headers, body} → {status, headers, body} (body as text, ≤ 5 MB).
func (m *Manager) fetch(k *pkg, body []byte) (int, []byte) {
	var in struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if json.Unmarshal(body, &in) != nil {
		return http.StatusBadRequest, []byte(`{"error":"bad fetch request"}`)
	}
	u, err := urlParse(in.URL)
	if err != nil || u.Scheme != "https" || !hostAllowed(k.mf.Service.Hosts, u.Hostname()) {
		return http.StatusForbidden, []byte(`{"error":"only https URLs on hosts listed in service.hosts may be fetched"}`)
	}
	if in.Method == "" {
		in.Method = http.MethodGet
	}
	ctx, cancel := contextWithTimeout(15 * time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(in.Method), u.String(), strings.NewReader(in.Body))
	if err != nil {
		return http.StatusBadRequest, []byte(`{"error":"bad fetch request"}`)
	}
	for h, v := range in.Headers {
		req.Header.Set(h, v)
	}
	req.Header.Set("User-Agent", "AnasakiStudio-Plugin/"+k.mf.ID)
	resp, err := publicClient.Do(req)
	if err != nil {
		b, _ := json.Marshal(map[string]string{"error": "fetch failed: " + err.Error()})
		return http.StatusBadGateway, b
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	hdr := map[string]string{}
	for h := range resp.Header {
		hdr[h] = resp.Header.Get(h)
	}
	b, _ := json.Marshal(map[string]any{"status": resp.StatusCode, "headers": hdr, "body": string(data)})
	return http.StatusOK, b
}

func urlParse(s string) (*url.URL, error)        { return url.Parse(s) }
func urlParseQuery(q string) (url.Values, error) { return url.ParseQuery(q) }

// publicClient reaches public hosts only (resolved IPs are checked, so a
// listed host can't be pointed at the private network).
var publicClient = &http.Client{
	Timeout:   20 * time.Second,
	Transport: &http.Transport{Proxy: nil, DialContext: publicDialer(10 * time.Second).DialContext},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" {
			return errors.New("redirect refused")
		}
		return nil
	},
}

func hostAllowed(allowed []string, host string) bool {
	host = strings.ToLower(host)
	for _, a := range allowed {
		a = strings.ToLower(a)
		if host == a || (strings.HasPrefix(a, "*.") && strings.HasSuffix(host, a[1:])) {
			return true
		}
	}
	return false
}
