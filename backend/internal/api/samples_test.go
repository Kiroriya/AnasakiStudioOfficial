package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/ascii85"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/db"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

// Smoke tests of the sample JS plugins in examples/js-plugins (phase 5): each is installed from its
// folder through the package API exactly as an administrator would, then exercised through public
// joints only (routes, Site Code imports, formatters, admin pages, actions, cron).

type sampleEnv struct {
	*pluginEnv
	mu    sync.Mutex
	mails []sitecode.MailMessage
}

func (e *sampleEnv) mailbox() []sitecode.MailMessage {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sitecode.MailMessage(nil), e.mails...)
}

// newSampleAPI is newPluginAPI with a mail sink (Config.Mailer) that records mail.send messages.
func newSampleAPI(t *testing.T) *sampleEnv {
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
	st := store.New(conn)
	up := t.TempDir()
	// clear the tables before the manager loads the installed packages, and leave no active
	// plugins behind for the next test (the manager reads plugin_packages when it starts)
	clear := func() {
		for _, tbl := range []string{"plugin_packages", "plugin_kv", "script_settings", "script_jobs", "script_cron"} {
			_, _ = conn.Exec("DELETE FROM " + tbl)
		}
	}
	clear()
	t.Cleanup(clear)
	pm, err := plugin.NewManager(st, up)
	if err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"users", "posts", "post_terms", "terms", "revisions", "media", "menus", "components", "options", "roles",
		"script_files", "script_versions", "script_logs", "script_audit", "script_access",
		"post_meta", "script_settings", "script_jobs", "script_cron", "plugin_packages", "plugin_kv"} {
		if _, err := conn.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Roles(st); err != nil {
		t.Fatal(err)
	}
	se := &sampleEnv{}
	web := site.New(st, pm)
	authn := auth.New("test-secret")
	resetCodeDB(t, conn, os.Getenv("TEST_CODE_DB_DSN"))
	code := sitecode.New(st, sitecode.Config{Auth: authn, Services: pm.ServicePublicPaths, PostURL: site.PostURL,
		DSN: dsn, CodeDSN: os.Getenv("TEST_CODE_DB_DSN"), Emit: pm.Emit, TypeSlugs: site.SetTypeSlugs,
		SecretKey: "test-site-code-key", KeyDir: t.TempDir(), Plugins: pm,
		Mailer: func(m sitecode.MailMessage) error {
			se.mu.Lock()
			defer se.mu.Unlock()
			se.mails = append(se.mails, m)
			return nil
		}})
	t.Cleanup(code.Close)
	pm.SetCodeRuntime(code)
	// bcrypt (members) and bundling on a busy CI machine: a little more room than the other tests
	code.RouteTimeout, code.EventTimeout = 2*time.Second, 3*time.Second
	web.SetCode(code.ServePublic)
	web.SetCodeSessions(code.Session)
	pm.Listen(code.OnEvent)
	mux := http.NewServeMux()
	a := New(st, authn, up, pm, web)
	a.SetSiteCode(code)
	a.Routes(mux)
	mux.Handle("/", pm.Middleware(web))
	t.Cleanup(code.WaitEvents)
	se.pluginEnv = &pluginEnv{c: &client{t: t, h: mux}, code: code, pm: pm, st: st}
	return se
}

func installSample(t *testing.T, admin *client, ids ...string) {
	t.Helper()
	for _, id := range ids {
		mustUpload(t, admin, examplePlugin(t, id))
		activate(t, admin, id)
	}
}

// sampleRoute calls a public route and decodes its JSON answer.
func sampleRoute(t *testing.T, c *client, path string) map[string]any {
	t.Helper()
	code, b := getJSON(t, c, "GET", path, "", nil)
	if code != 200 {
		t.Fatalf("GET %s: %d %v", path, code, b)
	}
	return b
}

// liveSession opens a Site Code session (all units) like a public request would.
func liveSession(t *testing.T, e *sampleEnv) interface {
	Format(name string, v any, args []any) (string, bool)
	Close()
} {
	t.Helper()
	opts, err := e.st.Options()
	if err != nil {
		t.Fatal(err)
	}
	s := e.code.Session(httptest.NewRequest("GET", "/", nil), opts)
	if s == nil {
		t.Fatal("no live session")
	}
	return s
}

// ---------------------------------------------------------------- thai

func TestSampleThai(t *testing.T) {
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "thai")
	publishCode(t, admin, map[string]string{"main.ts": `
import { bahtText, thaiDate, thaiNumber, baht, isThaiCitizenId, isThaiPhone, normalizePhone, formatCitizenId, registerFormatters } from 'thai';
registerFormatters('legacy');   // deprecated helper: still registers Site Code formatters (legacybaht, …)
site.route('GET /thai/call', { access: ['*'] }, () => ({
  has: plugins.has('thai'), version: plugins.version('thai'), missing: [plugins.has('nope'), plugins.version('nope')],
  words: plugins.call('thai', 'format', 'bahtText', 21), date: plugins.call('thai', 'format', 'thaiDate', '2026-10-30', 'D MMMM BBBB'),
}));
site.route('GET /thai', { access: ['*'] }, ({ request }) => {
  const vs = JSON.parse(String(request.query.v));
  return {
    text: vs.map((v) => bahtText(v)),
    date: [thaiDate('2026-10-30T07:05:00Z', 'full'), thaiDate('2026-10-30', 'short'), thaiDate('2026-12-31T17:30:00Z', 'datetime'), thaiDate('bad')],
    num: [thaiNumber(1234.5), thaiNumber('2569'), baht(-1234.5), baht('99.999')],
    id: [isThaiCitizenId('1-2345-67890-12-1'), isThaiCitizenId('1234567890122'), isThaiCitizenId('123'), formatCitizenId('1103702071838')],
    phone: [isThaiPhone('081-234-5678'), isThaiPhone('02-123-4567'), isThaiPhone('02-123-4567', { mobileOnly: true }), isThaiPhone('12345'),
      normalizePhone('+66 81 234 5678'), normalizePhone('0812345678', 'e164')],
  };
});`})
	vectors := []struct {
		in   any
		want string
	}{
		{0, "ศูนย์บาทถ้วน"}, {1, "หนึ่งบาทถ้วน"}, {11, "สิบเอ็ดบาทถ้วน"}, {21, "ยี่สิบเอ็ดบาทถ้วน"}, {101, "หนึ่งร้อยเอ็ดบาทถ้วน"},
		{1000000, "หนึ่งล้านบาทถ้วน"}, {1000001, "หนึ่งล้านเอ็ดบาทถ้วน"}, {12.50, "สิบสองบาทห้าสิบสตางค์"}, {"12.50", "สิบสองบาทห้าสิบสตางค์"},
		{21000000, "ยี่สิบเอ็ดล้านบาทถ้วน"}, {10, "สิบบาทถ้วน"}, {20, "ยี่สิบบาทถ้วน"}, {111, "หนึ่งร้อยสิบเอ็ดบาทถ้วน"}, {1001, "หนึ่งพันเอ็ดบาทถ้วน"},
		{0.5, "ห้าสิบสตางค์"}, {0.01, "หนึ่งสตางค์"}, {0.21, "ยี่สิบเอ็ดสตางค์"}, {1.005, "หนึ่งบาทหนึ่งสตางค์"}, {0.999, "หนึ่งบาทถ้วน"},
		{-5, "ลบห้าบาทถ้วน"}, {11000011, "สิบเอ็ดล้านสิบเอ็ดบาทถ้วน"}, {"1000000000000", "หนึ่งล้านล้านบาทถ้วน"},
		{"1000001000000", "หนึ่งล้านเอ็ดล้านบาทถ้วน"}, {"1000000000001", "หนึ่งล้านล้านเอ็ดบาทถ้วน"},
		{1234567.89, "หนึ่งล้านสองแสนสามหมื่นสี่พันห้าร้อยหกสิบเจ็ดบาทแปดสิบเก้าสตางค์"}, {"1,250.75", "หนึ่งพันสองร้อยห้าสิบบาทเจ็ดสิบห้าสตางค์"},
	}
	ins := make([]any, len(vectors))
	for i, v := range vectors {
		ins[i] = v.in
	}
	q, _ := json.Marshal(ins)
	b := sampleRoute(t, e.c, "/thai?v="+urlEscape(string(q)))
	for i, got := range b["text"].([]any) {
		if got != vectors[i].want {
			t.Errorf("bahtText(%v) = %v, want %s", vectors[i].in, got, vectors[i].want)
		}
	}
	want := map[string]string{
		"date":  "[วันศุกร์ที่ 30 ตุลาคม พ.ศ. 2569 30 ต.ค. 69 1 ม.ค. 2570 00:30 ]",
		"num":   "[๑,๒๓๔.๕ ๒๕๖๙ -฿1,234.50 ฿100.00]",
		"id":    "[true false false 1-1037-02071-83-8]",
		"phone": "[true true false false 0812345678 +66812345678]",
	}
	for k, w := range want {
		if got := fmt.Sprint(b[k]); got != w {
			t.Errorf("%s = %s, want %s", k, got, w)
		}
	}
	// phase 6: the plugin registers its formatters itself ("thai:<name>"), no Site Code needed
	_, bind := admin.do("GET", "/api/v1/code/bindings", nil)
	if f := fmt.Sprint(bind["formatters"]); !strings.Contains(f, "thai:bahtText") || !strings.Contains(f, "thai:thaiDate") ||
		!strings.Contains(f, "thai:thaiNumber") || !strings.Contains(f, "thai:baht") || !strings.Contains(f, "legacybaht") {
		t.Fatalf("formatters = %s", f)
	}
	formats := []struct {
		name string
		v    any
		args []any
		want string
	}{
		{"bahtText", 21, nil, "ยี่สิบเอ็ดบาทถ้วน"}, {"baht", "1234.5", nil, "฿1,234.50"}, {"baht", 10, []any{0}, "฿10"},
		{"thaiDate", "2026-10-30", []any{"D MMMM BBBB"}, "30 ตุลาคม 2569"}, {"thaiNumber", 2569, nil, "๒,๕๖๙"},
	}
	check := func(prefix string, wantOK bool) {
		t.Helper()
		s := liveSession(t, e)
		defer s.Close()
		for _, f := range formats {
			got, ok := s.Format(prefix+f.name, f.v, f.args)
			if wantOK && (!ok || got != f.want) {
				t.Errorf("| %s%s %v = %q (%v), want %q", prefix, f.name, f.v, got, ok, f.want)
			}
			if !wantOK && ok {
				t.Errorf("| %s%s resolved without an alias: %q", prefix, f.name, got)
			}
		}
	}
	check("thai:", true)
	check("legacy", true) // registerFormatters('legacy') in Site Code
	check("", false)      // unprefixed names need an alias enabled by the administrator
	if code, out := admin.do("PUT", "/api/v1/code/formatters/aliases", map[string]any{"aliases": map[string]any{
		"baht": "thai:baht", "bahtText": "thai:bahtText", "thaiDate": "thai:thaiDate", "thaiNumber": "thai:thaiNumber"}}); code != 200 {
		t.Fatalf("aliases: %d %v", code, out)
	}
	check("", true)
	// the export for optional users: plugins.call / has / version
	b = sampleRoute(t, e.c, "/thai/call")
	if b["has"] != true || b["version"] != "1.0.0" || fmt.Sprint(b["missing"]) != "[false <nil>]" ||
		b["words"] != "ยี่สิบเอ็ดบาทถ้วน" || b["date"] != "30 ตุลาคม 2569" {
		t.Fatalf("/thai/call = %v", b)
	}
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- promptpay

