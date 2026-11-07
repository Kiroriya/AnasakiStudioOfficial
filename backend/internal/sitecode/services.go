package sitecode

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 4a services: mail.send, http.fetch and site.settings.

// MailMessage is one mail.send message.
type MailMessage struct {
	To      []string
	Subject string
	HTML    string
	Text    string
	ReplyTo string
}

const (
	maxMailRecipients = 20
	maxFetchBytes     = 5 << 20
	defaultFetchMs    = 5000
	maxFetchMs        = 10000
)

// ---------------------------------------------------------------- mail.send

func (v *vm) mailMessage(x goja.Value, what string) MailMessage {
	o := optObject(v.rt, x)
	if o == nil {
		v.throw("%s: ต้องระบุ { to, subject, html | text }", what)
	}
	var m MailMessage
	switch t := o.Get("to").Export().(type) {
	case string:
		m.To = []string{t}
	case []any:
		for _, a := range t {
			s, ok := a.(string)
			if !ok {
				v.throw("%s: to ต้องเป็นอีเมลหรือ array ของอีเมล", what)
			}
			m.To = append(m.To, s)
		}
	default:
		v.throw("%s: to ต้องเป็นอีเมลหรือ array ของอีเมล", what)
	}
	m.Subject, m.HTML, m.Text, m.ReplyTo = optString(o, "subject"), optString(o, "html"), optString(o, "text"), optString(o, "replyTo")
	return m
}

func (v *vm) checkMail(m *MailMessage, what string) {
	if len(m.To) == 0 {
		v.throw("%s: ไม่มีผู้รับ", what)
	}
	if len(m.To) > maxMailRecipients {
		v.throw("%s: ส่งได้ไม่เกิน %d ผู้รับต่อครั้ง (ใช้ jobs สำหรับการส่งจำนวนมาก)", what, maxMailRecipients)
	}
	for i, t := range m.To {
		t = strings.TrimSpace(t)
		if a, err := mail.ParseAddress(t); err != nil || a.Address != t {
			v.throw("%s: อีเมลผู้รับไม่ถูกต้อง: %s", what, clip(t, 100))
		}
		m.To[i] = t
	}
	if m.ReplyTo != "" {
		if _, err := mail.ParseAddress(m.ReplyTo); err != nil {
			v.throw("%s: replyTo ไม่ถูกต้อง", what)
		}
	}
	if strings.TrimSpace(m.Subject) == "" || len(m.Subject) > 300 || strings.ContainsAny(m.Subject, "\r\n") {
		v.throw("%s: subject ต้องไม่ว่าง (ไม่เกิน 300 ตัวอักษร บรรทัดเดียว)", what)
	}
	if m.HTML == "" && m.Text == "" {
		v.throw("%s: ต้องมี html หรือ text", what)
	}
	if len(m.HTML)+len(m.Text) > 2<<20 {
		v.throw("%s: เนื้อหาอีเมลใหญ่เกิน 2 MB", what)
	}
}

// mailReady is mail.ready() (2.5.0): true when the site can send email (Settings → Email set up), so a
// plugin can skip a step that needs it (Members' email verification) instead of leaving people stuck.
func (v *vm) mailReady(goja.FunctionCall) goja.Value {
	e := v.h.e
	if e == nil || e.cfg.Demo {
		return v.rt.ToValue(false)
	}
	if e.cfg.MailReady != nil {
		return v.rt.ToValue(e.cfg.MailReady())
	}
	return v.rt.ToValue(e.cfg.Mailer != nil)
}

