package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// The CMS package validator accepts and rejects the same JS plugins as the
// Plugin Store's (AnasakiStudioStore pkgfmt/jscode_test.go mirrors these).

func jsFiles(t *testing.T, mod func(m map[string]any), extra map[string]string) map[string][]byte {
	t.Helper()
	m := map[string]any{
		"id": "shop", "name": "Shop", "version": "1.2.0", "minCore": "1.0.0", "runtime": "js", "category": "ecommerce",
		"permissions": []any{"code", "settings"},
		"code": map[string]any{
			"entry":   "code/main.ts",
			"scopes":  []any{"db", "content", "mail", "routes", "jobs", "http"},
			"hosts":   []any{"api.example.com", "*.stripe.com"},
			"routes":  []any{"/shop", "/cart"},
			"library": "code/lib/index.ts",
			"exports": []any{"cartOf", "createOrder"},
			"hooks":   map[string]any{"filters": []any{"shop.price"}, "events": []any{"shop.order.paid"}},
		},
		"requires": map[string]any{"money": "^1.0.0", "tax-tools": ">=1.2 <2"},
	}
	if mod != nil {
		mod(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"manifest.json":              b,
		"code/main.ts":               []byte(`import { money } from 'money'; site.route('GET /shop', () => 'hi')`),
		"code/lib/index.ts":          []byte(`export const x = 1`),
		"code/data.json":             []byte(`{}`),
		"code/README.md":             []byte(`# notes`),
		"code/util.mjs":              []byte(`export default 1`),
		"migrations/001_init.ts":     []byte(`export default migration({ up() {} })`),
		"migrations/notes/readme.md": []byte(`x`),
		"README.md":                  []byte("# Shop"),
	}
	for k, v := range extra {
		if v == "\x00delete" {
			delete(files, k)
		} else {
			files[k] = []byte(v)
		}
	}
	return files
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, b := range files {
		w, _ := zw.Create(n)
		_, _ = w.Write(b)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func codeOf(m map[string]any) map[string]any { return m["code"].(map[string]any) }

func TestJSPluginValid(t *testing.T) {
	mf, files, err := ParsePackage(zipOf(t, jsFiles(t, nil, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if mf.Runtime != RuntimeJS || mf.Code.Entry != "code/main.ts" || mf.Requires["tax-tools"] != ">=1.2 <2" {
		t.Fatalf("manifest = %+v", mf)
	}
	if len(codeFilesOf(files)) != 7 {
		t.Fatalf("code files = %v", codeFilesOf(files))
	}
	// one top folder (compress folder) works too
	nested := map[string][]byte{}
	for k, v := range jsFiles(t, nil, nil) {
		nested["shop/"+k] = v
	}
	if _, _, err := ParsePackage(zipOf(t, nested)); err != nil {
		t.Fatalf("nested: %v", err)
	}
	min := jsFiles(t, func(m map[string]any) {
		m["code"] = map[string]any{"entry": "code/main.js", "scopes": []any{"db"}}
		delete(m, "requires")
	}, map[string]string{"code/main.js": "1"})
	if _, _, err := ParsePackage(zipOf(t, min)); err != nil {
		t.Fatalf("minimal: %v", err)
	}
}

func TestJSPluginRejects(t *testing.T) {
	type tc struct {
		mod   func(m map[string]any)
		extra map[string]string
		want  string
	}
	cases := map[string]tc{
		"unknown runtime":    {mod: func(m map[string]any) { m["runtime"] = "python" }, want: "runtime"},
		"missing code":       {mod: func(m map[string]any) { delete(m, "code") }, want: `"code"`},
		"missing entry":      {mod: func(m map[string]any) { delete(codeOf(m), "entry") }, want: "code.entry"},
		"entry file missing": {extra: map[string]string{"code/main.ts": "\x00delete"}, want: "code/main.ts"},
		"entry outside code": {mod: func(m map[string]any) { codeOf(m)["entry"] = "main.ts" }, extra: map[string]string{"main.ts": "x"}, want: ".ts"},
		"entry bad ext":      {mod: func(m map[string]any) { codeOf(m)["entry"] = "code/data.json" }, want: "code.entry"},
		"no code permission": {mod: func(m map[string]any) { m["permissions"] = []any{"settings"} }, want: `"code"`},
		"bad scope":          {mod: func(m map[string]any) { codeOf(m)["scopes"] = []any{"db", "root"} }, want: "root"},
		"duplicate scope":    {mod: func(m map[string]any) { codeOf(m)["scopes"] = []any{"db", "db"} }, want: "twice"},
		"bad host":           {mod: func(m map[string]any) { codeOf(m)["hosts"] = []any{"https://api.example.com"} }, want: "host"},
		"host uppercase":     {mod: func(m map[string]any) { codeOf(m)["hosts"] = []any{"API.example.com"} }, want: "host"},
		"hosts without http": {mod: func(m map[string]any) { codeOf(m)["scopes"] = []any{"db", "routes"} }, want: `"http"`},
		"reserved route":     {mod: func(m map[string]any) { codeOf(m)["routes"] = []any{"/admin"} }, want: "reserved"},
		"reserved route _a":  {mod: func(m map[string]any) { codeOf(m)["routes"] = []any{"/_a/x"} }, want: "route"},
		"reserved route api": {mod: func(m map[string]any) { codeOf(m)["routes"] = []any{"/api/shop"} }, want: "reserved"},
		"reserved blog":      {mod: func(m map[string]any) { codeOf(m)["routes"] = []any{"/blog"} }, want: "reserved"},
		"bad route":          {mod: func(m map[string]any) { codeOf(m)["routes"] = []any{"shop"} }, want: "route"},
		"duplicate route":    {mod: func(m map[string]any) { codeOf(m)["routes"] = []any{"/shop", "/shop"} }, want: "twice"},
		"routes w/o scope":   {mod: func(m map[string]any) { codeOf(m)["scopes"] = []any{"db", "http"} }, want: `"routes"`},
		"library missing":    {extra: map[string]string{"code/lib/index.ts": "\x00delete"}, want: "library"},
		"library outside":    {mod: func(m map[string]any) { codeOf(m)["library"] = "lib/index.ts" }, extra: map[string]string{"lib/index.js": "x"}, want: "library"},
		"bad export":         {mod: func(m map[string]any) { codeOf(m)["exports"] = []any{"create-order"} }, want: "export"},
		"duplicate export":   {mod: func(m map[string]any) { codeOf(m)["exports"] = []any{"a", "a"} }, want: "twice"},
		"hook other prefix":  {mod: func(m map[string]any) { codeOf(m)["hooks"] = map[string]any{"filters": []any{"site.price"}} }, want: "shop."},
		"hook bad name":      {mod: func(m map[string]any) { codeOf(m)["hooks"] = map[string]any{"events": []any{"shop."}} }, want: "hook"},
		"requires bad id":    {mod: func(m map[string]any) { m["requires"] = map[string]any{"Money": "^1.0.0"} }, want: "requires"},
		"requires self":      {mod: func(m map[string]any) { m["requires"] = map[string]any{"shop": "^1.0.0"} }, want: "requires"},
		"requires builtin":   {mod: func(m map[string]any) { m["requires"] = map[string]any{"backup": "^1.0.0"} }, want: "requires"},
		"bad range":          {mod: func(m map[string]any) { m["requires"] = map[string]any{"money": "latest"} }, want: "money"},
		"bad range 2":        {mod: func(m map[string]any) { m["requires"] = map[string]any{"money": "^1.0.0.0"} }, want: "money"},
		"empty range":        {mod: func(m map[string]any) { m["requires"] = map[string]any{"money": ""} }, want: "money"},
		"bad code ext":       {extra: map[string]string{"code/style.css": "a{}"}, want: "code/"},
		"bad migration ext":  {extra: map[string]string{"migrations/002.html": "x"}, want: "migrations/"},
		"bad code segment":   {extra: map[string]string{"code/my file.ts": "x"}, want: "A-Z"},
		"case duplicate":     {extra: map[string]string{"code/Main.ts": "x"}, want: "duplicate"},
		"code file too big":  {extra: map[string]string{"code/big.ts": strings.Repeat("a", MaxCodeFileSize+1)}, want: "512 KB"},
		"migrations no db":   {mod: func(m map[string]any) { codeOf(m)["scopes"] = []any{"routes", "http"} }, want: `"db"`},
		"js with service": {mod: func(m map[string]any) {
			m["permissions"] = []any{"code", "service"}
			m["service"] = map[string]any{"public": []any{"/svc"}}
		}, want: "service"},
		"code section non-js": {mod: func(m map[string]any) { delete(m, "runtime") }, want: `"code"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := ParsePackage(zipOf(t, jsFiles(t, c.mod, c.extra)))
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
	many := map[string]string{}
	for i := 0; i <= MaxCodeFiles; i++ {
		many["code/f_"+strconv.Itoa(i)+".ts"] = "1"
	}
	if _, _, err := ParsePackage(zipOf(t, jsFiles(t, nil, many))); err == nil || !strings.Contains(err.Error(), "too many code files") {
		t.Fatalf("too many code files: %v", err)
	}
	big := map[string]string{}
	for i := 0; i < 5; i++ {
		big["code/big"+strconv.Itoa(i)+".ts"] = strings.Repeat("a", 500<<10)
	}
	if _, _, err := ParsePackage(zipOf(t, jsFiles(t, nil, big))); err == nil || !strings.Contains(err.Error(), "2 MB") {
		t.Fatalf("too much code: %v", err)
	}
}

func TestNonJSPackagesRejectCode(t *testing.T) {
	with := func(name, body string) map[string][]byte {
		f := map[string][]byte{"manifest.json": []byte(`{"id":"plain","name":"P","version":"1.0.0","category":"utilities","permissions":[]}`)}
		f[name] = []byte(body)
		return f
	}
	for _, p := range []string{"code/x.ts", "assets/x.ts", "code/x.mjs"} {
		if _, _, err := ParsePackage(zipOf(t, with(p, "1"))); err == nil {
			t.Errorf("%s in a non-JS package must be rejected", p)
		}
	}
	if _, _, err := ParsePackage(zipOf(t, with("manifest.json",
		`{"id":"plain","name":"P","version":"1.0.0","category":"utilities","permissions":["code"]}`))); err == nil {
		t.Fatal(`permission "code" without runtime js must be rejected`)
	}
	if _, _, err := ParsePackage(zipOf(t, with("manifest.json",
		`{"id":"plain","name":"P","version":"1.0.0","category":"utilities","requires":{"money":"~1.2"}}`))); err != nil {
		t.Fatalf("requires on a static package: %v", err)
	}
}

func TestRanges(t *testing.T) {
	cases := []struct {
		rng     string
		yes, no []string
	}{
		{"^1.2.3", []string{"1.2.3", "1.9.0"}, []string{"1.2.2", "2.0.0", "2.0.0-beta", "1.3.0-beta"}},
		{"^0.2.3", []string{"0.2.3", "0.2.9"}, []string{"0.3.0", "0.2.2"}},
		{"^0.0.3", []string{"0.0.3"}, []string{"0.0.4"}},
		{"^1.2", []string{"1.2.0", "1.5.1"}, []string{"1.1.9", "2.0.0"}},
		{"^1", []string{"1.0.0", "1.9.9"}, []string{"2.0.0", "0.9.0"}},
		{"^0.x", []string{"0.0.1", "0.9.0"}, []string{"1.0.0"}},
		{"~1.2.3", []string{"1.2.3", "1.2.9"}, []string{"1.3.0", "1.2.2"}},
		{"~1.2", []string{"1.2.0", "1.2.9"}, []string{"1.3.0"}},
		{"~1", []string{"1.0.0", "1.9.0"}, []string{"2.0.0"}},
		{"1.x", []string{"1.0.0", "1.99.0"}, []string{"2.0.0", "0.1.0"}},
		{"1.2.x", []string{"1.2.0", "1.2.7"}, []string{"1.3.0"}},
		{"1.2.*", []string{"1.2.5"}, []string{"1.1.0"}},
		{"*", []string{"0.0.1", "9.9.9"}, []string{"1.0.0-rc.1"}},
		{"x", []string{"3.0.0"}, nil},
		{"1.2.3", []string{"1.2.3"}, []string{"1.2.4"}},
		{"=1.2.3", []string{"1.2.3"}, []string{"1.2.4"}},
		{">=1.0.0 <2.0.0", []string{"1.0.0", "1.9.9"}, []string{"2.0.0", "0.9.9"}},
		{">= 1.0.0 < 2", []string{"1.5.0"}, []string{"2.0.0"}},
		{">1.2.3", []string{"1.2.4"}, []string{"1.2.3"}},
		{">1.2", []string{"1.3.0"}, []string{"1.2.9"}},
		{"<=1.2", []string{"1.2.9"}, []string{"1.3.0"}},
		{"<=1.2.3", []string{"1.2.3"}, []string{"1.2.4"}},
		{"<1.0.0", []string{"0.9.0"}, []string{"1.0.0"}},
		{"^1.0.0 || ^3.0.0", []string{"1.1.0", "3.2.0"}, []string{"2.0.0"}},
		{"^1.0.0-beta.2", []string{"1.0.0-beta.3", "1.0.0", "1.4.0"}, []string{"1.0.0-beta.1", "1.1.0-beta.1"}},
	}
	for _, c := range cases {
		if _, err := ParseRange(c.rng); err != nil {
			t.Errorf("ParseRange(%q): %v", c.rng, err)
			continue
		}
		for _, v := range c.yes {
			if !SatisfiesRange(v, c.rng) {
				t.Errorf("%q should match %s", c.rng, v)
			}
		}
		for _, v := range c.no {
			if SatisfiesRange(v, c.rng) {
				t.Errorf("%q should not match %s", c.rng, v)
			}
		}
	}
	for _, bad := range []string{"", "latest", "^", "1.2.3.4", ">=", "1.0 - 2.0", "^1.0.0 ||", "1.2-beta", "v1.0.0",
		strings.Repeat("1", 101)} {
		if _, err := ParseRange(bad); err == nil {
			t.Errorf("ParseRange(%q) should fail", bad)
		}
	}
}

// Upgrades ask again for new code scopes and hosts.
func TestReconsentCodeScopes(t *testing.T) {
	old := &pkg{version: "1.0.0", mf: &Manifest{Permissions: []string{"code"}, Code: &ManifestCode{Scopes: []string{"db"}}}}
	rc := reconsentFor(old, &Manifest{Permissions: []string{"code"}, Code: &ManifestCode{Scopes: []string{"db", "mail"}, Hosts: []string{"api.x.com"}}})
	if rc == nil || strings.Join(rc.Scopes, ",") != "mail" || strings.Join(rc.Hosts, ",") != "api.x.com" {
		t.Fatalf("reconsent = %+v", rc)
	}
	if reconsentFor(old, &Manifest{Permissions: []string{"code"}, Code: &ManifestCode{Scopes: []string{"db"}}}) != nil {
		t.Fatal("same scopes must not need consent")
	}
}