// crcCCITTFalse is an independent CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF).
func crcCCITTFalse(s string) string {
	crc := uint16(0xFFFF)
	for _, c := range []byte(s) {
		for i := 7; i >= 0; i-- {
			bit := (c>>uint(i))&1 == 1
			top := crc&0x8000 != 0
			crc <<= 1
			if top != bit {
				crc ^= 0x1021
			}
		}
	}
	return fmt.Sprintf("%04X", crc)
}

// bch appends the remainder of data·x^deg divided by gen (an independent BCH encoder).
func bch(data, gen uint32, deg, dataBits int) uint32 {
	v := data << uint(deg)
	for i := dataBits + deg - 1; i >= deg; i-- {
		if v&(1<<uint(i)) != 0 {
			v ^= gen << uint(i-deg)
		}
	}
	return data<<uint(deg) | v
}

// goldenPromptPayQR: payload({id:'0812345678', amount:100}) as version 5-M, mask 2 (1 = dark). The
// same matrix is produced by the segno Python library (byte mode, level M, mask 2) and decodes back
// to the payload with zxing-cpp.
var goldenPromptPayQR = []string{
	"1111111000101000100101111001101111111",
	"1000001001001000001001000111001000001",
	"1011101011010001000001000011101011101",
	"1011101011010001111010000101101011101",
	"1011101010000111000101111110001011101",
	"1000001011011111010001101001101000001",
	"1111111010101010101010101010101111111",
	"0000000010101100001001001000000000000",
	"1011111000010100011010000101101111100",
	"1011000011110101000101111011110001110",
	"0010001000011011101001001001001011001",
	"1111100110011111000001000000001101001",
	"1000001101000111111010000101101110101",
	"0111000000001010000101111010010001010",
	"0010001000001111011011001011111101001",
	"1101110101111001000001001011010001000",
	"0000111001011111011010000100101110111",
	"1000010001101001100101111010010001010",
	"0010111100110001001001001001001100001",
	"0111010110000001000111010001000101001",
	"0011001000111000111110000101101110101",
	"0110010000001010000101111010010001010",
	"0010011011110111101000101101000001101",
	"0001100101001010001001001010001001000",
	"1010111100001111111000000101101110111",
	"1110110101101000000101111010010001010",
	"1000011100100011001000100001001001101",
	"1010000000001000001001010011011101001",
	"1010011000111111111010000100111110111",
	"0000000010011000000101111010100011010",
	"1111111000011001100011000000101011001",
	"1000001010000101001111101001100011001",
	"1011101011011011000010000101111110101",
	"1011101010000111101101111011101111011",
	"1011101010000001001000100111010000111",
	"1000001000111000000001111000111101001",
	"1111111011110101111010001011011000111",
}

