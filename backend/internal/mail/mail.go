// Package mail sends email through the SMTP server configured in
// Settings → Email (WordPress needs a plugin for this; here it is built in).
package mail

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Option names (all strings, stored with the other settings).
const (
	OptHost       = "smtp_host"
	OptPort       = "smtp_port"
	OptUser       = "smtp_user"
	OptPassword   = "smtp_password"
	OptEncryption = "smtp_encryption" // starttls | ssl | none
	OptFromEmail  = "mail_from_email"
	OptFromName   = "mail_from_name"
)

// ErrNotConfigured means no SMTP server has been set up yet.
var ErrNotConfigured = errors.New("ยังไม่ได้ตั้งค่าระบบอีเมล (ตั้งค่า → อีเมล)")

// Config is an SMTP account.
type Config struct {
	Host, User, Password, Encryption string
	Port                             int
	From                             mail.Address
}

// FromOptions builds the config from site options. The sender falls back to
// admin_email and the site title.
func FromOptions(o map[string]string) (Config, error) {
	c := Config{Host: strings.TrimSpace(o[OptHost]), User: o[OptUser], Password: o[OptPassword],
		Encryption: strings.ToLower(o[OptEncryption])}
	if c.Encryption == "" {
		c.Encryption = "starttls"
	}
	c.Port, _ = strconv.Atoi(o[OptPort])
	if c.Port == 0 {
		c.Port = map[string]int{"ssl": 465, "none": 25}[c.Encryption]
		if c.Port == 0 {
			c.Port = 587
		}
	}
	from := strings.TrimSpace(o[OptFromEmail])
	if from == "" {
		from = strings.TrimSpace(o["admin_email"])
	}
	name := strings.TrimSpace(o[OptFromName])
	if name == "" {
		name = o["site_title"]
	}
	c.From = mail.Address{Name: name, Address: from}
	if c.Host == "" || from == "" {
		return c, ErrNotConfigured
	}
	return c, nil
}

// Message is one email. HTML is required; a plain-text part is derived from it.
type Message struct {
	To      []string
	Subject string
	HTML    string
	ReplyTo string
	// Text is the plain-text part (default: derived from HTML); with an empty
	// HTML the text is also sent as the HTML part.
	Text string
}

// Send delivers m through c.
func Send(c Config, m Message) error {
	if c.Host == "" || c.From.Address == "" {
		return ErrNotConfigured
	}
	if len(m.To) == 0 {
		return errors.New("ไม่มีผู้รับ")
	}
	for _, t := range m.To {
		if _, err := mail.ParseAddress(t); err != nil {
			return fmt.Errorf("อีเมลผู้รับไม่ถูกต้อง: %s", t)
		}
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsCfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	if c.Encryption == "ssl" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("เชื่อมต่อเซิร์ฟเวอร์อีเมลไม่ได้: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("เซิร์ฟเวอร์อีเมลตอบกลับผิดรูปแบบ: %w", err)
	}
	defer cl.Close()
	if c.Encryption == "starttls" {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("เซิร์ฟเวอร์อีเมลไม่รองรับ STARTTLS — ลองเปลี่ยนเป็น SSL/TLS")
		}
		if err := cl.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS ไม่สำเร็จ: %w", err)
		}
	}
	if c.User != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.User, c.Password, c.Host)); err != nil {
			return fmt.Errorf("ชื่อผู้ใช้หรือรหัสผ่านอีเมลไม่ถูกต้อง: %w", err)
		}
	}
	if err := cl.Mail(c.From.Address); err != nil {
		return fmt.Errorf("ผู้ส่งถูกปฏิเสธ: %w", err)
	}
	for _, t := range m.To {
		if err := cl.Rcpt(t); err != nil {
			return fmt.Errorf("ผู้รับถูกปฏิเสธ (%s): %w", t, err)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(build(c, m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("ส่งอีเมลไม่สำเร็จ: %w", err)
	}
	return cl.Quit()
}

// build renders the MIME message (multipart/alternative: text + HTML).
func build(c Config, m Message) []byte {
	var b bytes.Buffer
	boundary := "anasaki-" + randHex(12)
	hdr := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	hdr("From", c.From.String())
	hdr("To", strings.Join(m.To, ", "))
	if m.ReplyTo != "" {
		if a, err := mail.ParseAddress(m.ReplyTo); err == nil {
			hdr("Reply-To", a.String())
		}
	}
	hdr("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	hdr("Date", time.Now().Format(time.RFC1123Z))
	hdr("Message-ID", "<"+randHex(16)+"@"+domainOf(c.From.Address)+">")
	hdr("MIME-Version", "1.0")
	hdr("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	part := func(ctype, body string) {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + ctype + "; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString([]byte(body))
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	text, body := m.Text, m.HTML
	if text == "" {
		text = Text(m.HTML)
	}
	if body == "" {
		body = `<div style="white-space:pre-wrap">` + html.EscapeString(m.Text) + `</div>`
	}
	part("text/plain", text)
	part("text/html", body)
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes()
}

var (
	tagRe   = regexp.MustCompile(`(?s)<(style|script)[^>]*>.*?</(style|script)>|<[^>]+>`)
	blockRe = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr)>|<br\s*/?>`)
	linkRe  = regexp.MustCompile(`(?i)<a [^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	spaceRe = regexp.MustCompile(`[ \t]+`)
	nlRe    = regexp.MustCompile(`\n{3,}`)
)

// Text turns an HTML email into readable plain text.
func Text(h string) string {
	s := linkRe.ReplaceAllString(h, "$2 ($1)")
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRe.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(nlRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
