package api

import (
	"encoding/json"
	"net/http"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
)

// Frontend code blocks (design node "codeblock", Site Code phase 5) put
// JavaScript on public pages: only users with edit_scripts may add or change
// one. A save by anyone else that adds or changes a block is refused (403);
// blocks left exactly as stored (another author's) are kept byte for byte.
const codeBlockCap = auth.CapEditScripts

const msgCodeBlockCap = "เพิ่มหรือแก้บล็อกโค้ดหน้าเว็บ (Frontend code) ได้เฉพาะผู้ที่มีสิทธิ์แก้โค้ดของเว็บไซต์ (edit_scripts) / " +
	"Only users with the edit_scripts capability can add or change Frontend code blocks"

// denyCodeBlocks answers and returns true when the design doc (about to be
// saved by the caller) adds or changes a code block the caller may not
// write, or a changed block's CSS is refused. trusted are the stored
// versions the unchanged blocks come from.
func (a *API) denyCodeBlocks(w http.ResponseWriter, r *http.Request, doc json.RawMessage, trusted ...json.RawMessage) bool {
	if len(doc) == 0 || string(doc) == "null" {
		return false
	}
	changed := design.ChangedCodeBlocks(doc, trusted...)
	if len(changed) == 0 {
		return false
	}
	if !auth.From(r).Can(codeBlockCap) {
		ids := make([]string, 0, len(changed))
		for _, b := range changed {
			ids = append(ids, b.ID)
		}
		writeJSON(w, http.StatusForbidden, map[string]any{"error": msgCodeBlockCap, "blocks": ids})
		return true
	}
	if err := design.CheckCodeBlockCSS(changed); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return true
	}
	return false
}