func TestSamplePromptPay(t *testing.T) {
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "promptpay")
	publishCode(t, admin, map[string]string{"main.ts": `
import { payload, crc16, verifyPayload, qrMatrix, rsDivisor, rsRemainder, formatBits, versionBits, dataCodewords } from 'promptpay';
site.route('GET /pp', { access: ['*'] }, () => {
  const q = qrMatrix(payload({ id: '0812345678', amount: 100 }));
  return {
    p: [payload('0801234567'), payload({ id: '080-123-4567' }), payload({ id: '+66-89-123-4567' }), payload({ id: '1111111111111' }),
      payload({ id: '1-1111-11111-11-1' }), payload({ id: '0123456789012' }), payload({ id: '012345678901234' }),
      payload({ id: '000-000-0000', amount: 4.22 }), payload({ id: '0812345678' }), payload({ id: '0812345678', amount: '100' }),
      payload({ billerId: '010555512345601', ref1: 'inv001', amount: 990 })],
    check: crc16('123456789'),
    verify: [verifyPayload(payload({ id: '0812345678' })), verifyPayload('00020101021129370016A000000677010111011300668123456785802TH53037646304FFFF')],
    rs: rsRemainder([0x10, 0x20, 0x0c, 0x56, 0x61, 0x80, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11], rsDivisor(10)),
    fmt: [0, 1, 2, 3, 4, 5, 6, 7].map(formatBits),
    ver: [7, 20, 40].map(versionBits),
    cap: [1, 4, 5, 10, 40].map(dataCodewords),
    qr: { version: q.version, mask: q.mask, rows: q.modules.map((r) => r.map((x) => (x ? 1 : 0)).join('')) },
  };
});`})
	b := sampleRoute(t, e.c, "/pp")
	ps := b["p"].([]any)
	// published vectors (github.com/dtinth/promptpay-qr test suite)
	published := []string{
		"00020101021129370016A000000677010111011300668012345675802TH530376463046197",
		"00020101021129370016A000000677010111011300668012345675802TH530376463046197",
		"00020101021129370016A000000677010111011300668912345675802TH5303764630429C1",
		"00020101021129370016A000000677010111021311111111111115802TH530376463047B5A",
		"00020101021129370016A000000677010111021311111111111115802TH530376463047B5A",
		"00020101021129370016A000000677010111021301234567890125802TH530376463040CBD",
		"00020101021129390016A00000067701011103150123456789012345802TH530376463049781",
		"00020101021229370016A000000677010111011300660000000005802TH530376454044.226304E469",
	}
	for i, w := range published {
		if ps[i] != w {
			t.Errorf("payload %d = %v, want %s", i, ps[i], w)
		}
	}
	// the asked-for vectors, with the CRC recomputed here independently
	for i, body := range []string{
		"00020101021129370016A000000677010111011300668123456785802TH53037646304",
		"00020101021229370016A000000677010111011300668123456785802TH53037645406100.006304",
		"00020101021230490016A00000067701011201150105555123456010206INV0015802TH53037645406990.006304",
	} {
		if want := body + crcCCITTFalse(body); ps[8+i] != want {
			t.Errorf("payload %d = %v, want %s", 8+i, ps[8+i], want)
		}
	}
	if ps[8] != "00020101021129370016A000000677010111011300668123456785802TH530376463045D82" {
		t.Errorf("0812345678 = %v", ps[8])
	}
	if b["check"] != "29B1" || crcCCITTFalse("123456789") != "29B1" || fmt.Sprint(b["verify"]) != "[true false]" {
		t.Fatalf("crc check value / verify: %v %v", b["check"], b["verify"])
	}
	// Reed–Solomon: ISO/IEC 18004 Annex I example (1-M, "01234567")
	if got := fmt.Sprint(b["rs"]); got != "[165 36 212 193 237 54 199 135 44 85]" {
		t.Errorf("RS ecc = %s", got)
	}
	for m, x := range b["fmt"].([]any) {
		if want := bch(uint32(m), 0x537, 10, 5) ^ 0x5412; uint32(x.(float64)) != want {
			t.Errorf("format bits M/mask %d = %015b, want %015b", m, uint32(x.(float64)), want)
		}
	}
	if fmt.Sprint(b["fmt"].([]any)[0]) != fmt.Sprint(float64(0b101010000010010)) {
		t.Errorf("format M/0 = %v", b["fmt"].([]any)[0])
	}
	for i, v := range []uint32{7, 20, 40} {
		if want := bch(v, 0x1F25, 12, 6); uint32(b["ver"].([]any)[i].(float64)) != want {
			t.Errorf("version bits %d = %v, want %x", v, b["ver"].([]any)[i], want)
		}
	}
	if uint32(b["ver"].([]any)[0].(float64)) != 0x07C94 {
		t.Errorf("version 7 bits = %v", b["ver"].([]any)[0])
	}
	if got := fmt.Sprint(b["cap"]); got != "[16 64 86 216 2334]" {
		t.Errorf("data codewords (M) = %s", got)
	}
	qr := b["qr"].(map[string]any)
	if qr["version"].(float64) != 5 || qr["mask"].(float64) != 2 {
		t.Fatalf("qr version/mask = %v/%v", qr["version"], qr["mask"])
	}
	for i, row := range qr["rows"].([]any) {
		if row != goldenPromptPayQR[i] {
			t.Fatalf("qr row %d:\n got %v\nwant %s", i, row, goldenPromptPayQR[i])
		}
	}

	// the plugin's own route with its setting
	if code, _ := getJSON(t, e.c, "GET", "/promptpay/payload", "", nil); code != 400 {
		t.Fatalf("payload without promptpayId: %d", code)
	}
	if code, out := admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": "promptpay",
		"values": []any{map[string]any{"key": "promptpayId", "value": "081-234-5678", "secret": false}}}); code != 200 {
		t.Fatalf("settings: %d %v", code, out)
	}
	if p := sampleRoute(t, e.c, "/promptpay/payload?amount=100")["payload"]; p != ps[9] {
		t.Fatalf("/promptpay/payload = %v", p)
	}
	rec := e.c.send("GET", "/promptpay/qr.svg?amount=100&size=200", nil, nil)
	svg := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 45 45"`) {
		t.Fatalf("qr.svg: %d %v %.200s", rec.Code, rec.Header(), svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
		t.Fatalf("svg is not XML: %v", err)
	}
	// each dark run in the path is a dark module run of the golden matrix (quiet zone 4)
	dark := 0
	for _, m := range regexp.MustCompile(`M(\d+) (\d+)h(\d+)`).FindAllStringSubmatch(svg, -1) {
		var x, y, n int
		fmt.Sscan(m[1], &x)
		fmt.Sscan(m[2], &y)
		fmt.Sscan(m[3], &n)
		if strings.Repeat("1", n) != goldenPromptPayQR[y-4][x-4:x-4+n] {
			t.Fatalf("svg run at %d,%d len %d does not match", x, y, n)
		}
		dark += n
	}
	want := strings.Count(strings.Join(goldenPromptPayQR, ""), "1")
	if dark != want {
		t.Fatalf("svg dark modules %d, want %d", dark, want)
	}
	if code, _ := getJSON(t, e.c, "GET", "/promptpay/qr.svg?amount=abc", "", nil); code != 400 {
		t.Fatalf("bad amount: %d", code)
	}
}

// ---------------------------------------------------------------- files

