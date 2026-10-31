package mail

import (
	"bufio"
	"encoding/base64"
	"net"
	"strings"
	"testing"
)

func TestFromOptionsDefaults(t *testing.T) {
	if _, err := FromOptions(map[string]string{}); err != ErrNotConfigured {
		t.Fatalf("empty options should be not configured, got %v", err)
	}
	c, err := FromOptions(map[string]string{OptHost: "smtp.example.com", "admin_email": "a@example.com", "site_title": "Shop"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 587 || c.Encryption != "starttls" || c.From.Address != "a@example.com" || c.From.Name != "Shop" {
		t.Fatalf("defaults: %+v", c)
	}
	c, _ = FromOptions(map[string]string{OptHost: "h", OptFromEmail: "f@x.io", OptEncryption: "ssl"})
	if c.Port != 465 {
		t.Fatalf("ssl port: %d", c.Port)
	}
}

func TestText(t *testing.T) {
	got := Text(`<h1>Hi &amp; welcome</h1><p>Click <a href="https://x.io/r">here</a></p><style>p{}</style>`)
	if got != "Hi & welcome\nClick here (https://x.io/r)" {
		t.Fatalf("Text: %q", got)
	}
}

func TestRenderEscapes(t *testing.T) {
	h := Email{Site: "S", Heading: "<b>", Paragraphs: []string{"a<script>"}, Rows: [][2]string{{"k", "<v>"}}}.Render()
	if strings.Contains(h, "<script>") || strings.Contains(h, "<b>") || !strings.Contains(h, "&lt;v&gt;") {
		t.Fatal("render must escape content")
	}
}

// fakeSMTP is a minimal plain SMTP server that records one message.
func fakeSMTP(t *testing.T) (addr string, got chan string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got = make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		defer ln.Close()
		r := bufio.NewReader(c)
		say := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		say("220 fake")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					say("250 ok")
					got <- data.String()
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				say("250 fake")
			case cmd == "DATA":
				inData = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	return ln.Addr().String(), got
}

func TestSendPlain(t *testing.T) {
	addr, got := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	c, err := FromOptions(map[string]string{OptHost: host, OptPort: port, OptEncryption: "none", OptFromEmail: "site@example.com", OptFromName: "ร้านทดสอบ"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Send(c, Message{To: []string{"user@example.com"}, Subject: "สวัสดี", HTML: "<p>ทดสอบ</p>"}); err != nil {
		t.Fatal(err)
	}
	msg := <-got
	if !strings.Contains(msg, "Subject: =?utf-8?q?") || !strings.Contains(msg, "multipart/alternative") {
		t.Fatalf("headers: %s", msg)
	}
	html := base64.StdEncoding.EncodeToString([]byte("<p>ทดสอบ</p>"))
	if !strings.Contains(msg, html) {
		t.Fatalf("html part missing: %s", msg)
	}
	if err := Send(c, Message{To: []string{"not an email"}, Subject: "x", HTML: "x"}); err == nil {
		t.Fatal("invalid recipient must fail")
	}
}
