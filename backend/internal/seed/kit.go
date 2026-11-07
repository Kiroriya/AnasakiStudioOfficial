package seed

import (
	"strconv"

	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// Starter kits (2.5.0): the setup wizard can start a site from a kit the
// Plugin Store sends (pages, menu, content types, sample posts — data only).
// KitBase makes what every kit shares, as the demo content does: the main
// menu, the site header and footer, the single-post and archive templates.

// KitFrame is what KitBase made; Page wraps a kit page's sections in the site
// header and footer.
type KitFrame struct {
	Header, Footer int64
	b              *builder
}

// KitBase creates the menu (items: {label, url}), header / footer components
// and the default templates, and points the site options at them.
func KitBase(st *store.Store, authorID int64, menu []map[string]any) (*KitFrame, error) {
	items := make([]M, 0, len(menu))
	for _, it := range menu {
		items = append(items, M{"label": it["label"], "url": it["url"]})
	}
	if len(items) == 0 {
		items = []M{{"label": "หน้าแรก", "url": "/"}}
	}
	m := &model.Menu{Name: "เมนูหลัก", Items: mustJSON(items)}
	if err := st.SaveMenu(m); err != nil {
		return nil, err
	}
	header := &model.Component{Name: "ส่วนหัวเว็บ", Node: mustJSON(headerNode(m.ID))}
	footer := &model.Component{Name: "ส่วนท้ายเว็บ", Node: mustJSON(footerNode())}
	for _, c := range []*model.Component{header, footer} {
		if err := st.SaveComponent(c); err != nil {
			return nil, err
		}
	}
	f := &KitFrame{Header: header.ID, Footer: footer.ID, b: &builder{header: header.ID, footer: footer.ID, seq: 1000}}
	opts := map[string]string{
		"site_header_component": strconv.FormatInt(header.ID, 10),
		"site_footer_component": strconv.FormatInt(footer.ID, 10),
	}
	for _, tp := range []struct {
		title, slug, opt string
		design           M
	}{
		{"Single post", "single-post", "post_template_id", f.b.singleTemplate()},
		{"Category archive", "category-archive", "archive_template_id", f.b.archiveTemplate()},
	} {
		id, err := f.Template(st, authorID, tp.title, tp.slug, tp.design)
		if err != nil {
			return nil, err
		}
		opts[tp.opt] = strconv.FormatInt(id, 10)
	}
	return f, st.SetOptions(opts)
}

// Page wraps sections (design nodes) in the site header and footer.
func (f *KitFrame) Page(sections []any) map[string]any {
	kids := make([]M, 0, len(sections)+2)
	kids = append(kids, f.b.instance("ส่วนหัวเว็บ", f.Header))
	for _, s := range sections {
		if n, ok := s.(map[string]any); ok {
			kids = append(kids, n)
		}
	}
	kids = append(kids, f.b.instance("ส่วนท้ายเว็บ", f.Footer))
	return M{"id": "root", "type": "frame", "name": "หน้า", "width": fill(), "height": hug(),
		"layout": M{"mode": "vertical"}, "style": M{"fill": "#ffffff"}, "children": kids}
}

// Template stores a published template (single, archive, a type's archive).
func (f *KitFrame) Template(st *store.Store, authorID int64, title, slug string, design map[string]any) (int64, error) {
	s, err := st.UniqueSlug(model.TypeTemplate, slug, 0)
	if err != nil {
		return 0, err
	}
	p := &model.Post{Type: model.TypeTemplate, Title: title, Slug: s, Status: model.StatusPublish, Design: mustJSON(design), AuthorID: authorID}
	if err := st.CreatePost(p, nil); err != nil {
		return 0, err
	}
	return p.ID, nil
}