func TestSampleFiles(t *testing.T) {
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "files")
	publishCode(t, admin, map[string]string{"main.ts": `
import { csvDownload, xlsxPayload, xlsxDownload, pdfDownload, thaiPdfDownload, toCSV } from 'files';
const rows = [
  { name: 'สมชาย, "ใจดี"', total: 1234.5, note: '=HYPERLINK("x")', paid: true, at: '2026-10-30' },
  { name: ' space', total: -5, note: 'line1\nline2', paid: false, at: null },
];
const cols = [{ key: 'name', label: 'ชื่อ' }, 'total', 'note', 'paid', 'at'];
site.route('GET /x/orders.csv', { access: ['*'] }, () => csvDownload('ยอดขาย.csv', rows, cols));
site.route('GET /x/plain.csv', { access: ['*'] }, () => ({ status: 200, headers: { 'Content-Type': 'text/csv' }, body: toCSV(rows, ['total'], { bom: false, separator: ';' }) }));
site.route('GET /x/orders.xlsx', { access: ['*'] }, () => xlsxPayload('orders.xlsx', rows, cols, { sheetName: 'ยอดขาย' }));
site.route('GET /x/direct.xlsx', { access: ['*'] }, () => xlsxDownload('ยอดขาย.xlsx', rows, cols, { sheetName: 'ยอดขาย' }));
site.route('GET /x/report.pdf', { access: ['*'] }, () => pdfDownload('report.pdf', { title: 'Orders (2026)', lines: ['Line one', 'ไทย'],
  table: { columns: ['total', 'note'], rows: Array.from({ length: 150 }, (_, i) => ({ total: i, note: 'row ' + i })) } }));
site.route('GET /x/thai.pdf', { access: ['*'] }, () => thaiPdfDownload('รายงาน.pdf', { title: 'รายงานยอดขาย (Sales)',
  lines: ['ผู้ใหญ่หาผ้าใหม่ น้ำใจ ปี่ ฟื้น'],
  table: { columns: [{ key: 'name', label: 'ชื่อสินค้า' }, { key: 'total', label: 'ยอด' }], rows: Array.from({ length: 60 }, (_, i) => ({ name: 'สินค้า ' + i, total: i * 10 })) } }));`})

	rec := e.c.send("GET", "/x/orders.csv", nil, nil)
	csv := rec.Body.String()
	wantCSV := "\uFEFFชื่อ,total,note,paid,at\r\n" +
		`"สมชาย, ""ใจดี""",1234.5,"'=HYPERLINK(""x"")",true,2026-10-30` + "\r\n" +
		`" space",-5,"line1` + "\n" + `line2",false,` + "\r\n"
	if rec.Code != 200 || csv != wantCSV {
		t.Fatalf("csv: %d %q\nwant %q", rec.Code, csv, wantCSV)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("csv: no UTF-8 BOM")
	}
	if ct, cd := rec.Header().Get("Content-Type"), rec.Header().Get("Content-Disposition"); ct != "text/csv; charset=utf-8" ||
		cd != `attachment; filename="______.csv"; filename*=UTF-8''%E0%B8%A2%E0%B8%AD%E0%B8%94%E0%B8%82%E0%B8%B2%E0%B8%A2.csv` {
		t.Fatalf("csv headers: %q %q", ct, cd)
	}
	if _, b := e.c.public("GET", "/x/plain.csv"); b != "total\r\n1234.5\r\n-5\r\n" {
		t.Fatalf("plain csv: %q", b)
	}

	// .xlsx as JSON + base64 (kept for compatibility) and as a binary download (phase 6)
	x := sampleRoute(t, e.c, "/x/orders.xlsx")
	if x["contentType"] != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || x["filename"] != "orders.xlsx" {
		t.Fatalf("xlsx payload: %v", x)
	}
	raw, err := base64.StdEncoding.DecodeString(x["base64"].(string))
	if err != nil {
		t.Fatal(err)
	}
	checkXLSX(t, raw)
	rec = e.c.send("GET", "/x/direct.xlsx", nil, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="______.xlsx"; filename*=UTF-8''`) {
		t.Fatalf("direct xlsx: %d %v", rec.Code, rec.Header())
	}
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("direct xlsx (%d bytes) differs from the base64 payload (%d bytes)", rec.Body.Len(), len(raw))
	}
	checkXLSX(t, rec.Body.Bytes())

	// Latin-1 PDF with the built-in Helvetica
	rec = e.c.send("GET", "/x/report.pdf", nil, nil)
	pdf := rec.Body.Bytes()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf: %d %v %.40q", rec.Code, rec.Header(), pdf)
	}
	checkPDF(t, pdf)
	if n := bytes.Count(pdf, []byte("/Type /Page ")); n < 3 || !bytes.Contains(pdf, []byte("/Count "+fmt.Sprint(n))) {
		t.Fatalf("pdf pages: %d", n)
	}
	if !bytes.Contains(pdf, []byte("(Orders \\(2026\\)) Tj")) || !bytes.Contains(pdf, []byte("(row 149) Tj")) || bytes.Contains(pdf, []byte("/FontFile2")) {
		t.Fatal("pdf text missing")
	}

	// Thai PDF: Sarabun (site.asset of the files package) embedded as CIDFontType2 / Identity-H
	rec = e.c.send("GET", "/x/thai.pdf", nil, nil)
	pdf = rec.Body.Bytes()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "filename*=UTF-8''%E0%B8%A3") {
		t.Fatalf("thai pdf: %d %v %.200q", rec.Code, rec.Header(), pdf)
	}
	checkPDF(t, pdf)
	for _, w := range []string{"/Subtype /Type0 /BaseFont /Sarabun-Regular /Encoding /Identity-H", "/Subtype /CIDFontType2",
		"/CIDToGIDMap /Identity", "/FontFile2 ", "/ToUnicode ", "/Filter /ASCII85Decode", "/Count 2 "} {
		if !bytes.Contains(pdf, []byte(w)) {
			t.Fatalf("thai pdf lacks %q", w)
		}
	}
	// the embedded font file is the sfnt of fonts/Sarabun-Regular.woff (every table byte-identical)
	m := regexp.MustCompile(`/Length (\d+) /Length1 (\d+) /Filter /ASCII85Decode >>\nstream\n`).FindSubmatchIndex(pdf)
	if m == nil {
		t.Fatal("no FontFile2 stream")
	}
	encLen, _ := strconv.Atoi(string(pdf[m[2]:m[3]]))
	sfntLen, _ := strconv.Atoi(string(pdf[m[4]:m[5]]))
	enc := pdf[m[1] : m[1]+encLen]
	if !bytes.HasSuffix(enc, []byte("~>")) || !bytes.HasPrefix(pdf[m[1]+encLen:], []byte("\nendstream")) {
		t.Fatalf("ascii85 stream framing: %q", enc[len(enc)-10:])
	}
	sfnt, err := io.ReadAll(ascii85.NewDecoder(bytes.NewReader(bytes.TrimSuffix(enc, []byte("~>")))))
	if err != nil || len(sfnt) != sfntLen {
		t.Fatalf("ascii85: %v, %d bytes, Length1 %d", err, len(sfnt), sfntLen)
	}
	woff := []byte(examplePlugin(t, "files")["fonts/Sarabun-Regular.woff"])
	tables := sfntTables(t, sfnt)
	checkWoffTables(t, woff, tables)
	cmap := sfntCmap4(t, sfnt, tables["cmap"])
	// ToUnicode: every used glyph maps back to the character it was drawn for
	used := map[rune]bool{}
	for _, bf := range regexp.MustCompile(`<([0-9A-F]{4})> <([0-9A-F]{4})>`).FindAllSubmatch(pdf, -1) {
		gid, _ := strconv.ParseUint(string(bf[1]), 16, 16)
		r, _ := strconv.ParseUint(string(bf[2]), 16, 16)
		if cmap[rune(r)] != uint16(gid) {
			t.Errorf("ToUnicode <%s> → U+%04X, but the cmap maps it to glyph %d", bf[1], r, cmap[rune(r)])
		}
		used[rune(r)] = true
	}
	for _, r := range "รายงานยอดขผู้ใหญ่ห้ำปีฟื้ชื่อสินค้ด0123456789" {
		if r == 'ำ' { // SARA AM is drawn as NIKHAHIT + SARA AA
			continue
		}
		if !used[r] {
			t.Errorf("ToUnicode lacks U+%04X %q", r, r)
		}
	}
	// "รายงาน" (no marks) is one run of glyph ids in the content
	run := ""
	for _, r := range "รายงาน" {
		run += fmt.Sprintf("%04X", cmap[r])
	}
	if !bytes.Contains(pdf, []byte("<"+run)) {
		t.Fatalf("title glyph run %s not in the content", run)
	}
	// marks are placed with their own text matrix (GPOS anchors): ญ่ in ผู้ใหญ่ is followed by a positioned tone mark
	if !regexp.MustCompile(`1 0 0 1 [\d.]+ [\d.]+ Tm <` + fmt.Sprintf("%04X", cmap['่']) + `> Tj`).Match(pdf) {
		t.Fatal("no positioned tone mark")
	}
}

// checkXLSX opens an .xlsx and checks its parts and cells.
func checkXLSX(t *testing.T, raw []byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("xlsx is not a zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc) // verifies the CRC-32
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if f.Method != zip.Store {
			t.Fatalf("%s: method %d", f.Name, f.Method)
		}
		if err := xml.Unmarshal(b, new(struct{})); err != nil {
			t.Fatalf("%s is not XML: %v", f.Name, err)
		}
		parts[f.Name] = string(b)
	}
	for _, p := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/worksheets/sheet1.xml"} {
		if _, ok := parts[p]; !ok {
			t.Fatalf("xlsx part %s missing (have %v)", p, parts)
		}
	}
	sheet := parts["xl/worksheets/sheet1.xml"]
	for _, w := range []string{`<c r="A1" s="1" t="inlineStr"><is><t xml:space="preserve">ชื่อ</t></is></c>`,
		`<c r="A2" t="inlineStr"><is><t xml:space="preserve">สมชาย, &quot;ใจดี&quot;</t></is></c>`, `<c r="B2"><v>1234.5</v></c>`,
		`<c r="D2" t="b"><v>1</v></c>`, `<c r="B3"><v>-5</v></c>`, `<row r="3">`} {
		if !strings.Contains(sheet, w) {
			t.Fatalf("sheet lacks %s:\n%s", w, sheet)
		}
	}
	if !strings.Contains(parts["xl/workbook.xml"], `name="ยอดขาย"`) || !strings.Contains(parts["[Content_Types].xml"], "spreadsheetml.worksheet+xml") {
		t.Fatalf("workbook: %s", parts["xl/workbook.xml"])
	}
}

// checkPDF: header, trailer, pure ASCII, and xref offsets pointing at their objects.
func checkPDF(t *testing.T, pdf []byte) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4\n")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatalf("pdf framing: %.40q", pdf)
	}
	for _, c := range pdf {
		if c >= 0x80 {
			t.Fatal("pdf is not ASCII")
		}
	}
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindSubmatch(pdf)
	var xref int
	fmt.Sscan(string(m[1]), &xref)
	if !bytes.HasPrefix(pdf[xref:], []byte("xref\n0 ")) {
		t.Fatal("startxref does not point at xref")
	}
	offs := regexp.MustCompile(`(?m)^(\d{10}) 00000 n `).FindAllSubmatch(pdf[xref:], -1)
	for i, o := range offs {
		off, _ := strconv.Atoi(string(o[1]))
		if !bytes.HasPrefix(pdf[off:], []byte(fmt.Sprintf("%d 0 obj\n", i+1))) {
			t.Fatalf("xref entry %d → %d is not its object", i+1, off)
		}
	}
	// every stream's /Length matches its data
	for _, s := range regexp.MustCompile(`<< /Length (\d+)[^\n]*>>\nstream\n`).FindAllSubmatchIndex(pdf, -1) {
		n, _ := strconv.Atoi(string(pdf[s[2]:s[3]]))
		if !bytes.HasPrefix(pdf[s[1]+n:], []byte("\nendstream")) {
			t.Fatalf("stream at %d: /Length %d does not end at endstream", s[0], n)
		}
	}
}

// sfntTables reads the table directory of a TrueType file: tag → bytes.
func sfntTables(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	if len(b) < 12 || binary.BigEndian.Uint32(b) != 0x00010000 {
		t.Fatalf("not a TrueType sfnt: % x", b[:min(len(b), 4)])
	}
	out := map[string][]byte{}
	n := int(binary.BigEndian.Uint16(b[4:]))
	for i := 0; i < n; i++ {
		e := b[12+16*i:]
		off, ln := binary.BigEndian.Uint32(e[8:]), binary.BigEndian.Uint32(e[12:])
		out[string(e[:4])] = b[off : off+ln]
	}
	return out
}

// checkWoffTables: each (uncompressed) table of the WOFF equals the sfnt's.
func checkWoffTables(t *testing.T, woff []byte, tables map[string][]byte) {
	t.Helper()
	if string(woff[:4]) != "wOFF" {
		t.Fatal("fonts/Sarabun-Regular.woff is not a WOFF")
	}
	n := int(binary.BigEndian.Uint16(woff[12:]))
	if len(tables) != n {
		t.Fatalf("sfnt has %d tables, woff %d", len(tables), n)
	}
	for i := 0; i < n; i++ {
		e := woff[44+20*i:]
		off, comp, orig := binary.BigEndian.Uint32(e[4:]), binary.BigEndian.Uint32(e[8:]), binary.BigEndian.Uint32(e[12:])
		if comp != orig {
			t.Fatalf("woff table %s is compressed", e[:4])
		}
		if !bytes.Equal(tables[string(e[:4])], woff[off:off+orig]) {
			t.Fatalf("table %s differs", e[:4])
		}
	}
}

// sfntCmap4 decodes the (3,1) format 4 cmap: rune → glyph id.
func sfntCmap4(t *testing.T, sfnt, cmap []byte) map[rune]uint16 {
	t.Helper()
	u16 := func(b []byte, o int) int { return int(binary.BigEndian.Uint16(b[o:])) }
	out := map[rune]uint16{}
	for i := 0; i < u16(cmap, 2); i++ {
		e := 4 + 8*i
		if u16(cmap, e) != 3 || u16(cmap, e+2) != 1 {
			continue
		}
		st := cmap[binary.BigEndian.Uint32(cmap[e+4:]):]
		if u16(st, 0) != 4 {
			t.Fatal("cmap (3,1) is not format 4")
		}
		segs := u16(st, 6) / 2
		ends, starts, deltas, ranges := 14, 16+2*segs, 16+4*segs, 16+6*segs
		for s := 0; s < segs; s++ {
			end, start, delta, ro := u16(st, ends+2*s), u16(st, starts+2*s), u16(st, deltas+2*s), u16(st, ranges+2*s)
			for c := start; c <= end && c != 0xFFFF; c++ {
				var g int
				if ro == 0 {
					g = (c + delta) & 0xFFFF
				} else if g = u16(st, ranges+2*s+ro+2*(c-start)); g != 0 {
					g = (g + delta) & 0xFFFF
				}
				if g != 0 {
					out[rune(c)] = uint16(g)
				}
			}
		}
		return out
	}
	t.Fatal("no (3,1) cmap")
	return nil
}

// ---------------------------------------------------------------- admin-crud

// adminPage opens a site.adminPage through its ticket and returns the HTML, the ticket and a function
// that runs anasaki.call(name, input) like the page's helper does.
func adminPage(t *testing.T, e *sampleEnv, admin *client, id, query string) (string, func(name string, input any) (int, map[string]any)) {
	t.Helper()
	code, tk := admin.do("POST", "/api/v1/code/admin-pages/"+id+"/ticket", map[string]any{"query": query})
	if code != 200 {
		t.Fatalf("ticket %s: %d %v", id, code, tk)
	}
	u := tk["url"].(string)
	rec := e.c.send("GET", u, nil, nil)
	if rec.Code != 200 {
		t.Fatalf("admin page %s: %d %s", u, rec.Code, rec.Body.String())
	}
	tok := strings.Split(strings.TrimPrefix(u, "/ext-ui/code/"), "/")[0]
	call := func(name string, input any) (int, map[string]any) {
		b, _ := json.Marshal(map[string]any{"input": input})
		r := e.c.send("POST", "/ext-ui/code/"+tok+"/_a/"+name, bytes.NewReader(b), map[string]string{"Content-Type": "text/plain;charset=utf-8", "Origin": "null"})
		out := map[string]any{}
		_ = json.Unmarshal(r.Body.Bytes(), &out)
		return r.Code, out
	}
	return rec.Body.String(), call
}

// adminDownload opens a fresh ticket for an admin page and GETs its download handler
// (/ext-ui/code/<ticket>/<page>/download?query), like anasaki.download(query) does.
func adminDownload(t *testing.T, e *sampleEnv, admin *client, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	code, tk := admin.do("POST", "/api/v1/code/admin-pages/"+id+"/ticket", map[string]any{})
	if code != 200 {
		t.Fatalf("ticket %s: %d %v", id, code, tk)
	}
	tok := strings.Split(strings.TrimPrefix(tk["url"].(string), "/ext-ui/code/"), "/")[0]
	return e.c.send("GET", "/ext-ui/code/"+tok+"/"+id+"/download?"+query, nil, nil)
}

func TestSampleAdminCrud(t *testing.T) {
	e := newSampleAPI(t)
	admin, user := setupSite(t, e.c)
	installSample(t, admin, "admin-crud")
	if !codeTable(t, e.pluginEnv, "p_admin_crud_contacts") || !codeTable(t, e.pluginEnv, "p_admin_crud_faqs") {
		t.Fatal("migrations did not create the tables")
	}
	if _, l := admin.doAny("GET", "/api/v1/code/admin-pages", nil); !strings.Contains(fmt.Sprint(l), "admin-crud:data") {
		t.Fatalf("admin pages: %v", l)
	}
	if code, _ := user("Ed", "editor").do("POST", "/api/v1/code/admin-pages/admin-crud:data/ticket", map[string]any{}); code != 404 {
		t.Fatalf("editor ticket: %d", code)
	}
	// ctx.locale: the owner's admin language is English (setupSite) → English screen and messages
	html, call := adminPage(t, e, admin, "admin-crud:data", "")
	if !strings.Contains(html, `<table data-ac="list">`) || !strings.Contains(html, "<title>Contacts</title>") || !strings.Contains(html, "Nothing here yet") ||
		!strings.Contains(html, `data-ac="export"`) || !strings.Contains(html, "Export CSV") {
		t.Fatalf("empty list (en):\n%s", html)
	}
	if code, out := call("admin-crud:save", map[string]any{"resource": "contacts", "values": map[string]any{}}); code != 200 ||
		out["errors"].(map[string]any)["name"] != "Required" {
		t.Fatalf("validation (en): %d %v", code, out)
	}
	// switching the admin language to Thai switches ctx.locale
	if code, out := admin.do("PUT", "/api/v1/auth/me", map[string]any{"locale": "th"}); code != 200 {
		t.Fatalf("locale: %d %v", code, out)
	}
	html, call = adminPage(t, e, admin, "admin-crud:data", "")
	if !strings.Contains(html, `<table data-ac="list">`) || !strings.Contains(html, "รายชื่อติดต่อ") || !strings.Contains(html, "ไม่มีข้อมูล") ||
		!strings.Contains(html, "<title>รายชื่อติดต่อ</title>") || !strings.Contains(html, "ส่งออก CSV") {
		t.Fatalf("empty list:\n%s", html)
	}
	// validation (field errors), then create
	if code, out := call("admin-crud:save", map[string]any{"resource": "contacts", "values": map[string]any{"email": "bad", "status": "x"}}); code != 200 ||
		out["errors"].(map[string]any)["name"] != "จำเป็นต้องกรอก" || out["errors"].(map[string]any)["email"] != "อีเมลไม่ถูกต้อง" ||
		out["errors"].(map[string]any)["status"] != "ตัวเลือกไม่ถูกต้อง" {
		t.Fatalf("validation: %d %v", code, out)
	}
	for i, n := range []string{"Alice <script>", "Bob", "=cmd()"} {
		code, out := call("admin-crud:save", map[string]any{"resource": "contacts", "values": map[string]any{"name": n, "email": fmt.Sprintf("U%d@Example.com", i), "status": "new"}})
		if code != 200 || out["data"].(map[string]any)["id"] == nil {
			t.Fatalf("create: %d %v", code, out)
		}
	}
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=contacts")
	if !strings.Contains(html, "Alice &lt;script&gt;") || strings.Contains(html, "Alice <script>") || !strings.Contains(html, "u0@example.com") || !strings.Contains(html, "3 รายการ") {
		t.Fatalf("list after create:\n%s", html)
	}
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=contacts&q=bo&lang=en")
	// row values only ("=cmd()"), not a bare "cmd" that a random ticket in the page URLs could contain;
	// on failure say which check failed and show the list part, not the whole page
	var bad []string
	for _, c := range []struct {
		ok   bool
		what string
	}{
		{strings.Contains(html, ">Bob<"), `row "Bob" missing`},
		{!strings.Contains(html, "=cmd()") && !strings.Contains(html, "Alice"), "rows that do not match q=bo are listed"},
		{strings.Contains(html, ">1 rows<"), `count "1 rows" missing`},
		{strings.Contains(html, `name="lang" value="en"`), "lang=en not kept in the search form"},
	} {
		if !c.ok {
			bad = append(bad, c.what)
		}
	}
	if len(bad) > 0 {
		part := html
		if i := strings.Index(html, `<div class="bar">`); i >= 0 {
			part = html[i:]
			if j := strings.Index(part, "</table>"); j >= 0 {
				part = part[:j+len("</table>")]
			}
		}
		t.Fatalf("search (r=contacts&q=bo&lang=en): %s\n%s", strings.Join(bad, "; "), part)
	}
	// Export CSV (the page's download handler, same ticket + role check)
	rec := adminDownload(t, e, admin, "admin-crud:data", "r=contacts")
	csv := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="contacts.csv"`) ||
		!strings.HasPrefix(csv, "\uFEFF#,ชื่อ,อีเมล,โทรศัพท์,สถานะ,หมายเหตุ\r\n") || strings.Count(csv, "\r\n") != 4 ||
		!strings.Contains(csv, ",Alice <script>,u0@example.com,,new,\r\n") || !strings.Contains(csv, ",'=cmd(),u2@example.com,") {
		t.Fatalf("csv export: %d %v\n%s", rec.Code, rec.Header(), csv)
	}
	if rec := adminDownload(t, e, admin, "admin-crud:data", "r=contacts&q=bo"); strings.Count(rec.Body.String(), "\r\n") != 2 || !strings.Contains(rec.Body.String(), "Bob") {
		t.Fatalf("filtered export: %q", rec.Body.String())
	}
	if rec := e.c.send("GET", "/ext-ui/code/not-a-ticket/admin-crud:data/download?r=contacts", nil, nil); rec.Code < 400 || strings.Contains(rec.Body.String(), "Alice") {
		t.Fatalf("download without a ticket: %d", rec.Code)
	}
	id := regexp.MustCompile(`<tr data-id="(\d+)"><td>\d+</td><td>Bob`).FindStringSubmatch(html)[1]
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=contacts&edit="+id)
	if !strings.Contains(html, `<form class="edit" data-ac="form"`) || !strings.Contains(html, `value="Bob"`) || !strings.Contains(html, `<option selected>new</option>`) {
		t.Fatalf("edit form:\n%s", html)
	}
	var n float64
	fmt.Sscan(id, &n)
	if code, out := call("admin-crud:save", map[string]any{"resource": "contacts", "id": n, "values": map[string]any{"name": "Bobby", "status": "closed", "note": "x"}}); code != 200 || out["message"] != "บันทึกแล้ว" {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, out := call("admin-crud:delete", map[string]any{"resource": "contacts", "id": n}); code != 200 || out["data"].(map[string]any)["deleted"].(float64) != 1 {
		t.Fatalf("delete: %d %v", code, out)
	}
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=contacts")
	if strings.Contains(html, "Bob") || !strings.Contains(html, "2 รายการ") {
		t.Fatalf("after delete:\n%s", html)
	}
	if code, out := call("admin-crud:save", map[string]any{"resource": "nope", "values": map[string]any{}}); code != 200 || out["errors"] == nil {
		t.Fatalf("unknown resource: %d %v", code, out)
	}
	// FAQ resource + the public collection
	for i, pub := range []bool{true, false} {
		if code, out := call("admin-crud:save", map[string]any{"resource": "faqs", "values": map[string]any{"question": fmt.Sprintf("Q%d", i), "answer": "A", "sort": i, "published": pub}}); code != 200 {
			t.Fatalf("faq: %d %v", code, out)
		}
	}
	_, pv := admin.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:admin-crud:faqs", "viewAs": "anonymous"})
	if it := pv["items"].([]any); len(it) != 1 || it[0].(map[string]any)["question"] != "Q0" {
		t.Fatalf("faqs collection: %v", pv)
	}

	// a resource backed by another plugin's exports (config.json "orders": source shop) — shop missing:
	// the screen degrades (plugins.has), writes and the export answer with an error instead of failing
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=orders")
	if !strings.Contains(html, `data-ac="unavailable"`) || !strings.Contains(html, "shop&quot;") || strings.Contains(html, `data-ac="list"`) {
		t.Fatalf("orders without shop:\n%s", html)
	}
	if code, out := call("admin-crud:save", map[string]any{"resource": "orders", "values": map[string]any{"item": "X", "status": "paid"}}); code != 200 ||
		!strings.Contains(fmt.Sprint(out["errors"]), "shop") {
		t.Fatalf("save without shop: %d %v", code, out)
	}
	if rec := adminDownload(t, e, admin, "admin-crud:data", "r=orders"); rec.Code != 409 {
		t.Fatalf("export without shop: %d %s", rec.Code, rec.Body.String())
	}
	// with shop active: listed through plugins.call(shop.listOrders), edited through saveOrder
	installSample(t, admin, "money", "shop")
	if code, _ := getJSON(t, e.c, "POST", "/shop/order", `{"id":1}`, nil); code != 200 {
		t.Fatal("order")
	}
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=orders")
	oid := regexp.MustCompile(`<tr data-id="(\d+)"><td>\d+</td><td>Mug</td><td>250</td><td>paid</td>`).FindStringSubmatch(html)
	if oid == nil || !strings.Contains(html, "1 รายการ") || strings.Contains(html, `class="danger"`) || !strings.Contains(html, "edit="+oid[1]) {
		t.Fatalf("orders from shop:\n%s", html)
	}
	var on float64
	fmt.Sscan(oid[1], &on)
	html, _ = adminPage(t, e, admin, "admin-crud:data", "r=orders&edit="+oid[1])
	if !strings.Contains(html, `value="Mug"`) || !strings.Contains(html, `<option selected>paid</option>`) {
		t.Fatalf("order edit form:\n%s", html)
	}
	if code, out := call("admin-crud:save", map[string]any{"resource": "orders", "id": on, "values": map[string]any{"item": "Mug", "total": "250", "status": "cancelled"}}); code != 200 || out["errors"] != nil {
		t.Fatalf("save order: %d %v", code, out)
	}
	if p := sampleRoute(t, e.c, "/shop?order="+oid[1])["pay"].(map[string]any); p["status"] != "cancelled" {
		t.Fatalf("order after save: %v", p)
	}
	if code, out := call("admin-crud:save", map[string]any{"resource": "orders", "values": map[string]any{"item": "Pen", "total": "20", "status": "pending"}}); code != 200 || out["data"] == nil {
		t.Fatalf("create order: %d %v", code, out)
	}
	if code, out := call("admin-crud:delete", map[string]any{"resource": "orders", "id": on}); code != 200 || out["errors"] == nil {
		t.Fatalf("delete is not offered by shop: %d %v", code, out)
	}
	if rec := adminDownload(t, e, admin, "admin-crud:data", "r=orders&q=pen"); rec.Code != 200 || !strings.Contains(rec.Body.String(), ",Pen,20,pending,") || strings.Contains(rec.Body.String(), "Mug") {
		t.Fatalf("orders export: %d %q", rec.Code, rec.Body.String())
	}

	// the settings override (JSON) replaces config.json
	admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": "admin-crud", "values": []any{map[string]any{"key": "resources", "secret": false,
		"value": `[{"id":"faq","table":"faqs","label":"Only FAQ","columns":[{"name":"question","list":true}]}]`}}})
	html, _ = adminPage(t, e, admin, "admin-crud:data", "")
	if !strings.Contains(html, "Only FAQ") || strings.Contains(html, "รายชื่อติดต่อ") || !strings.Contains(html, "Q1") {
		t.Fatalf("resources setting:\n%s", html)
	}
}