func (v *vm) mailSend(c goja.FunctionCall) goja.Value {
	const what = "mail.send"
	v.handlerOnly(what)
	v.need("mail", what)
	e := v.h.e
	if e == nil || e.cfg.Demo {
		v.fail("%s: ปิดในเว็บทดลอง", what)
	}
	m := v.mailMessage(c.Argument(0), what)
	v.checkMail(&m, what)
	if len(v.filtersFor("mail.beforeSend")) > 0 {
		val := map[string]any{"to": m.To, "subject": m.Subject, "html": m.HTML, "text": m.Text, "replyTo": m.ReplyTo}
		ctx := v.toJS(map[string]any{"source": v.source}).ToObject(v.rt)
		_ = ctx.Set("user", v.userValue())
		v.defineVisitor(ctx)
		out := v.applyNested("mail.beforeSend", v.toJS(val), ctx)
		if out == nil || goja.IsNull(out) {
			v.addLogLine("info", fmt.Sprintf("mail.send ถูกยกเลิกโดย mail.beforeSend (ถึง %s)", strings.Join(m.To, ", ")))
			return v.toJS(map[string]any{"ok": false, "cancelled": true})
		}
		m = v.mailMessage(out, what+" (mail.beforeSend)")
		v.checkMail(&m, what+" (mail.beforeSend)")
	}
	if e.st != nil {
		if opts, err := e.st.Options(); err == nil {
			e.applyLimits(opts)
		}
	}
	if !e.mailLimit.Allow("mail") {
		v.fail("%s: ส่งเกินโควตาต่อชั่วโมงแล้ว (ปรับได้ในหน้า Site code → ค่าลับและการตั้งค่า)", what)
	}
	if e.cfg.Mailer == nil {
		v.fail("%s: ยังไม่ได้ตั้งค่าระบบอีเมล (ตั้งค่า → อีเมล)", what)
	}
	if err := e.cfg.Mailer(m); err != nil {
		v.fail("%s: %s", what, err.Error())
	}
	v.addLogLine("info", fmt.Sprintf("mail.send → %s (%d ผู้รับ) หัวข้อ: %s", strings.Join(m.To, ", "), len(m.To), clip(m.Subject, 120)))
	return v.toJS(map[string]any{"ok": true})
}

// ---------------------------------------------------------------- http.fetch

// fetchHook lets tests point allowlisted host names at a local TLS server
// (bypassing the public-address guard for exactly those addresses).
type fetchHook struct {
	addrs   map[string]string // "host:443" → "127.0.0.1:port"
	rootCAs *tls.Config
	hosts   []string // allowlist override (nil = the site setting)
}

func (e *Engine) fetchHosts() []string {
	if e.fetchHook != nil && e.fetchHook.hosts != nil {
		return e.fetchHook.hosts
	}
	if e.st == nil {
		return nil
	}
	opts, err := e.st.Options()
	if err != nil {
		return nil
	}
	return FetchHosts(opts)
}

// fetchClient returns a client that dials public addresses only, follows
// at most 5 redirects on the same host over https and never uses a proxy.
func (e *Engine) fetchClient(host string) *http.Client {
	dialer := plugin.PublicDialer(5 * time.Second)
	tr := &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 5 * time.Second,
		DialContext: dialer.DialContext, MaxResponseHeaderBytes: 64 << 10}
	if h := e.fetchHook; h != nil {
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if real, ok := h.addrs[addr]; ok {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, real)
			}
			return dialer.DialContext(ctx, network, addr)
		}
		if h.rootCAs != nil {
			tr.TLSClientConfig = h.rootCAs.Clone()
		}
	}
	return &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("redirect มากเกิน 5 ครั้ง")
		}
		if req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Hostname(), host) {
			return fmt.Errorf("ไม่ตาม redirect ไปยัง host อื่น (%s)", req.URL.Host)
		}
		return nil
	}}
}

var fetchMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true}

var fetchDropHeaders = map[string]bool{"host": true, "content-length": true, "connection": true, "transfer-encoding": true,
	"keep-alive": true, "upgrade": true, "proxy-authorization": true, "te": true, "trailer": true}

