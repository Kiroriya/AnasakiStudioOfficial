package plugin

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestRenderTemplateEscapes(t *testing.T) {
	d := TemplateData{
		Settings: Values{"text": `<script>alert("x")</script>`, "n": 3.5, "on": true},
		Props:    Values{"title": `Tom & "Jerry"`},
		Site:     map[string]string{"title": "Site's"},
		Post:     map[string]string{"title": "<b>Post</b>"},
		Item:     map[string]string{"price": "฿290"},
	}
	tpl := `<p title="{{props.title}}">{{settings.text}}</p>{{ settings.n }}|{{settings.on}}|{{site.title}}|{{post.title}}|{{item.price}}|{{props.missing}}|{{evil.x}}|{{settings.text|raw}}`
	got := RenderTemplate(tpl, d)
	want := `<p title="Tom &amp; &#34;Jerry&#34;">&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;</p>3.5|true|Site&#39;s|&lt;b&gt;Post&lt;/b&gt;|฿290|||`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if strings.Contains(got, "<script>") {
		t.Fatal("raw script leaked")
	}
	if u := RenderText("{{settings.url}}?a=1", TemplateData{Settings: Values{"url": "https://x.io/h"}}); u != "https://x.io/h?a=1" {
		t.Errorf("RenderText: %s", u)
	}
}

type zf struct {
	name string
	body string
	mode fs.FileMode
}