// ---------------------------------------------------------------- members

func cookieOf(rec *httptest.ResponseRecorder, name string) (string, bool) {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c.Value, true
		}
	}
	return "", false
}

func TestSampleMembers(t *testing.T) {
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "members")
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /vip', { access: ['group:verified'] }, ({ visitor }) => ({ id: visitor.id, name: visitor.name, groups: visitor.groups }));
site.route('GET /who', { access: ['*'] }, ({ visitor }) => ({ visitor }));`})
	post := func(path, body, cookie string) (int, map[string]any, *httptest.ResponseRecorder) {
		h := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
		if cookie != "" {
			h["Cookie"] = cookie
		}
		rec := e.c.send("POST", path, strings.NewReader(body), h)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out, rec
	}
	get := func(path, cookie string) (int, map[string]any) {
		code, b := withCookie(e.c, "GET", path, cookie)
		out := map[string]any{}
		_ = json.Unmarshal([]byte(b), &out)
		return code, out
	}

	// a cross-site form cannot post (JSON only)
	if rec := e.c.send("POST", "/members/register", strings.NewReader("email=a@b.co"), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}); rec.Code != 415 {
		t.Fatalf("form post: %d", rec.Code)
	}
	if code, out, _ := post("/members/register", `{"email":"bad","password":"short"}`, ""); code != 400 || out["errors"].(map[string]any)["email"] == nil || out["errors"].(map[string]any)["password"] == nil {
		t.Fatalf("invalid register: %d %v", code, out)
	}
	code, out, _ := post("/members/register", `{"email":"Somchai@Example.com","password":"s3cret-pass","name":"สมชาย"}`, "")
	if code != 200 || out["ok"] != true || out["data"].(map[string]any)["mailSent"] != true {
		t.Fatalf("register: %d %v", code, out)
	}
	if code, out, _ := post("/members/register", `{"email":"somchai@example.com","password":"s3cret-pass"}`, ""); code != 400 {
		t.Fatalf("duplicate: %d %v", code, out)
	}
	mails := e.mailbox()
	if len(mails) != 1 || mails[0].To[0] != "somchai@example.com" || !strings.Contains(mails[0].Subject, "Confirm") {
		t.Fatalf("mails: %+v", mails)
	}
	link := regexp.MustCompile(`https?://[^/\s]+(/members/verify\?token=[0-9a-f]{64})`).FindStringSubmatch(mails[0].Text)
	if link == nil {
		t.Fatalf("no verify link in %q", mails[0].Text)
	}
	// not verified yet: login refused (requireVerification defaults to true)
	if code, out, _ := post("/members/login", `{"email":"somchai@example.com","password":"s3cret-pass"}`, ""); code != 400 || !strings.Contains(fmt.Sprint(out["message"]), "ยืนยันอีเมล") {
		t.Fatalf("login before verify: %d %v", code, out)
	}
	if rec := e.c.send("GET", link[1], nil, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "ยืนยันอีเมลแล้ว") {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.c.send("GET", link[1], nil, nil); rec.Code != 400 {
		t.Fatalf("verify link reused: %d", rec.Code)
	}
	if code, _, _ := post("/members/login", `{"email":"somchai@example.com","password":"wrong-pass"}`, ""); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, _, _ := post("/members/login", `{"email":"nobody@example.com","password":"wrong-pass"}`, ""); code != 401 {
		t.Fatalf("unknown email: %d", code)
	}
	code, out, rec := post("/members/login", `{"email":"SOMCHAI@example.com","password":"s3cret-pass"}`, "")
	sid, ok := cookieOf(rec, "pl_members_sid")
	if code != 200 || !ok || len(sid) != 64 || fmt.Sprint(out["data"].(map[string]any)["groups"]) != "[member verified]" {
		t.Fatalf("login: %d %v %v", code, out, rec.Header())
	}
	cookie := "pl_members_sid=" + sid
	// the visitor resolved by the members unit reaches Site Code (access group:verified)
	if code, _ := get("/vip", ""); code != 401 {
		t.Fatalf("/vip anonymous: %d", code)
	}
	code, vip := get("/vip", cookie)
	if code != 200 || vip["name"] != "สมชาย" || fmt.Sprint(vip["groups"]) != "[member verified]" {
		t.Fatalf("/vip: %d %v", code, vip)
	}
	if _, me := get("/members/me", cookie); me["visitor"].(map[string]any)["email"] != "somchai@example.com" || me["visitor"].(map[string]any)["verified"] != true {
		t.Fatalf("/members/me: %v", me)
	}
	if _, who := get("/who", "pl_members_sid="+strings.Repeat("0", 64)); who["visitor"] != nil {
		t.Fatalf("forged session: %v", who)
	}
	// "my profile" collection as that visitor
	_, pv := admin.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:members:me", "viewAs": "anonymous"})
	if pv["denied"] != true && len(pv["items"].([]any)) != 0 {
		t.Fatalf("me collection for anonymous: %v", pv)
	}
	// admin screen
	html, call := adminPage(t, e, admin, "members:list", "q=somchai")
	if !strings.Contains(html, `<table data-members="list">`) || !strings.Contains(html, "somchai@example.com") ||
		!strings.Contains(html, "<title>Members</title>") || !strings.Contains(html, "Export CSV") || !strings.Contains(html, `data-members="export"`) {
		t.Fatalf("members admin:\n%s", html)
	}
	if html, _ := adminPage(t, e, admin, "members:list", "lang=th"); !strings.Contains(html, "<title>สมาชิก</title>") || !strings.Contains(html, "ส่งออก CSV") {
		t.Fatalf("members admin (?lang=th):\n%s", html)
	}
	// Export CSV through the admin page's download handler: no password hashes or tokens
	dl := adminDownload(t, e, admin, "members:list", "q=somchai")
	csv := dl.Body.String()
	if dl.Code != 200 || !strings.HasPrefix(dl.Header().Get("Content-Disposition"), `attachment; filename="members.csv"`) ||
		!strings.HasPrefix(csv, "\xef\xbb\xbf#,Email,Name,Status,Verified,Groups,Last login,Registered\r\n") || strings.Count(csv, "\r\n") != 2 ||
		!strings.Contains(csv, ",somchai@example.com,สมชาย,active,yes,member verified,") || strings.Contains(csv, "$2") {
		t.Fatalf("members csv: %d %v\n%s", dl.Code, dl.Header(), csv)
	}
	mid := vip["id"].(float64)
	// logout ends the session
	code, _, rec = post("/members/logout", `{}`, cookie)
	if v, ok := cookieOf(rec, "pl_members_sid"); code != 200 || !ok || v != "" {
		t.Fatalf("logout: %d %v", code, rec.Header())
	}
	if code, _ := get("/vip", cookie); code != 401 {
		t.Fatalf("/vip after logout: %d", code)
	}
	// password reset by mail (same answer for unknown emails)
	if code, out, _ := post("/members/forgot", `{"email":"nobody@example.com"}`, ""); code != 200 || len(e.mailbox()) != 1 {
		t.Fatalf("forgot unknown: %d %v", code, out)
	}
	if code, _, _ := post("/members/forgot", `{"email":"somchai@example.com"}`, ""); code != 200 {
		t.Fatal("forgot")
	}
	mails = e.mailbox()
	tok := regexp.MustCompile(`/reset-password\?token=([0-9a-f]{64})`).FindStringSubmatch(mails[len(mails)-1].Text)
	if len(mails) != 2 || tok == nil {
		t.Fatalf("reset mail: %+v", mails)
	}
	if code, out, _ := post("/members/reset", `{"token":"`+tok[1]+`","password":"new-pass-123"}`, ""); code != 200 {
		t.Fatalf("reset: %d %v", code, out)
	}
	if code, _, _ := post("/members/login", `{"email":"somchai@example.com","password":"s3cret-pass"}`, ""); code != 401 {
		t.Fatal("old password still works")
	}
	code, _, rec = post("/members/login", `{"email":"somchai@example.com","password":"new-pass-123"}`, "")
	sid2, _ := cookieOf(rec, "pl_members_sid")
	if code != 200 || sid2 == "" {
		t.Fatalf("login with the new password: %d", code)
	}
	// disabling a member drops their sessions and blocks login
	if code, out := call("members:setStatus", map[string]any{"id": mid, "status": "disabled"}); code != 200 || out["data"].(map[string]any)["changed"].(float64) != 1 {
		t.Fatalf("disable: %d %v", code, out)
	}
	if code, _ := get("/vip", "pl_members_sid="+sid2); code != 401 {
		t.Fatalf("/vip of a disabled member: %d", code)
	}
	if code, out, _ := post("/members/login", `{"email":"somchai@example.com","password":"new-pass-123"}`, ""); code != 401 || !strings.Contains(fmt.Sprint(out["message"]), "ระงับ") {
		t.Fatalf("login when disabled: %d %v", code, out)
	}
	// allowed domains
	admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": "members", "values": []any{map[string]any{"key": "allowedDomains", "secret": false, "value": "corp.co.th"}}})
	if code, out, _ := post("/members/register", `{"email":"x@gmail.com","password":"s3cret-pass"}`, ""); code != 400 || out["errors"].(map[string]any)["email"] == nil {
		t.Fatalf("domain not allowed: %d %v", code, out)
	}
	if code, _, _ := post("/members/register", `{"email":"x@corp.co.th","password":"s3cret-pass"}`, ""); code != 200 {
		t.Fatal("allowed domain refused")
	}
	// one site.visitor per site: a second resolver in Site Code no longer builds
	if code, out := admin.do("PUT", "/api/v1/code/files/main.ts", map[string]any{"content": "site.visitor(() => null)", "baseUpdatedAt": filesUpdatedAt(t, admin, "main.ts")}); code != 200 {
		t.Fatal(out)
	}
	if _, b := admin.do("POST", "/api/v1/code/build", nil); b["ok"] != false || !strings.Contains(fmt.Sprint(b["errors"]), "members") {
		t.Fatalf("second visitor: %v", b)
	}
}

