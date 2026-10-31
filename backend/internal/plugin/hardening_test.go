package plugin

import (
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/db"
	"github.com/anasaki/studio/backend/internal/store"
)

// ---------------------------------------------------------------- ScopeCSS

func TestScopeCSSCommentAndStringEscapes(t *testing.T) {
	const s = ".s"
	cases := []struct {
		in      string
		want    []string // substrings of the output
		notWant []string
	}{
		// a comment hiding a "{" must not end the prelude early
		{in: `a/*{*/,body{color:red}`, want: []string{`.s a,.s{color:red}`}, notWant: []string{"body", "/*"}},
		{in: `a/**/,/**/html{x:y}`, want: []string{`.s a,.s{x:y}`}, notWant: []string{"html"}},
		// braces inside strings are not structure
		{in: `a{content:"}"} body{x:y}`, want: []string{`.s a{content:"}"}`, ".s{x:y}"}, notWant: []string{"body"}},
		{in: `a{content:'{'} p{x:y}`, want: []string{`.s a{content:'{'}`, ".s p{x:y}"}},
		{in: `[title="a{b"],i{x:y}`, want: []string{`.s [title="a{b"],.s i{x:y}`}},
		{in: `a{content:"/*"} b{c:d}`, want: []string{`.s a{content:"/*"}`, ".s b{c:d}"}},
		// an escaped brace is part of the selector
		{in: `a\{,body{color:red}`, want: []string{`.s a\{,.s{color:red}`}, notWant: []string{",body"}},
		// unbalanced brackets: the browser would merge following rules into it
		{in: `:is(a{} ) , body{color:red}`, notWant: []string{"body", ":is("}},
		{in: `a[x{} ] , body{color:red}`, notWant: []string{"body"}},
		// unterminated string is closed at the end of the line
		{in: "a{content:\"x\n} body{color:red}", want: []string{".s a{content:\"x\"", ".s{color:red}"}, notWant: []string{"body"}},
		// unterminated comment swallows the rest
		{in: `p{x:y} /* body{color:red}`, want: []string{".s p{x:y}"}, notWant: []string{"body"}},
		// nested at-rules stay scoped
		{in: `@media (max-width:600px){ @supports (display:grid){ body{c:d} a{content:"}{"} } p{e:f} }`,
			want:    []string{`@media (max-width:600px){@supports (display:grid){.s{c:d}`, `.s a{content:"}{"}`, `.s p{e:f}`},
			notWant: []string{"body"}},
		{in: `@media screen{a/*}*/,body{x:y}}`, want: []string{"@media screen{.s a,.s{x:y}"}, notWant: []string{"body"}},
	}
	for _, c := range cases {
		got := ScopeCSS(c.in, s)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("ScopeCSS(%q): missing %q in\n%s", c.in, w, got)
			}
		}
		for _, w := range c.notWant {
			if strings.Contains(got, w) {
				t.Errorf("ScopeCSS(%q): unexpected %q in\n%s", c.in, w, got)
			}
		}
	}
}

// ---------------------------------------------------------------- signatures

// Same vector as sdk/go/anasaki: both sides must sign v2 identically.
const sigV2Vector = "f2f8040e88affb8cd010613668ff2b579f196be84cf27345bf3fe875bc718a8a"

func TestServiceSignatureV2(t *testing.T) {
	got := ServiceSignatureV2("s3cret", 1700000000, "00112233445566778899aabbccddeeff", "post", "/admin/x?y=1",
		[]byte(`{"a":1}`), "hello", "eyJpZCI6MX0=", "/ext-ui/hello/t/")
	if got != sigV2Vector {
		t.Fatalf("v2 signature %s, want %s", got, sigV2Vector)
	}
}

func TestVerifyServiceSignatureV2(t *testing.T) {
	body := []byte(`{"x":1}`)
	r := httptest.NewRequest("POST", "/admin/save?a=b", strings.NewReader(string(body)))
	r.Header.Set(HeaderPlugin, "v2-test")
	r.Header.Set(HeaderUser, base64.StdEncoding.EncodeToString([]byte(`{"id":1,"role":"editor"}`)))
	signServiceRequest("k", r, body)
	if r.Header.Get(HeaderSignature) == "" || r.Header.Get(HeaderNonce) == "" {
		t.Fatal("v1 signature or nonce missing")
	}
	if err := VerifyServiceSignature("k", r, body); err != nil {
		t.Fatalf("valid v2 request refused: %v", err)
	}
	if VerifyServiceSignature("k", r, body) == nil {
		t.Fatal("replayed nonce accepted")
	}

	// the user header is covered
	signServiceRequest("k", r, body)
	r.Header.Set(HeaderUser, base64.StdEncoding.EncodeToString([]byte(`{"id":1,"role":"administrator"}`)))
	if VerifyServiceSignature("k", r, body) == nil {
		t.Fatal("tampered user header accepted")
	}

	// v1 only (older SDK) still verifies
	r2 := httptest.NewRequest("GET", "/api/service/site", nil)
	ts := time.Now().Unix()
	r2.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	r2.Header.Set(HeaderSignature, ServiceSignature("k", ts, "GET", "/api/service/site", nil))
	if err := VerifyServiceSignature("k", r2, nil); err != nil {
		t.Fatalf("v1 request refused: %v", err)
	}
}

