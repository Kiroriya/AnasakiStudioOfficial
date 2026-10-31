// Package backup is the core system tool "Export / import site" (a built-in
// plugin before phase 7): it exports the site's content as a zip (data.json +
// the uploaded files) and imports such a zip into a site.
//
// Import ADDS content: every post, term, menu, component and media file is
// created with a new id; existing content is never overwritten or deleted.
// References inside designs (component instances, menus, download media,
// image URLs) and featured images are remapped to the new ids. Limits:
// options/settings are exported for reference but not imported; template
// assignments (post/archive/product template, home page) and links typed by
// hand in menus or HTML keep their old targets; categories that already
// exist (same slug) are reused; users are not exported (imported content
// belongs to the importing administrator).
//
// Since phase 7 data.json also lists the installed package plugins (id,
// version, active, settings without secrets — never their code) and the Site
// Code draft files. Import reports which packages are missing on the target
// (the admin installs them from the Store) and adds Site Code files that do
// not exist yet to the draft (never published automatically, and only for
// an importer with edit_scripts).
package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// Format identifies a backup's data.json.
const Format = "anasaki-backup"

// PostTerm links a post to a term.
type PostTerm struct {
	PostID int64 `json:"postId"`
	TermID int64 `json:"termId"`
}

// Data is data.json.
type Data struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	ExportedAt time.Time         `json:"exportedAt"`
	Site       string            `json:"site"`
	Posts      []*model.Post     `json:"posts"`
	Terms      []model.Term      `json:"terms"`
	PostTerms  []PostTerm        `json:"postTerms"`
	Menus      []model.Menu      `json:"menus"`
	Components []model.Component `json:"components"`
	Options    map[string]string `json:"options"`
	Media      []model.Media     `json:"media"`
	// Packages are the installed package plugins (phase 7; code not included).
	Packages []Package `json:"packages"`
	// SiteCode is the Site Code draft (path → source; phase 7).
	SiteCode map[string]string `json:"siteCode,omitempty"`
}

// Package is an installed package plugin in a backup.
type Package struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Version  string         `json:"version"`
	Active   bool           `json:"active"`
	Source   string         `json:"source"` // store | upload
	Settings map[string]any `json:"settings"`
}

// PublicSettings drops settings whose key looks secret (API keys, tokens…).
func PublicSettings(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if !secretRe.MatchString(k) {
			out[k] = v
		}
	}
	return out
}

var secretRe = regexp.MustCompile(`(?i)secret|token|password|passwd|salt|private|api_?key|smtp|bypass`)

// SecretOption reports options that are never exported (secret-looking
// names, and the stored settings of former built-ins that hold secrets).
func SecretOption(name string) bool { return secretRe.MatchString(name) || legacy.SecretOption(name) }

// Collect gathers everything to export.
func Collect(st *store.Store) (*Data, error) {
	d := &Data{Format: Format, Version: 1, ExportedAt: time.Now().UTC(), Posts: []*model.Post{},
		PostTerms: []PostTerm{}, Options: map[string]string{}, Packages: []Package{}}
	rows, err := st.DB.Query(`SELECT DISTINCT type FROM posts`)
	if err != nil {
		return nil, err
	}
	var types []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			types = append(types, t)
		}
	}
	rows.Close()
	for _, typ := range types {
		for off := 0; ; off += 200 {
			list, _, err := st.ListPosts(model.PostFilter{Type: typ, Order: "oldest", Limit: 200, Offset: off})
			if err != nil {
				return nil, err
			}
			for _, p := range list {
				full, err := st.GetPost(p.ID)
				if err != nil {
					return nil, err
				}
				d.Posts = append(d.Posts, full)
			}
			if len(list) < 200 {
				break
			}
		}
	}
	if d.Terms, err = allTerms(st); err != nil {
		return nil, err
	}
	rows, err = st.DB.Query(`SELECT post_id, term_id FROM post_terms`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pt PostTerm
		if rows.Scan(&pt.PostID, &pt.TermID) == nil {
			d.PostTerms = append(d.PostTerms, pt)
		}
	}
	rows.Close()
	if d.Menus, err = st.ListMenus(); err != nil {
		return nil, err
	}
	if d.Components, err = st.ListComponents(); err != nil {
		return nil, err
	}
	if d.Media, err = st.ListMedia(); err != nil {
		return nil, err
	}
	opts, err := st.Options()
	if err != nil {
		return nil, err
	}
	for k, v := range opts {
		if !SecretOption(k) {
			d.Options[k] = v
		}
	}
	d.Site = opts["site_title"]
	return d, nil
}

