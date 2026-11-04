package sitecode

import (
	"strings"
	"testing"
)

// 2.1.1: a function schedule can be named (Background jobs showed @cron-1, @cron-2 …).
func TestCronNames(t *testing.T) {
	p := mustCompile(t, map[string]string{"main.ts": `
site.cron('30 3 * * *', () => 1, { name: 'linkCleanup' })
site.cron('@daily', () => 2)
site.cron('@hourly', () => 3, { name: 'warm' })
`})
	m := p.Manifest
	if len(m.Crons) != 3 || m.Crons[0].Name != "@linkCleanup" || m.Crons[0].Job != "@linkCleanup" || m.Crons[1].Name != "@cron-1" || m.Crons[2].Name != "@warm" {
		t.Fatalf("crons: %+v", m.Crons)
	}
	if p.jobs["@linkCleanup"] == nil || p.jobs["@cron-1"] == nil {
		t.Fatalf("internal jobs: %v", p.jobs)
	}
	_, probs := compile(t, map[string]string{"main.ts": "site.cron('@daily', () => 1, { name: 'a' })\nsite.cron('@hourly', () => 2, { name: 'a' })\n"})
	if len(probs) != 1 || !strings.Contains(probs[0].Message, "ซ้ำ") {
		t.Fatalf("duplicate name: %+v", probs)
	}
	_, probs = compile(t, map[string]string{"main.ts": "site.cron('@daily', () => 1, { name: 'Bad name' })\n"})
	if len(probs) != 1 || !strings.Contains(probs[0].Message, "name") {
		t.Fatalf("bad name: %+v", probs)
	}
}

// 2.2.0: atob / btoa in the sandbox.
func TestBase64Globals(t *testing.T) {
	v := newUnitVM(nil, siteUnitInfo)
	for src, want := range map[string]string{
		`btoa('hello')`:    "aGVsbG8=",
		`atob('aGVsbG8=')`: "hello",
		`atob('aGVs bG8')`: "hello",
		`decodeURIComponent(escape(atob(btoa(unescape(encodeURIComponent('สวัสดี'))))))`: "สวัสดี",
	} {
		got, err := v.rt.RunString(src)
		if err != nil || got.String() != want {
			t.Errorf("%s = %v %v, want %q", src, got, err, want)
		}
	}
	if _, err := v.rt.RunString(`btoa('ก')`); err == nil {
		t.Error("btoa of a non-Latin1 string must throw")
	}
}