func TestNonceCacheBounded(t *testing.T) {
	c := newNonceCache(3)
	now := time.Now()
	for i := 0; i < 5; i++ {
		if !c.add(fmt.Sprint(i), now) {
			t.Fatalf("fresh nonce %d refused", i)
		}
	}
	if len(c.seen) > 3 || len(c.order) > 3 {
		t.Fatalf("cache grew past its bound: %d", len(c.seen))
	}
	if c.add("4", now) {
		t.Fatal("recent duplicate accepted")
	}
	if !c.add("4", now.Add(11*time.Minute)) {
		t.Fatal("expired nonce still remembered")
	}
}

// svcManager has one active, connected service plugin and no database.
func svcManager(public ...string) *Manager {
	m := &Manager{host: &Host{}, byID: map[string]*entry{}}
	m.pkgs = &packages{m: m, files: map[string]string{}, list: []*pkg{{
		mf: &Manifest{ID: "svc-test", Name: "Svc", Permissions: []string{"service"},
			Service: &ManifestService{API: []string{"site"}, Public: public}},
		version: "1.0.0", active: true, serviceURL: "http://svc:9000", secret: "k"}}}
	return m
}

func TestVerifyServiceRequestNoDowngrade(t *testing.T) {
	m := svcManager()
	mk := func(v2 bool) error {
		r := httptest.NewRequest("GET", "/api/service/site", nil)
		r.Header.Set(HeaderPlugin, "svc-test")
		if v2 {
			signServiceRequest("k", r, nil)
		} else {
			ts := time.Now().Unix()
			r.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
			r.Header.Set(HeaderSignature, ServiceSignature("k", ts, "GET", "/api/service/site", nil))
		}
		_, _, err := m.VerifyServiceRequest(r, nil)
		return err
	}
	v2Plugins.Delete("svc-test")
	if err := mk(false); err != nil {
		t.Fatalf("v1 refused before any v2: %v", err)
	}
	if err := mk(true); err != nil {
		t.Fatalf("v2 refused: %v", err)
	}
	if mk(false) == nil {
		t.Fatal("v1-only request accepted after the service switched to v2")
	}
	v2Plugins.Delete("svc-test")
}

// ---------------------------------------------------------------- public paths

func TestPublicPathsNeverShadowSite(t *testing.T) {
	m := svcManager("/blog", "/shop")
	p := m.pkgs
	// no database: the default bases apply
	if got := p.publicConflicts(&ManifestService{Public: []string{"/shop", "/blog/x", "/category", "/admin"}}); !reflect.DeepEqual(got, []string{"/blog/x", "/category", "/admin"}) {
		t.Fatalf("conflicts: %v", got)
	}
	if k := p.publicService("/blog/post"); k != nil {
		t.Fatal("a service took over the blog base")
	}
	if k := p.publicService("/shop/cart"); k == nil {
		t.Fatal("service path not served")
	}
	// custom bases (as read from the options)
	p.segMu.Lock()
	p.segs, p.segsAt = map[string]bool{"backoffice": true, "shop": true, "topics": true}, time.Now()
	p.segMu.Unlock()
	if p.publicService("/shop") != nil || p.publicService("/blog/x") == nil {
		t.Fatal("custom bases not honoured")
	}
}

// ---------------------------------------------------------------- re-consent

func TestReconsentFor(t *testing.T) {
	old := &pkg{version: "1.0.0", mf: &Manifest{Permissions: []string{"head", "service"},
		Service: &ManifestService{API: []string{"site"}, Hosts: []string{"*.stripe.com"}}}}
	same := &Manifest{Permissions: []string{"service", "head"},
		Service: &ManifestService{API: []string{"site"}, Hosts: []string{"api.stripe.com"}}}
	if rc := reconsentFor(old, same); rc != nil {
		t.Fatalf("nothing new, got %+v", rc)
	}
	more := &Manifest{Permissions: []string{"head", "service", "bodyEnd"},
		Service: &ManifestService{API: []string{"site", "mail"}, Hosts: []string{"*.stripe.com", "evil.example"}}}
	rc := reconsentFor(old, more)
	if rc == nil || rc.FromVersion != "1.0.0" || !reflect.DeepEqual(rc.Permissions, []string{"bodyEnd"}) ||
		!reflect.DeepEqual(rc.API, []string{"mail"}) || !reflect.DeepEqual(rc.Hosts, []string{"evil.example"}) {
		t.Fatalf("reconsent: %+v", rc)
	}
	// still pending after a further upgrade that adds nothing
	pending := &pkg{version: "1.1.0", mf: more, reconsent: rc}
	rc2 := reconsentFor(pending, more)
	if rc2 == nil || rc2.FromVersion != "1.0.0" || !reflect.DeepEqual(rc2.API, []string{"mail"}) {
		t.Fatalf("pending grants lost: %+v", rc2)
	}
	// a wildcard is not covered by a narrower approval
	if rc := reconsentFor(&pkg{mf: same}, &Manifest{Permissions: same.Permissions,
		Service: &ManifestService{API: []string{"site"}, Hosts: []string{"*.stripe.com"}}}); rc == nil {
		t.Fatal("wildcard host widened without consent")
	}
}

