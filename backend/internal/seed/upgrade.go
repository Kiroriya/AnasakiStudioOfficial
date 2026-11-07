package seed

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/store"
)

// One-off upgrades of designs made by older versions, run at start once each
// (option upgrade_<version>_<name> = "1").

// archiveSubtitleTH is the subtitle the archive template was seeded with
// before 2.5.0; it read "all posts in this category" on content type archives
// (/menu, /events) too. 2.5.0 puts {{post.excerpt}} there (the category's
// description or that sentence; empty on a type's archive).
const archiveSubtitleTH = "บทความทั้งหมดในหมวดหมู่นี้"

// Upgrade runs the pending one-off design upgrades.
func Upgrade(st *store.Store) error {
	opts, err := st.Options()
	if err != nil {
		return err
	}
	const flag = "upgrade_250_archive"
	if opts[flag] == "1" {
		return nil
	}
	if id, err := strconv.ParseInt(opts["post_template_id"], 10, 64); err == nil && id > 0 {
		if p, err := st.GetPost(id); err == nil && len(p.Design) > 0 {
			if out, changed := UpgradeSingleDesign(p.Design); changed {
				if err := st.UpgradePostDesign(id, out); err != nil {
					return err
				}
				log.Printf("upgrade 2.5.0: post template %d (tags)", id)
			}
		}
	}
	if id, err := strconv.ParseInt(opts["archive_template_id"], 10, 64); err == nil && id > 0 {
		if p, err := st.GetPost(id); err == nil && len(p.Design) > 0 {
			if out, changed := UpgradeArchiveDesign(p.Design); changed {
				if err := st.UpgradePostDesign(id, out); err != nil {
					return err
				}
				log.Printf("upgrade 2.5.0: archive template %d (subtitle, card fields)", id)
			}
		}
	}
	return st.SetOptions(map[string]string{flag: "1"})
}

// UpgradeArchiveDesign rewrites an archive template made before 2.5.0: the
// fixed subtitle becomes {{post.excerpt}}, and in its post lists the card's
// date shows on blog posts only, followed by a compact Fields block for
// content types (price, date of an event …). changed is false when nothing
// matched (a template the owner rebuilt).
func UpgradeArchiveDesign(raw []byte) ([]byte, bool) {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return raw, false
	}
	changed := false
	hasPager := strings.Contains(string(raw), `"pagination"`)
	var walk func(n map[string]any, inList bool)
	walk = func(n map[string]any, inList bool) {
		props, _ := n["props"].(map[string]any)
		if n["type"] == "text" && props != nil && props["text"] == archiveSubtitleTH {
			props["text"] = "{{post.excerpt}}"
			changed = true
		}
		kids, _ := n["children"].([]any)
		inList = inList || n["type"] == "postList"
		var out []any
		for _, k := range kids {
			c, ok := k.(map[string]any)
			if !ok {
				out = append(out, k)
				continue
			}
			walk(c, inList)
			out = append(out, c)
			if c["type"] == "postList" && !hasPager { // 2.5.0: page links under the archive's list
				if id, _ := c["id"].(string); id != "" {
					out = append(out, PagerNode(id))
					hasPager, changed = true, true
				}
			}
			cp, _ := c["props"].(map[string]any)
			if inList && c["type"] == "text" && cp != nil && cp["text"] == "{{post.date}}" && cp["showWhen"] == nil {
				cp["showWhen"] = "post.type == 'post'"
				out = append(out, CardFieldsNode())
				changed = true
			}
		}
		if kids != nil {
			n["children"] = out
		}
	}
	walk(root, false)
	if !changed {
		return raw, false
	}
	b, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return b, true
}

// PagerNode is a Pagination block for the post list listID (numbers, 2.5.0).
func PagerNode(listID string) map[string]any {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return map[string]any{"id": "pg" + hex.EncodeToString(b[:]), "type": "pagination", "name": "เลขหน้า",
		"width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
		"layout": map[string]any{"padding": []any{0, 24, 72, 24}},
		"props":  map[string]any{"collection": listID, "style": "numbers"}}
}

// TagsNode is the post's tags block (links to /tag/<slug>, 2.5.0).
func TagsNode() map[string]any {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return map[string]any{"id": "tg" + hex.EncodeToString(b[:]), "type": "postTags", "name": "แท็ก",
		"width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"}}
}

// UpgradeSingleDesign adds the Tags block after the post content of a post template made before 2.5.0
// (changed is false when it has one, or no post content).
func UpgradeSingleDesign(raw []byte) ([]byte, bool) {
	if strings.Contains(string(raw), `"postTags"`) {
		return raw, false
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return raw, false
	}
	done := false
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		kids, _ := n["children"].([]any)
		for i, k := range kids {
			c, ok := k.(map[string]any)
			if !ok || done {
				continue
			}
			if c["type"] == "postContent" {
				out := append(append(append([]any{}, kids[:i+1]...), TagsNode()), kids[i+1:]...)
				n["children"] = out
				done = true
				return
			}
			walk(c)
		}
	}
	walk(root)
	if !done {
		return raw, false
	}
	b, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return b, true
}

// CardFieldsNode is the compact Fields block of a card in a list: the first
// three fields of a content type's post, hidden on blog posts.
func CardFieldsNode() map[string]any {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return map[string]any{"id": "pf" + hex.EncodeToString(b[:]), "type": "postFields", "name": "ข้อมูลเพิ่มเติม",
		"width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
		"style": map[string]any{"fontSize": 14},
		"props": map[string]any{"max": 3, "layout": "inline", "showWhen": "post.type != 'post'"}}
}