func mkzip(t *testing.T, files ...zf) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate}
		if f.mode != 0 {
			h.SetMode(f.mode)
		}
		fw, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(f.body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const goodManifest = `{"id":"hello-banner","name":"Hello Banner","version":"1.2.0","icon":"📣","category":"marketing",
 "permissions":["head","bodyEnd","blocks","settings","webhooks"],
 "settings":[{"key":"text","type":"text","label":{"th":"ข้อความ","en":"Text"},"default":"Hello"},
             {"key":"hook","type":"url","label":"Webhook"}],
 "head":["assets/style.css"],"bodyEnd":"templates/body.html",
 "blocks":[{"type":"card","name":{"th":"การ์ด","en":"Card"},"template":"templates/card.html","css":"templates/card.css",
   "props":[{"key":"title","type":"text","label":"Title"}]}],
 "webhooks":[{"event":"form.submitted","url":"{{settings.hook}}"}]}`

func goodFiles() []zf {
	return []zf{{name: "manifest.json", body: goodManifest}, {name: "assets/style.css", body: "body{}"},
		{name: "templates/body.html", body: "<div>{{settings.text}}</div>"},
		{name: "templates/card.html", body: "<h3>{{props.title}}</h3>"}, {name: "templates/card.css", body: "h3{color:red}"}}
}

func TestParsePackage(t *testing.T) {
	mf, files, err := ParsePackage(mkzip(t, goodFiles()...))
	if err != nil {
		t.Fatal(err)
	}
	if mf.ID != "hello-banner" || len(files) != 5 || mf.Settings[1].Label.TH != "Webhook" {
		t.Fatalf("unexpected: %+v %d", mf, len(files))
	}
	// a zip of the folder (single top-level directory) is accepted too
	var nested []zf
	for _, f := range goodFiles() {
		f.name = "hello-banner/" + f.name
		nested = append(nested, f)
	}
	if _, _, err := ParsePackage(mkzip(t, nested...)); err != nil {
		t.Fatalf("nested root: %v", err)
	}

	bad := map[string][]zf{
		"zip-slip":     append(goodFiles(), zf{name: "../evil.js", body: "x"}),
		"zip-slip-mid": append(goodFiles(), zf{name: "assets/../../evil.css", body: "x"}),
		"absolute":     append(goodFiles(), zf{name: "/etc/passwd.txt", body: "x"}),
		"backslash":    append(goodFiles(), zf{name: `assets\..\x.css`, body: "x"}),
		"drive":        append(goodFiles(), zf{name: "C:/x.css", body: "x"}),
		"bad-ext":      append(goodFiles(), zf{name: "assets/run.exe", body: "MZ"}),
		"php":          append(goodFiles(), zf{name: "x.php", body: "<?php"}),
		"symlink":      append(goodFiles(), zf{name: "assets/link.css", body: "/etc/passwd", mode: fs.ModeSymlink | 0o777}),
		"oversized":    append(goodFiles(), zf{name: "assets/big.js", body: strings.Repeat("a", MaxFileSize+1)}),
		"no-manifest":  goodFiles()[1:],
		"dup-case":     append(goodFiles(), zf{name: "Assets/Style.css", body: "x"}, zf{name: "assets/STYLE.css", body: "y"}),
	}
	for name, files := range bad {
		if _, _, err := ParsePackage(mkzip(t, files...)); err == nil {
			t.Errorf("%s: expected an error", name)
		} else {
			var pe *PackageError
			if !errors.As(err, &pe) {
				t.Errorf("%s: want *PackageError, got %T", name, err)
			}
		}
	}
	if _, _, err := ParsePackage(bytes.Repeat([]byte("x"), MaxPackageSize+1)); err == nil {
		t.Error("oversized package accepted")
	}
	if _, _, err := ParsePackage([]byte("not a zip")); err == nil {
		t.Error("garbage accepted")
	}

	badManifests := map[string]string{
		"bad-id":       strings.Replace(goodManifest, `"hello-banner"`, `"Hello Banner"`, 1),
		"bad-version":  strings.Replace(goodManifest, `"1.2.0"`, `"v1"`, 1),
		"missing-perm": strings.Replace(goodManifest, `"head","bodyEnd",`, `"bodyEnd",`, 1),
		"missing-file": strings.Replace(goodManifest, `templates/body.html`, `templates/nope.html`, 1),
		"bad-field":    strings.Replace(goodManifest, `"type":"url"`, `"type":"script"`, 1),
		"bad-category": strings.Replace(goodManifest, `"marketing"`, `"warez"`, 1),
		"future-core":  strings.Replace(goodManifest, `"icon"`, `"minCore":"9.0.0","icon"`, 1),
		"head-js-html": strings.Replace(goodManifest, `"head":["assets/style.css"]`, `"head":["templates/body.html"]`, 1),
	}
	for name, m := range badManifests {
		files := goodFiles()
		files[0].body = m
		if _, _, err := ParsePackage(mkzip(t, files...)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestValidateSettings(t *testing.T) {
	min, max := 1.0, 10.0
	schema := []Field{
		{Key: "title", Type: FieldText, Default: "Hi"},
		{Key: "count", Type: FieldNumber, Min: &min, Max: &max, Default: 3.0},
		{Key: "on", Type: FieldBoolean},
		{Key: "color", Type: FieldColor, Default: "#fff"},
		{Key: "mode", Type: FieldSelect, Options: []Option{{Value: "a"}, {Value: "b"}}},
		{Key: "link", Type: FieldURL},
		{Key: "img", Type: FieldImage},
	}
	v, err := Validate(schema, map[string]any{"count": "7", "on": "true", "mode": "b", "link": "/about",
		"img": "https://x.io/a.png", "unknown": "dropped"})
	if err != nil {
		t.Fatal(err)
	}
	if v["title"] != "Hi" || v["count"] != 7.0 || v["on"] != true || v["mode"] != "b" || v["color"] != "#fff" {
		t.Errorf("values: %v", v)
	}
	if _, ok := v["unknown"]; ok {
		t.Error("unknown key kept")
	}
	_, err = Validate(schema, map[string]any{"count": 99.0, "on": "maybe", "color": "red;}body{", "mode": "z",
		"link": "javascript:alert(1)", "img": "//evil.com/x.png", "title": map[string]any{"x": 1}})
	var verr ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	for _, k := range []string{"count", "on", "color", "mode", "link", "img", "title"} {
		if _, ok := verr[k]; !ok {
			t.Errorf("%s should be invalid (%v)", k, verr)
		}
	}
	// Sanitize falls back to defaults
	s := Sanitize(schema, map[string]any{"color": "url(javascript:x)", "count": 2.0})
	if s["color"] != "#fff" || s["count"] != 2.0 {
		t.Errorf("sanitize: %v", s)
	}
	if err := ValidateSchema([]Field{{Key: "a", Type: FieldSelect}}); err == nil {
		t.Error("select without options accepted")
	}
}

func TestScopeCSS(t *testing.T) {
	in := `/* c */ @import url(x.css); h3, .a > b {color:red} :scope{padding:0} &:hover{x:y} body{margin:0}
@media (max-width:600px){ p{font-size:12px} } @keyframes spin{from{transform:rotate(0)}to{transform:rotate(1turn)}}
a:is(.x,.y){c:d} </style><script>`
	got := ScopeCSS(in, ".pb-p-b")
	for _, want := range []string{
		".pb-p-b h3,.pb-p-b .a > b{color:red}", ".pb-p-b{padding:0}", ".pb-p-b:hover{x:y}", ".pb-p-b{margin:0}",
		"@media (max-width:600px){.pb-p-b p{font-size:12px}", "@keyframes spin{from{transform:rotate(0)}to{transform:rotate(1turn)}}",
		".pb-p-b a:is(.x,.y){c:d}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "@import") || strings.Contains(got, "<") {
		t.Errorf("unsafe output:\n%s", got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"1.0.0", "1.0.1", -1}, {"1.10.0", "1.9.9", 1}, {"2.0.0", "2.0.0", 0}, {"1.0.0-beta", "1.0.0", -1}}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("%s vs %s = %d", c.a, c.b, got)
		}
	}
}

func TestSafeURL(t *testing.T) {
	for _, u := range []string{"https://a.io", "/x", "#top", "mailto:a@b.co", "tel:+66"} {
		if !SafeURL(u) {
			t.Errorf("%s should be safe", u)
		}
	}
	for _, u := range []string{"javascript:alert(1)", " JavaScript:x", "data:text/html,x", "//evil.com", "vbscript:x", "x.html", "/a\"onload"} {
		if SafeURL(u) {
			t.Errorf("%s should be unsafe", u)
		}
	}
}