func filesUpdatedAt(t *testing.T, admin *client, p string) string {
	t.Helper()
	_, f := admin.do("GET", "/api/v1/code/files/"+p, nil)
	s, _ := f["updatedAt"].(string)
	return s
}

// ---------------------------------------------------------------- shop (cron)

func TestSampleShopCron(t *testing.T) {
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "money", "shop")
	if code, _ := getJSON(t, e.c, "POST", "/shop/order", `{"id":1}`, nil); code != 200 {
		t.Fatal("paid order")
	}
	opts, _ := e.st.Options()
	run := func() {
		s := e.code.LiveSession(httptest.NewRequest("POST", "/_a/shop:order", nil), opts)
		defer s.Close()
		if out := s.RunAction("shop:order", map[string]any{"id": float64(2)}); out.Status != 200 {
			t.Fatalf("order action: %+v", out)
		}
	}
	run() // pending, old
	run() // pending, fresh
	tbl := "`" + e.code.Data().Database + "`.`" + e.code.Data().N.Prefix + "p_shop_orders`"
	if _, err := e.st.DB.Exec("UPDATE " + tbl + " SET created_at = created_at - INTERVAL 2 HOUR WHERE id = (SELECT id FROM (SELECT MIN(id) id FROM " + tbl + " WHERE status = 'pending') x)"); err != nil {
		t.Fatal(err)
	}
	_, cr := admin.doAny("GET", "/api/v1/code/cron?unit=shop", nil)
	if !strings.Contains(fmt.Sprint(cr), "*/5 * * * *") || !strings.Contains(fmt.Sprint(cr), "shop:cancelStale") {
		t.Fatalf("cron: %v", cr)
	}
	minute := time.Now().UTC().Truncate(5 * time.Minute)
	if n := e.code.CronTick(minute); n != 1 {
		t.Fatalf("cron runs started: %d", n)
	}
	if n := e.code.ProcessJobs(context.Background(), 5); n != 1 {
		t.Fatalf("jobs processed: %d", n)
	}
	rows, err := e.st.DB.Query("SELECT status FROM " + tbl + " ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		got = append(got, s)
	}
	rows.Close()
	if strings.Join(got, ",") != "paid,cancelled,pending" {
		t.Fatalf("order statuses: %v", got)
	}
	_, jl := admin.do("GET", "/api/v1/code/jobs?unit=shop", nil)
	if it := jl["items"].([]any); len(it) != 1 || it[0].(map[string]any)["status"] != "done" || fmt.Sprint(it[0].(map[string]any)["result"]) != "map[cancelled:1]" {
		t.Fatalf("jobs: %v", jl)
	}
	e.code.WaitEvents()
}