// ---------------------------------------------------------------- install lifecycle (needs TEST_DB_DSN)

func testManager(t *testing.T) *Manager {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set (use a throw-away database)")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Skip("test database unreachable: ", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLUGINS_DIR", t.TempDir())
	m, err := NewManager(store.New(conn), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DELETE FROM plugin_packages WHERE id LIKE 'zz-test-%'`, `DELETE FROM plugin_kv WHERE plugin_id LIKE 'zz-test-%'`} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func lifecycleZip(t *testing.T, version, perms, css string) []byte {
	mf := `{"id":"zz-test-pkg","name":"ZZ","version":"` + version + `","permissions":[` + perms + `],"head":["a.css"]`
	files := []zf{{name: "a.css", body: css}}
	if strings.Contains(perms, "bodyEnd") {
		mf += `,"bodyEnd":"end.html"`
		files = append(files, zf{name: "end.html", body: "<i>end</i>"})
	}
	return mkzip(t, append(files, zf{name: "manifest.json", body: mf + "}"})...)
}

func TestPackageLifecycle(t *testing.T) {
	m := testManager(t)
	p := m.pkgs
	const id = "zz-test-pkg"
	if _, err := m.InstallPackage(lifecycleZip(t, "1.0.0", `"head"`, "a{color:red}"), "upload", ""); err != nil {
		t.Fatal(err)
	}
	if in, err := m.SetPackageActive(id, true); err != nil || !in.Active {
		t.Fatalf("activate: %v %+v", err, in)
	}
	if got := p.file(p.get(id), "a.css"); got != "a{color:red}" {
		t.Fatalf("file: %q", got)
	}

	// same-version reinstall: no stale cached file
	if _, err := m.InstallPackage(lifecycleZip(t, "1.0.0", `"head"`, "a{color:blue}"), "upload", ""); err != nil {
		t.Fatal(err)
	}
	if got := p.file(p.get(id), "a.css"); got != "a{color:blue}" {
		t.Fatalf("stale file after reinstall: %q", got)
	}

	// upgrade with nothing new stays active
	in, err := m.InstallPackage(lifecycleZip(t, "1.0.1", `"head"`, "x{}"), "upload", "")
	if err != nil || !in.Active || in.Reconsent != nil {
		t.Fatalf("plain upgrade: %v %+v", err, in)
	}

	// upgrade with a new permission is switched off until re-approved
	in, err = m.InstallPackage(lifecycleZip(t, "1.1.0", `"head","bodyEnd"`, "x{}"), "upload", "")
	if err != nil {
		t.Fatal(err)
	}
	if in.Active || in.Reconsent == nil || !reflect.DeepEqual(in.Reconsent.Permissions, []string{"bodyEnd"}) ||
		in.Reconsent.FromVersion != "1.0.1" {
		t.Fatalf("upgrade with new permission: %+v %+v", in, in.Reconsent)
	}
	if in, err = m.SetPackageActive(id, true); err != nil || !in.Active || in.Reconsent != nil {
		t.Fatalf("re-approve: %v %+v", err, in)
	}
	ents, _ := os.ReadDir(filepath.Join(p.root, id))
	if len(ents) != 1 || ents[0].Name() != "1.1.0" {
		t.Fatalf("old versions left behind: %v", ents)
	}

	// concurrent installs never fail or leave a broken tree
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := m.InstallPackage(lifecycleZip(t, "1.2."+strconv.Itoa(i%3), `"head","bodyEnd"`, "c{}"), "upload", "")
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent install: %v", err)
		}
	}
	k := p.get(id)
	if _, err := os.Stat(filepath.Join(k.dir, "manifest.json")); err != nil {
		t.Fatalf("installed version %s has no files: %v", k.version, err)
	}

	// uninstall removes the stored data too
	if _, err := p.db.Exec(`INSERT INTO plugin_kv (plugin_id, k, v) VALUES (?, 'a', '1')`, id); err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePackage(id); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM plugin_kv WHERE plugin_id=?`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("plugin_kv rows left: %d", n)
	}
	if _, err := os.Stat(filepath.Join(p.root, id)); !os.IsNotExist(err) {
		t.Fatalf("files left: %v", err)
	}
	p.mu.Lock()
	for key := range p.files {
		if strings.Contains(key, id) {
			t.Errorf("cached file left: %s", key)
		}
	}
	p.mu.Unlock()
}
