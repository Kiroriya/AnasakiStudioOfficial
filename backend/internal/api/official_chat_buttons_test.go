package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/chat-buttons: the floating menu on public pages with links built from the settings (bad
// values left out), hidden on chosen paths, and the designer block.
func TestOfficialChatButtons(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "chat-buttons")
	if b := e.offBSend(offBReq{path: "/"}).Body.String(); strings.Contains(b, `class="acb `) {
		t.Fatal("menu without any channel")
	}
	offBPkgSettings(t, admin, "chat-buttons", map[string]any{"line": "@myshop", "whatsapp": "+66 81-234-5678", "messenger": "javascript:alert(1)",
		"phone": "081-234-5678", "email": "a@b.co", "label": "แชทกับเรา", "hidePaths": "/secret"})
	home := e.offBSend(offBReq{path: "/"}).Body.String()
	offBContains(t, "home", home, `<details class="acb acb-right">`, `href="https://line.me/R/ti/p/%40myshop"`, `href="https://wa.me/66812345678"`,
		`href="tel:0812345678"`, `href="mailto:a@b.co"`, "แชทกับเรา", "/plugin-assets/chat-buttons/assets/chat.css")
	if strings.Contains(home, "javascript:") || strings.Contains(home, "m.me/") {
		t.Fatal("a bad Messenger value reached the page")
	}
	if b := e.offBSend(offBReq{path: "/secret/page"}).Body.String(); strings.Contains(b, `class="acb `) {
		t.Fatal("shown on a hidden path")
	}
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	if !strings.Contains(fmt.Sprint(blocks), "type:buttons") {
		t.Fatalf("block missing: %v", blocks)
	}
}