// ---------------------------------------------------------------- shop + optional thai / promptpay

// shop lists thai and promptpay in "optionalRequires": it installs without them, and uses them through
// plugins.has / plugins.call once they are active (and degrades again when one goes away).
func TestSampleShopWorksWith(t *testing.T) {
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "money", "shop")
	if p := pkgOf(t, admin, "shop"); fmt.Sprint(p["optionalRequires"]) != "map[promptpay:^1.0.0 thai:^1.0.0]" {
		t.Fatalf("shop package: optionalRequires = %v", p["optionalRequires"])
	}
	if w := sampleRoute(t, e.c, "/shop")["worksWith"]; fmt.Sprint(w) != "map[promptpay:false thai:false]" {
		t.Fatalf("worksWith without the optional plugins: %v", w)
	}
	_, o := getJSON(t, e.c, "POST", "/shop/order", `{"id":1}`, nil)
	if o["total"] != 250.0 || o["label"] != "฿250.00" || o["words"] != nil {
		t.Fatalf("order answer: %v", o)
	}
	pay := func() map[string]any {
		return sampleRoute(t, e.c, "/shop?order="+fmt.Sprint(o["id"]))["pay"].(map[string]any)
	}
	if p := pay(); p["total"] != 250.0 || p["label"] != "฿250.00" || p["words"] != nil || p["promptpay"] != nil {
		t.Fatalf("pay without thai/promptpay: %v", p)
	}
	installSample(t, admin, "thai", "promptpay")
	if w := sampleRoute(t, e.c, "/shop")["worksWith"]; fmt.Sprint(w) != "map[promptpay:true thai:true]" {
		t.Fatalf("worksWith: %v", w)
	}
	// thai: the amount in words; promptpay: nothing until the shop's PromptPay ID is set
	if p := pay(); p["words"] != "สองร้อยห้าสิบบาทถ้วน" || p["promptpay"] != nil {
		t.Fatalf("pay with thai: %v", p)
	}
	if code, out := admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": "promptpay",
		"values": []any{map[string]any{"key": "promptpayId", "value": "0812345678", "secret": false}}}); code != 200 {
		t.Fatalf("settings: %d %v", code, out)
	}
	pp, _ := pay()["promptpay"].(map[string]any)
	payload, _ := pp["payload"].(string)
	if pp == nil || !strings.Contains(payload, "01130066812345678") || !strings.Contains(payload, "5406250.00") ||
		len(payload) < 8 || crcCCITTFalse(payload[:len(payload)-4]) != payload[len(payload)-4:] {
		t.Fatalf("promptpay: %v", pp)
	}
	if svg, _ := pp["svg"].(string); !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200"`) || pp["url"] != "/promptpay/qr.svg?amount=250.00" {
		t.Fatalf("promptpay svg/url: %.120v %v", pp["svg"], pp["url"])
	}
	// thai switched off: back to no words (plugins.has is false), the rest keeps working
	if code, out := admin.do("POST", "/api/v1/package-plugins/thai/deactivate", nil); code != 200 {
		t.Fatalf("deactivate thai: %d %v", code, out)
	}
	if p := pay(); p["words"] != nil || p["promptpay"] == nil || p["total"] != 250.0 {
		t.Fatalf("pay after thai was deactivated: %v", p)
	}
}
