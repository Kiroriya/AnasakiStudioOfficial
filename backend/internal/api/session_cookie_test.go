package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/auth"
)

// 2.3.0 (security assessment 45): the admin app's session is an HttpOnly cookie, never a token in the answer
// or in page storage; the cookie counts only with the X-Anasaki-Session header, so another site can't use it.
func TestSessionCookie(t *testing.T) {
	c, _, _ := newSecAPI(t)
	setupSite(t, c)

	send := func(method, path string, body any, cookie *http.Cookie, header bool) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://site.example")
		if header {
			req.Header.Set(auth.SessionHeader, "cookie")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		return rec
	}
	sessionOf := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == auth.SessionCookie {
				return ck
			}
		}
		return nil
	}

	rec := send("POST", "/api/v1/auth/login", map[string]any{"email": "owner@x.io", "password": "secret123"}, nil, true)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	ck := sessionOf(rec)
	if rec.Code != 200 || out["token"] != "" || ck == nil || ck.Value == "" {
		t.Fatalf("admin-app login: %d %v cookie %+v", rec.Code, out, ck)
	}
	if !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" {
		t.Fatalf("session cookie attributes: %+v", ck)
	}

	if rec := send("GET", "/api/v1/auth/me", nil, ck, true); rec.Code != 200 {
		t.Fatalf("cookie + header: %d %s", rec.Code, rec.Body)
	}
	// the cookie alone — what a cross-site form or image would carry — is not a session
	if rec := send("GET", "/api/v1/auth/me", nil, ck, false); rec.Code != 401 {
		t.Fatalf("cookie without the header was accepted: %d", rec.Code)
	}
	if rec := send("GET", "/api/v1/auth/me", nil, nil, true); rec.Code != 401 {
		t.Fatalf("header without a cookie: %d", rec.Code)
	}

	// refresh renews the cookie, still without a token in the answer
	rec = send("POST", "/api/v1/auth/refresh", nil, ck, true)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	fresh := sessionOf(rec)
	if rec.Code != 200 || out["token"] != "" || fresh == nil || fresh.Value == ck.Value {
		t.Fatalf("refresh: %d %v %+v", rec.Code, out, fresh)
	}

	// logout revokes the session and removes the cookie
	rec = send("POST", "/api/v1/auth/logout", nil, fresh, true)
	gone := sessionOf(rec)
	if rec.Code != 204 || gone == nil || gone.MaxAge >= 0 {
		t.Fatalf("logout: %d %+v", rec.Code, gone)
	}
	if rec := send("GET", "/api/v1/auth/me", nil, fresh, true); rec.Code != 401 {
		t.Fatalf("cookie still works after logout: %d", rec.Code)
	}

	// API clients and the CLI keep getting a bearer token
	code, b := c.do("POST", "/api/v1/auth/login", map[string]any{"email": "owner@x.io", "password": "secret123"})
	if tok, _ := b["token"].(string); code != 200 || tok == "" {
		t.Fatalf("API login: %d %v", code, b)
	}
}

// Security assessment 6: decode errors never show Go type names.
func TestJSONErrTextHidesGoTypes(t *testing.T) {
	var v struct{ N int }
	for _, body := range []string{`{"N":"x"}`, `[1]`, `{"N":1}{`, `{"N":`} {
		err := json.Unmarshal([]byte(body), &v)
		if err == nil {
			continue
		}
		if got := jsonErrText(err); strings.Contains(got, "Go ") || strings.Contains(got, "json:") || strings.Contains(got, "struct") {
			t.Errorf("%s → %q", body, got)
		}
	}
}