func allTerms(st *store.Store) ([]model.Term, error) {
	rows, err := st.DB.Query(`SELECT id, taxonomy, name, slug FROM terms ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Term{}
	for rows.Next() {
		var t model.Term
		if err := rows.Scan(&t.ID, &t.Taxonomy, &t.Name, &t.Slug); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// uploadRel returns a media URL's path relative to the upload dir ("" when
// it is not a local, safe upload path).
func uploadRel(u string) string {
	rel, ok := strings.CutPrefix(u, "/uploads/")
	if !ok || rel == "" || strings.Contains(rel, "\\") || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		return ""
	}
	return rel
}

// WriteZip writes d and the uploaded files it references as a backup zip.
func WriteZip(w io.Writer, d *Data, uploadDir string) error {
	zw := zip.NewWriter(w)
	f, err := zw.CreateHeader(&zip.FileHeader{Name: "data.json", Method: zip.Deflate, Modified: d.ExportedAt})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	if err := enc.Encode(d); err != nil {
		return err
	}
	for _, m := range d.Media {
		rel := uploadRel(m.URL)
		if rel == "" {
			continue
		}
		src, err := os.Open(filepath.Join(uploadDir, filepath.FromSlash(rel)))
		if err != nil {
			continue // missing file: keep the row, skip the bytes
		}
		hdr := &zip.FileHeader{Name: "uploads/" + rel, Method: zip.Store, Modified: m.CreatedAt}
		if !strings.HasPrefix(m.Mime, "image/") {
			hdr.Method = zip.Deflate
		}
		if dst, err := zw.CreateHeader(hdr); err == nil {
			_, _ = io.Copy(dst, src)
		}
		src.Close()
	}
	return zw.Close()
}

// FileName is the download name of a backup made at t.
func FileName(t time.Time) string { return "anasaki-backup-" + t.Format("20060102-1504") + ".zip" }

// ---------------------------------------------------------------- import

// Imported counts what an import created.
type Imported struct {
	Posts      int `json:"posts"`
	Terms      int `json:"terms"`
	Menus      int `json:"menus"`
	Components int `json:"components"`
	Media      int `json:"media"`
}

// MaxImport caps the uploaded backup size.
const MaxImport = 1 << 30

type userError string

func (e userError) Error() string { return string(e) }

// IsUserError reports an import error caused by the file (HTTP 400).
func IsUserError(err error) bool { _, ok := err.(userError); return ok }

// maxUnpacked caps the total size of the media files written by one import.
const maxUnpacked = 4 << 30

// Import reads a backup zip and adds its content to the site. Without
// unfiltered_html the same rules as the editor and media upload apply: post
// HTML is sanitized, raw HTML is stripped from designs and script-capable
// files (html, js, svg…) are skipped.
func Import(st *store.Store, uploadDir string, f multipart.File, hdr *multipart.FileHeader, userID int64, canUnfiltered bool) (Imported, error) {
	return ImportWith(st, uploadDir, f, hdr, userID, canUnfiltered, canUnfiltered)
}

// ImportWith is Import where canCode says whether the importer may add
// Frontend code blocks (edit_scripts); without it their code is emptied.
func ImportWith(st *store.Store, uploadDir string, f multipart.File, hdr *multipart.FileHeader, userID int64, canUnfiltered, canCode bool) (Imported, error) {
	res, _, err := ImportData(st, uploadDir, f, hdr, userID, canUnfiltered, canCode)
	return res, err
}

// ImportData is ImportWith that also returns the backup's data.json (the
// caller handles Packages and SiteCode).
func ImportData(st *store.Store, uploadDir string, f multipart.File, hdr *multipart.FileHeader, userID int64, canUnfiltered, canCode bool) (Imported, *Data, error) {
	res, d, err := importData(st, uploadDir, f, hdr, userID, canUnfiltered, canCode)
	return res, d, err
}

func importData(st *store.Store, uploadDir string, f multipart.File, hdr *multipart.FileHeader, userID int64, canUnfiltered, canCode bool) (res Imported, dp *Data, err error) {
	zr, err := zip.NewReader(f, hdr.Size)
	if err != nil {
		return res, nil, userError("ไฟล์ไม่ใช่ zip ที่ถูกต้อง")
	}
	files := map[string]*zip.File{}
	for _, zf := range zr.File {
		files[zf.Name] = zf
	}
	df := files["data.json"]
	if df == nil {
		return res, nil, userError("ไม่พบ data.json ในไฟล์สำรองข้อมูล")
	}
	rc, err := df.Open()
	if err != nil {
		return res, nil, userError("data.json เสียหาย")
	}
	var d Data
	err = json.NewDecoder(io.LimitReader(rc, 512<<20)).Decode(&d)
	rc.Close()
	if err != nil || d.Format != Format {
		return res, nil, userError("ไฟล์นี้ไม่ใช่ไฟล์สำรองข้อมูลของ Anasaki Studio")
	}

	mp := &mapper{media: map[int64]int64{}, urls: map[string]string{}, comps: map[int64]int64{},
		menus: map[int64]int64{}, terms: map[int64]int64{}}

	// media files
	budget := int64(maxUnpacked)
	for _, m := range d.Media {
		rel := uploadRel(m.URL)
		zf := files["uploads/"+rel]
		if rel == "" || zf == nil || !importableFile(rel, canUnfiltered) {
			continue
		}
		newURL, size, err := copyUpload(uploadDir, rel, zf, &budget)
		if err != nil {
			return res, nil, err
		}
		nm := m
		nm.ID, nm.URL, nm.Size, nm.AuthorID = 0, newURL, size, userID
		if err := st.CreateMedia(&nm); err != nil {
			return res, nil, err
		}
		mp.media[m.ID] = nm.ID
		mp.urls[m.URL] = newURL
		res.Media++
	}
	// terms (reuse same taxonomy+slug)
	for _, t := range d.Terms {
		if ex, err := st.TermBySlug(t.Taxonomy, t.Slug); err == nil {
			mp.terms[t.ID] = ex.ID
			continue
		}
		nt := model.Term{Taxonomy: t.Taxonomy, Name: t.Name, Slug: t.Slug}
		if err := st.CreateTerm(&nt); err != nil {
			return res, nil, err
		}
		mp.terms[t.ID] = nt.ID
		res.Terms++
	}
	// menus (items are links; kept as they are)
	for _, m := range d.Menus {
		nm := model.Menu{Name: m.Name, Items: m.Items}
		if err := st.SaveMenu(&nm); err != nil {
			return res, nil, err
		}
		mp.menus[m.ID] = nm.ID
		res.Menus++
	}
	// components: create, then remap (they may reference each other)
	created := map[int64]*model.Component{}
	for _, c := range d.Components {
		nc := &model.Component{Name: c.Name, Node: cleanDesign(c.Node, canUnfiltered, canCode)}
		if err := st.SaveComponent(nc); err != nil {
			return res, nil, err
		}
		mp.comps[c.ID] = nc.ID
		created[c.ID] = nc
		res.Components++
	}
	for _, c := range d.Components {
		nc := created[c.ID]
		nc.Node = cleanDesign(mp.remapJSON(c.Node), canUnfiltered, canCode)
		if err := st.SaveComponent(nc); err != nil {
			return res, nil, err
		}
	}
	// posts
	postTerms := map[int64][]int64{}
	for _, pt := range d.PostTerms {
		if nt, ok := mp.terms[pt.TermID]; ok {
			postTerms[pt.PostID] = append(postTerms[pt.PostID], nt)
		}
	}
	for _, p := range d.Posts {
		if p == nil || p.Type == "" {
			continue
		}
		slug, err := st.UniqueSlug(p.Type, p.Slug, 0)
		if err != nil {
			return res, nil, err
		}
		content := mp.replaceURLs(p.Content)
		if !canUnfiltered {
			content = kses.SanitizeHTML(content)
		}
		np := &model.Post{Type: p.Type, Title: p.Title, Slug: slug, Status: p.Status, Excerpt: p.Excerpt,
			Content: content, AuthorID: userID, PublishedAt: p.PublishedAt}
		if len(p.Design) > 0 {
			np.Design = cleanDesign(mp.remapJSON(p.Design), canUnfiltered, canCode)
		}
		if p.FeaturedMediaID != nil {
			if id, ok := mp.media[*p.FeaturedMediaID]; ok {
				np.FeaturedMediaID = &id
			}
		}
		if err := st.CreatePost(np, postTerms[p.ID]); err != nil {
			return res, nil, err
		}
		if err := st.SetPostMeta(np.ID, p.Meta); err != nil { // custom fields (Site Code post types)
			return res, nil, err
		}
		res.Posts++
	}
	return res, &d, nil
}

// importableFile reports whether a media file from a backup may be written
// to the upload dir (the media-upload deny list; SVG needs unfiltered_html).
func importableFile(rel string, canUnfiltered bool) bool {
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(rel)), ".")
	if ext == "" || kses.DeniedExt[ext] {
		return false
	}
	return ext != "svg" || canUnfiltered
}

// cleanDesign strips raw HTML from an imported design tree when the importer
// lacks unfiltered_html, and the code of Frontend code blocks without
// edit_scripts.
func cleanDesign(raw json.RawMessage, canUnfiltered, canCode bool) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	if !canCode {
		raw = design.StripCodeBlocks(raw)
	}
	if canUnfiltered {
		return raw
	}
	return design.StripRawHTML(raw)
}

// copyUpload writes a zip entry into the upload dir under a free name and
// returns its new URL and size. budget is the space left for this import.
func copyUpload(uploadDir, rel string, zf *zip.File, budget *int64) (string, int64, error) {
	dir := path.Dir(rel)
	base := path.Base(rel)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	target := rel
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(uploadDir, filepath.FromSlash(target))); os.IsNotExist(err) {
			break
		}
		target = path.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
		if i > 1000 {
			return "", 0, fmt.Errorf("backup: no free name for %s", rel)
		}
	}
	full := filepath.Join(uploadDir, filepath.FromSlash(target))
	if !strings.HasPrefix(full, filepath.Clean(uploadDir)+string(filepath.Separator)) {
		return "", 0, userError("ชื่อไฟล์ในไฟล์สำรองข้อมูลไม่ปลอดภัย")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", 0, err
	}
	rc, err := zf.Open()
	if err != nil {
		return "", 0, userError("ไฟล์ในไฟล์สำรองข้อมูลเสียหาย")
	}
	defer rc.Close()
	out, err := os.Create(full)
	if err != nil {
		return "", 0, err
	}
	limit := min(int64(512<<20), *budget)
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = userError("ไฟล์ในไฟล์สำรองข้อมูลใหญ่เกินไป")
	}
	if err != nil {
		os.Remove(full)
		return "", 0, err
	}
	*budget -= n
	return "/uploads/" + target, n, nil
}

// mapper rewrites references from exported ids/URLs to imported ones.
type mapper struct {
	media, comps, menus, terms map[int64]int64
	urls                       map[string]string
}

func (m *mapper) replaceURLs(s string) string {
	for old, nu := range m.urls {
		if old != nu {
			s = strings.ReplaceAll(s, old, nu)
		}
	}
	return s
}

// remapJSON rewrites a design tree: instance componentId, menu menuId,
// download mediaId and every string containing an old upload URL.
func (m *mapper) remapJSON(raw json.RawMessage) json.RawMessage {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	v = m.walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func (m *mapper) walk(v any) any {
	switch t := v.(type) {
	case string:
		return m.replaceURLs(t)
	case []any:
		for i := range t {
			t[i] = m.walk(t[i])
		}
	case map[string]any:
		for k := range t {
			t[k] = m.walk(t[k])
		}
		typ, _ := t["type"].(string)
		props, _ := t["props"].(map[string]any)
		if props == nil {
			return t
		}
		swap := func(key string, ids map[int64]int64) {
			if f, ok := props[key].(float64); ok {
				if nid, ok := ids[int64(f)]; ok {
					props[key] = float64(nid)
				}
			}
		}
		switch typ {
		case "instance":
			swap("componentId", m.comps)
		case "menu":
			swap("menuId", m.menus)
		case "download":
			swap("mediaId", m.media)
		}
	}
	return v
}