func (v *vm) httpFetch(c goja.FunctionCall) goja.Value {
	const what = "http.fetch"
	v.handlerOnly(what)
	v.need("http", what)
	e := v.h.e
	if e == nil {
		v.fail("%s: ไม่พร้อมใช้งาน", what)
	}
	raw, _ := c.Argument(0).Export().(string)
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		v.fail("%s: ใช้ได้เฉพาะ URL https:// (ไม่รับ http หรือ user:password@)", what)
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		v.fail("%s: ใช้ชื่อโดเมนเท่านั้น (ไม่รับ IP)", what)
	}
	if !hostAllowed(host, e.fetchHosts()) {
		v.fail("%s: host %q ไม่อยู่ในรายการที่อนุญาต (ตั้งค่าในหน้า Site code → ค่าลับและการตั้งค่า)", what, host)
	}
	if v.u.plugin() && !hostAllowed(host, v.u.hosts) { // plugins: also within code.hosts
		v.fail("%s: host %q ไม่อยู่ใน code.hosts ของปลั๊กอิน %s", what, host, v.u.ID)
	}
	method, timeoutMs := "GET", defaultFetchMs
	hdr := http.Header{}
	var body io.Reader
	if o := optObject(v.rt, c.Argument(1)); o != nil {
		if m := optString(o, "method"); m != "" {
			method = strings.ToUpper(m)
			if !fetchMethods[method] {
				v.fail("%s: method %q ไม่รองรับ", what, m)
			}
		}
		if t := o.Get("timeout"); t != nil && !goja.IsUndefined(t) && !goja.IsNull(t) {
			timeoutMs = int(t.ToInteger())
			if timeoutMs < 1 || timeoutMs > maxFetchMs {
				v.fail("%s: timeout ต้องอยู่ระหว่าง 1 ถึง %d ms", what, maxFetchMs)
			}
		}
		if h := optObject(v.rt, o.Get("headers")); h != nil {
			for _, k := range h.Keys() {
				val := h.Get(k).String()
				if fetchDropHeaders[strings.ToLower(k)] || strings.ContainsAny(k, "\r\n: ") || strings.ContainsAny(val, "\r\n") {
					continue
				}
				hdr.Set(k, val)
			}
		}
		if b := o.Get("body"); b != nil && !goja.IsUndefined(b) && !goja.IsNull(b) {
			if s, ok := b.Export().(string); ok {
				body = strings.NewReader(s)
			} else {
				s, ok, err := v.toJSON(b)
				if err != nil || !ok {
					v.fail("%s: body ต้องเป็นข้อความหรือ object ที่แปลงเป็น JSON ได้", what)
				}
				body = strings.NewReader(s)
				if hdr.Get("Content-Type") == "" {
					hdr.Set("Content-Type", "application/json")
				}
			}
		}
	}
	d := time.Duration(timeoutMs) * time.Millisecond
	if !v.deadline.IsZero() {
		rem := time.Until(v.deadline) - 20*time.Millisecond
		if rem <= 0 {
			v.fail("%s: หมดเวลาของ handler แล้ว", what)
		}
		d = min(d, rem)
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		v.fail("%s: URL ไม่ถูกต้อง", what)
	}
	req.Header = hdr
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "AnasakiStudio-SiteCode/1")
	}
	resp, err := e.fetchClient(host).Do(req)
	if err != nil {
		switch {
		case plugin.IsPrivateAddrErr(err):
			v.fail("%s: %s ชี้ไปยังที่อยู่ภายใน (private / loopback) — ไม่อนุญาต", what, host)
		case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
			v.fail("%s: หมดเวลา (%d ms)", what, d.Milliseconds())
		}
		v.fail("%s: เชื่อมต่อไม่สำเร็จ: %s", what, err.Error())
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			v.fail("%s: หมดเวลา (%d ms)", what, d.Milliseconds())
		}
		v.fail("%s: อ่านคำตอบไม่สำเร็จ: %s", what, err.Error())
	}
	if len(b) > maxFetchBytes {
		v.fail("%s: คำตอบใหญ่เกิน 5 MB", what)
	}
	headers := map[string]string{}
	for k, vals := range resp.Header {
		headers[strings.ToLower(k)] = strings.Join(vals, ", ")
	}
	text := string(b)
	out := v.toJS(map[string]any{"status": resp.StatusCode, "ok": resp.StatusCode >= 200 && resp.StatusCode < 300,
		"headers": headers, "body": text, "url": resp.Request.URL.String()}).ToObject(v.rt)
	_ = out.Set("json", func(goja.FunctionCall) goja.Value {
		val, err := v.parse(goja.Undefined(), v.rt.ToValue(text))
		if err != nil {
			v.throw("http.fetch: คำตอบไม่ใช่ JSON ที่ถูกต้อง")
		}
		return val
	})
	_ = out.Set("text", func(goja.FunctionCall) goja.Value { return v.rt.ToValue(text) })
	return out
}
