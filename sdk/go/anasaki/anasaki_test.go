package anasaki

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Same vector as the CMS (backend/internal/plugin/service_test.go).
const sigVector = "40e6b9f8aed3e8e9b9eda1a1f7ed7239126bcf97f717cc17196f45d02a4b6c8d"

func TestSign(t *testing.T) {
	if got := Sign("s3cret", 1700000000, "post", "/blocks/x?y=1", []byte(`{"a":1}`)); got != sigVector {
		t.Fatalf("signature %s, want %s", got, sigVector)
	}
}

func signed(secret, method, uri, body string) *http.Request {
	r := httptest.NewRequest(method, uri, strings.NewReader(body))
	ts := time.Now().Unix()
	r.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	r.Header.Set(HeaderSignature, Sign(secret, ts, method, uri, []byte(body)))
	return r
}

func TestPluginRouting(t *testing.T) {
	p := New(Config{ID: "x", Secret: "k", Name: "X", Version: "1.0.0"})
	p.Public("GET /hello", func(w http.ResponseWriter, r *http.Request) { Page(w, 200, "Hi there", "<h1>hi</h1>") })
	p.Block("b", func(in BlockInput) BlockOutput { return BlockOutput{HTML: "<b>" + in.Values["n"].(string) + "</b>"} })

	w := httptest.NewRecorder()
	p.ServeHTTP(w, signed("k", "GET", "/public/hello", ""))
	if w.Code != 200 || w.Header().Get(HeaderLayout) != "page" || w.Header().Get(HeaderTitle) != "Hi+there" {
		t.Fatalf("public page: %d %v", w.Code, w.Header())
	}
	w = httptest.NewRecorder()
	p.ServeHTTP(w, signed("k", "POST", "/blocks/b", `{"values":{"n":"ok"}}`))
	var out BlockOutput
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.HTML != "<b>ok</b>" {
		t.Fatalf("block: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	p.ServeHTTP(w, signed("wrong", "GET", "/health", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned request served: %d", w.Code)
	}
}

// Same vector as the CMS (backend/internal/plugin/hardening_test.go).
const sigV2Vector = "f2f8040e88affb8cd010613668ff2b579f196be84cf27345bf3fe875bc718a8a"

func TestSignV2(t *testing.T) {
	got := SignV2("s3cret", 1700000000, "00112233445566778899aabbccddeeff", "post", "/admin/x?y=1",
		[]byte(`{"a":1}`), "hello", "eyJpZCI6MX0=", "/ext-ui/hello/t/")
	if got != sigV2Vector {
		t.Fatalf("v2 signature %s, want %s", got, sigV2Vector)
	}
}

func signedV2(secret, method, uri, body, user string) *http.Request {
	r := httptest.NewRequest(method, uri, strings.NewReader(body))
	r.Header.Set(HeaderPlugin, "x")
	if user != "" {
		r.Header.Set(HeaderUser, user)
	}
	SignRequest(secret, r, []byte(body))
	return r
}

func TestVerifyV2(t *testing.T) {
	sawV2.Store(false)
	defer sawV2.Store(false)
	p := New(Config{ID: "x", Secret: "k", Name: "X", Version: "1.0.0"})
	var role string
	p.Admin("GET /me", func(w http.ResponseWriter, r *http.Request) { role = UserFrom(r).Role })
	user := base64.StdEncoding.EncodeToString([]byte(`{"id":1,"role":"editor"}`))

	// a v1-only request (older CMS) works until the CMS has used v2
	w := httptest.NewRecorder()
	p.ServeHTTP(w, signed("k", "GET", "/health", ""))
	if w.Code != 200 {
		t.Fatalf("v1 request refused: %d", w.Code)
	}

	r := signedV2("k", "GET", "/admin/me", "", user)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != 200 || role != "editor" {
		t.Fatalf("v2 request: %d %q", w.Code, role)
	}
	// replay of the very same request
	r.Body = http.NoBody
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("replayed request served: %d", w.Code)
	}
	// the user header is signed
	r = signedV2("k", "GET", "/admin/me", "", user)
	r.Header.Set(HeaderUser, base64.StdEncoding.EncodeToString([]byte(`{"id":1,"role":"administrator"}`)))
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered user served: %d", w.Code)
	}
	// no downgrade to v1 once v2 has been seen
	w = httptest.NewRecorder()
	p.ServeHTTP(w, signed("k", "GET", "/health", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("v1 request served after v2: %d", w.Code)
	}
}

func TestRequireV2(t *testing.T) {
	sawV2.Store(false)
	defer sawV2.Store(false)
	p := New(Config{ID: "x", Secret: "k", RequireV2: true})
	w := httptest.NewRecorder()
	p.ServeHTTP(w, signed("k", "GET", "/health", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("v1 request served with RequireV2: %d", w.Code)
	}
	w = httptest.NewRecorder()
	p.ServeHTTP(w, signedV2("k", "GET", "/health", "", ""))
	if w.Code != 200 {
		t.Fatalf("v2 request refused: %d", w.Code)
	}
}
