package design

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Site Code in the designer (phase 3): collections over code:/table:
// sources with paging and filters, pagination nodes, inputs and forms,
// "call code" actions, and partial re-renders (RenderFragments) for the
// client runtime (/_c and /_a).

type collState struct {
	key     string
	page    int
	perPage int
	total   int
	generic bool // the page number came from the generic ?page=
}

type pagSlot struct {
	token string
	n     *Node
	cls   string
	html  *string
}

type pageScan struct {
	colls     map[string]*Node // collection nodes by (rendered) id
	paginated map[string]bool  // collections a pagination node points at
}

type formState struct {
	names   map[string]bool // input names inside the form
	resets  map[string]bool // page params of the collections it filters
	filters bool
}

type dynState struct {
	runtime   bool // the client runtime is needed
	usedPag   bool
	usedInput bool
	colls     map[string]*collState
	pags      []*pagSlot
	scanned   *pageScan
	qvars     map[string]any
	forms     []*formState

	codeBlocks map[string]bool // Frontend code blocks whose style/script is written

	// partial render (RenderFragments)
	frag      map[string]bool // requested node ids
	fragNames map[string]bool // requested layer names
	fragColls map[string]bool // collections to re-render (+ their pagination)
	fragOut   map[string]string
	fragDepth int
}

var (
	collKeyRe    = regexp.MustCompile(`^[a-z0-9]{1,12}$`)
	paramNameRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
	queryParamRe = regexp.MustCompile(`^\s*query\.([A-Za-z_$][A-Za-z0-9_$]*)\s*$`)
	// ActionNameRe is the name rule of site.collection / site.action / site.formatter.
	ActionNameRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9_-]{0,62}$`)
	// QualifiedActionRe also accepts a JS plugin's action "<plugin id>:<name>".
	QualifiedActionRe = regexp.MustCompile(`^(?:[a-z0-9][a-z0-9-]{2,39}:)?[a-z][a-zA-Z0-9_-]{0,62}$`)
)

// IsCodeSource reports a Site Code collection source ("code:x" / "table:x").
func IsCodeSource(src string) bool {
	return strings.HasPrefix(src, "code:") || strings.HasPrefix(src, "table:")
}

// CollectionKey is a collection's URL key: props.key ([a-z0-9]{1,12}) or the
// first 12 letters/digits of its node id (lower-case).
func CollectionKey(n *Node) string {
	if k := getStr(n.Props, "key"); collKeyRe.MatchString(k) {
		return k
	}
	var b []byte
	for _, c := range []byte(strings.ToLower(n.ID)) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b = append(b, c)
		}
	}
	if len(b) > 12 {
		b = b[:12]
	}
	if len(b) == 0 {
		return "c"
	}
	return string(b)
}

// SameSitePath accepts "/path?x" style redirects on this site only.
func SameSitePath(s string) bool {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, "/\\") || len(s) > 2000 {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f || c == '\\' {
			return false
		}
	}
	return true
}

func jsonText(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "{}"
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r *renderer) queryVars() any {
	if r.qvars == nil {
		r.qvars = map[string]any{}
		for k, v := range r.ctx.Query {
			switch len(v) {
			case 0:
			case 1:
				r.qvars[k] = v[0]
			default:
				arr := make([]any, len(v))
				for i, s := range v {
					arr[i] = s
				}
				r.qvars[k] = arr
			}
		}
	}
	return r.qvars
}

// itemPostCtx flattens a collection item for plugin blocks.
func itemPostCtx(it map[string]any) PostCtx {
	if it == nil {
		return nil
	}
	out := make(PostCtx, len(it))
	for k, v := range it {
		out[k] = Stringify(v)
	}
	return out
}

func postCtxItem(p PostCtx) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// capture renders fn into a separate buffer and returns what it wrote.
func (r *renderer) capture(fn func()) string {
	old := r.body
	r.body = &strings.Builder{}
	fn()
	s := r.body.String()
	r.body = old
	return s
}

// withAttrs renders a node and adds attrs to its element (the one carrying
// class="n-<id>…").
func (r *renderer) withAttrs(n *Node, attrs string, fn func()) {
	s := r.capture(fn)
	marker := `class="` + ClassName(n.ID)
	for off := 0; ; {
		i := strings.Index(s[off:], marker)
		if i < 0 {
			break
		}
		i += off
		if j := i + len(marker); j < len(s) && (s[j] == '"' || s[j] == ' ') {
			s = s[:i] + attrs + s[i:]
			break
		}
		off = i + len(marker)
	}
	r.body.WriteString(s)
}

// ---------------------------------------------------------------- page scan

// pageScan finds the collection and pagination nodes of the whole page
// (component placements included), for keys, filters and ?page=.
func (r *renderer) pageScan() *pageScan {
	if r.scanned != nil {
		return r.scanned
	}
	ps := &pageScan{colls: map[string]*Node{}, paginated: map[string]bool{}}
	r.scanned = ps
	seen := map[int64]bool{}
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		if n == nil || depth > 40 {
			return
		}
		switch n.Type {
		case TypeInstance:
			id := int64(num(n.Props["componentId"]))
			if r.data == nil || seen[id] {
				return
			}
			comp := r.data.Component(id)
			if comp == nil {
				return
			}
			if v := VariantNode(comp, getStr(n.Props, "variant")); v != nil {
				comp = v
			}
			seen[id] = true
			merged := *comp
			merged.ID = n.ID
			ov, _ := n.Props["overrides"].(map[string]any)
			merged.Children = instanceChildren(comp.Children, n.ID, ov)
			prefixRefs([]*Node{&merged}, n.ID, comp.ID)
			walk(&merged, depth+1)
			delete(seen, id)
			return
		case TypeCollection, TypePostList: // a post list can be filtered too (2.2.0)
			ps.colls[n.ID] = n
		case TypePagination:
			if c := getStr(n.Props, "collection"); c != "" {
				ps.paginated[c] = true
			}
		}
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(r.root, 0)
	// a pagination inside a component may point at a page-level collection
	for c := range ps.paginated {
		if ps.colls[c] == nil {
			if i := strings.LastIndex(c, "-"); i >= 0 && ps.colls[c[i+1:]] != nil {
				ps.paginated[c[i+1:]] = true
			}
		}
	}
	return ps
}

// prefixRefs points pagination / filter / update references inside a
// component placement at the placement's own copies (ids "<inst>-<layer>");
// the original id is kept as a fallback (a page-level collection).
func prefixRefs(nodes []*Node, prefix, rootID string) {
	pre := func(id string) string {
		if id == rootID {
			return prefix
		}
		return prefix + "-" + id
	}
	for _, c := range nodes {
		switch c.Type {
		case TypePagination:
			if v := getStr(c.Props, "collection"); v != "" {
				c.Props = cloneMap(c.Props)
				c.Props["collection"] = pre(v)
				if _, ok := c.Props["collectionAlt"]; !ok {
					c.Props["collectionAlt"] = v
				}
			}
		case TypeInput:
			if f, ok := c.Props["filter"].(map[string]any); ok && getStr(f, "collection") != "" {
				c.Props = cloneMap(c.Props)
				nf := cloneMap(f)
				nf["collection"] = pre(getStr(f, "collection"))
				if _, ok := f["collectionAlt"]; !ok {
					nf["collectionAlt"] = getStr(f, "collection")
				}
				c.Props["filter"] = nf
			}
		}
		if m, ok := c.Props["action"].(map[string]any); ok && getStr(m, "type") == "code" {
			if ups, ok := m["update"].([]any); ok && len(ups) > 0 {
				out := make([]any, 0, len(ups))
				for _, u := range ups {
					if s, ok := u.(string); ok && s != "" {
						out = append(out, pre(s))
					}
				}
				c.Props = cloneMap(c.Props)
				a := cloneMap(m)
				a["update"] = out
				c.Props["action"] = a
			}
		}
		prefixRefs(c.Children, prefix, rootID)
	}
}

// collNode resolves a referenced collection id (with the component fallback).
func (r *renderer) collNode(id, alt string) (string, *Node) {
	ps := r.pageScan()
	if n := ps.colls[id]; n != nil {
		return id, n
	}
	if alt != "" {
		if n := ps.colls[alt]; n != nil {
			return alt, n
		}
	}
	return id, nil
}

func atoiPage(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return 1
	}
	if n > 100000 {
		return 100000
	}
	return n
}

// pageFor is the current page of a collection: ?<key>_page=, or ?page= when
// it is the only paginated collection of the page.
func (r *renderer) pageFor(id, key string) (int, bool) {
	q := r.ctx.Query
	if v := q.Get(key + "_page"); v != "" {
		return atoiPage(v), false
	}
	if v := q.Get("page"); v != "" {
		if ps := r.pageScan(); len(ps.paginated) == 1 && ps.paginated[id] {
			return atoiPage(v), true
		}
	}
	return 1, false
}

// ---------------------------------------------------------------- collections

func (r *renderer) renderCollection(n *Node, cls string, sc scope) {
	w := r.body
	src := getStr(n.Props, "source")
	key := CollectionKey(n)
	page, generic := r.pageFor(n.ID, key)
	perPage := int(num(n.Props["perPage"]))
	if perPage < 0 {
		perPage = 0
	}
	if perPage > 100 {
		perPage = 100
	}
	st := &collState{key: key, page: page, perPage: perPage, generic: generic}
	var items []map[string]any
	switch {
	case IsCodeSource(src):
		if r.dyn != nil {
			res := r.dyn.LoadCollection(CollectionReq{Source: src, Page: page, PerPage: perPage,
				Params: r.collParams(n, key, sc), Table: r.tableSpec(n, sc)})
			items, st.total, st.perPage = res.Items, res.Total, res.PerPage
			if res.Page > 0 {
				st.page = res.Page
			}
		}
	case src != "" && r.data != nil:
		q, _ := n.Props["query"].(map[string]any)
		if q == nil {
			q = map[string]any{}
		}
		all := r.data.Collection(src, q)
		st.total = len(all)
		lo, hi := 0, len(all)
		if perPage > 0 {
			lo = min((page-1)*perPage, len(all))
			hi = min(lo+perPage, len(all))
		}
		for _, p := range all[lo:hi] {
			items = append(items, postCtxItem(p))
		}
	}
	if r.colls == nil {
		r.colls = map[string]*collState{}
	}
	r.colls[n.ID] = st
	noReload := getBool(n.Props, "noReload")
	w.WriteString(`<div class="` + cls + `" data-rea-collection="` + attr(n.ID) + `" data-rea-key="` + key + `"`)
	if noReload {
		r.runtime = true
		w.WriteString(` data-rea-noreload`)
	}
	if strings.HasPrefix(src, "code:") || strings.HasPrefix(src, "table:") { // anasaki.refresh(name)
		w.WriteString(` data-rea-source="` + attr(src) + `"`)
	}
	w.WriteString(`>`)
	modes := layoutModes(n)
	if len(items) == 0 {
		if t := getStr(n.Props, "emptyText"); t != "" {
			w.WriteString(`<p class="rea-empty">` + html.EscapeString(t) + `</p>`)
		}
		for _, c := range n.Children { // empty-state layers (showWhen "$empty") render once
			if c != nil && strings.Contains(getStr(c.Props, "showWhen"), "$empty") {
				r.renderNode(c, modes, scope{post: sc.post, empty: true})
			}
		}
	}
	for i, it := range items {
		for _, c := range n.Children {
			r.renderNode(c, modes, scope{post: sc.post, item: it, index: i, inColl: true})
		}
	}
	w.WriteString(`</div>`)
}

// collParams evaluates props.params in the page context; filter values in
// the URL (?<key>_<param>=) win.
func (r *renderer) collParams(n *Node, key string, sc scope) map[string]any {
	out := map[string]any{}
	if m, ok := n.Props["params"].(map[string]any); ok {
		env := r.env(sc)
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = EvalString(s, env)
			} else {
				out[k] = v
			}
		}
	}
	pre := key + "_"
	for k, vals := range r.ctx.Query {
		if strings.HasPrefix(k, pre) && k != key+"_page" && len(k) > len(pre) && len(vals) > 0 {
			out[k[len(pre):]] = vals[0]
		}
	}
	return out
}

// tableSpec reads props.table, evaluating where values.
func (r *renderer) tableSpec(n *Node, sc scope) *TableSpec {
	m, ok := n.Props["table"].(map[string]any)
	if !ok || !strings.HasPrefix(getStr(n.Props, "source"), "table:") {
		return nil
	}
	spec := &TableSpec{}
	if cols, ok := m["columns"].([]any); ok {
		for _, c := range cols {
			if s, ok := c.(string); ok {
				spec.Columns = append(spec.Columns, s)
			}
		}
	}
	var env *Env
	if ws, ok := m["where"].([]any); ok {
		for _, x := range ws {
			w, ok := x.(map[string]any)
			if !ok {
				continue
			}
			c := TableCond{Column: getStr(w, "column"), Op: getStr(w, "op")}
			switch v := w["value"].(type) {
			case string:
				if env == nil {
					env = r.env(sc)
				}
				c.Value = EvalString(v, env)
			default:
				c.Value = v
			}
			spec.Where = append(spec.Where, c)
		}
	}
	if os, ok := m["orderBy"].([]any); ok {
		for _, x := range os {
			if o, ok := x.(map[string]any); ok {
				spec.OrderBy = append(spec.OrderBy, TableSort{Column: getStr(o, "column"), Dir: getStr(o, "dir")})
			}
		}
	}
	return spec
}

// ---------------------------------------------------------------- pagination

// renderPagination leaves a placeholder: the collection (anywhere on the
// page) may render after it; finishPagination fills it in.
func (r *renderer) renderPagination(n *Node, cls string) {
	r.usedPag = true
	tok := "\x00rea-pg-" + strconv.Itoa(len(r.pags)) + "\x00"
	r.pags = append(r.pags, &pagSlot{token: tok, n: n, cls: cls})
	r.body.WriteString(tok)
}

func (r *renderer) finishPagination(s string) string {
	if len(r.pags) == 0 || !strings.Contains(s, "\x00rea-pg-") {
		return s
	}
	pairs := make([]string, 0, 2*len(r.pags))
	for _, p := range r.pags {
		if p.html == nil {
			h := r.paginationHTML(p.n, p.cls)
			p.html = &h
		}
		pairs = append(pairs, p.token, *p.html)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// pagColl is the collection state a pagination node points at.
func (r *renderer) pagColl(n *Node) (string, *collState) {
	id, alt := getStr(n.Props, "collection"), getStr(n.Props, "collectionAlt")
	if st := r.colls[id]; st != nil {
		return id, st
	}
	if alt != "" {
		if st := r.colls[alt]; st != nil {
			return alt, st
		}
	}
	return id, nil
}

func (r *renderer) english() bool { return r.ctx.Locale == "en" }

func (r *renderer) pgLabel(n *Node, which string) string {
	if m, ok := n.Props["labels"].(map[string]any); ok {
		switch v := m[which].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]any:
			lang := "th"
			if r.english() {
				lang = "en"
			}
			if s := getStr(v, lang); s != "" {
				return s
			}
		}
	}
	en := map[string]string{"prev": "Previous", "next": "Next", "more": "Load more", "aria": "Pagination"}
	th := map[string]string{"prev": "ก่อนหน้า", "next": "ถัดไป", "more": "โหลดเพิ่ม", "aria": "เลขหน้า"}
	if r.english() {
		return en[which]
	}
	return th[which]
}

// pageURL is the current URL with the collection on page (other query
// parameters kept; page 1 drops the parameter).
func (r *renderer) pageURL(st *collState, page int) string {
	q := url.Values{}
	for k, v := range r.ctx.Query {
		q[k] = append([]string(nil), v...)
	}
	if st.generic {
		q.Del("page")
	}
	if page <= 1 {
		q.Del(st.key + "_page")
	} else {
		q.Set(st.key+"_page", strconv.Itoa(page))
	}
	enc := q.Encode()
	if enc == "" {
		if r.ctx.Path != "" {
			return r.ctx.Path
		}
		return "?"
	}
	return r.ctx.Path + "?" + enc
}

func pageBounds(st *collState) (cur, last int) {
	last = (st.total + st.perPage - 1) / st.perPage
	cur = max(1, min(st.page, last))
	return cur, last
}

// paginationHTML renders
//
//	<nav class="n-<id> rea-pagination" data-rea-pagination="<collection>" data-rea-style="numbers" aria-label="…">
//	  <a class="rea-pg-item rea-pg-prev" href="?k_page=1" rel="prev">Previous</a>
//	  <a class="rea-pg-item" href="…">1</a><a class="rea-pg-item rea-pg-active" href="…" aria-current="page">2</a>
//	  <span class="rea-pg-gap">…</span> … <a class="rea-pg-item rea-pg-next is-disabled" aria-disabled="true">Next</a>
//	</nav>
//
// (hidden when everything fits on one page).
func (r *renderer) paginationHTML(n *Node, cls string) string {
	collID, st := r.pagColl(n)
	style := getStr(n.Props, "style")
	switch style {
	case "prevnext", "loadmore":
	default:
		style = "numbers"
	}
	var b strings.Builder
	b.WriteString(`<nav class="` + cls + ` rea-pagination" data-rea-pagination="` + attr(collID) + `" data-rea-style="` + style +
		`" aria-label="` + attr(r.pgLabel(n, "aria")) + `"`)
	if style == "loadmore" {
		r.runtime = true
	}
	if st == nil || st.perPage <= 0 || st.total <= st.perPage {
		b.WriteString(` hidden></nav>`)
		return b.String()
	}
	cur, last := pageBounds(st)
	link := func(page int, label, extra, rel string, current bool) {
		b.WriteString(`<a class="rea-pg-item` + extra + `" href="` + attr(r.pageURL(st, page)) + `"`)
		if rel != "" {
			b.WriteString(` rel="` + rel + `"`)
		}
		if current {
			b.WriteString(` aria-current="page"`)
		}
		b.WriteString(`>` + html.EscapeString(label) + `</a>`)
	}
	off := func(label, extra string) {
		b.WriteString(`<a class="rea-pg-item` + extra + ` is-disabled" aria-disabled="true">` + html.EscapeString(label) + `</a>`)
	}
	prev := func() {
		if cur > 1 {
			link(cur-1, r.pgLabel(n, "prev"), " rea-pg-prev", "prev", false)
		} else {
			off(r.pgLabel(n, "prev"), " rea-pg-prev")
		}
	}
	next := func() {
		if cur < last {
			link(cur+1, r.pgLabel(n, "next"), " rea-pg-next", "next", false)
		} else {
			off(r.pgLabel(n, "next"), " rea-pg-next")
		}
	}
	switch style {
	case "loadmore":
		if cur >= last {
			b.WriteString(` hidden></nav>`)
			return b.String()
		}
		b.WriteString(`>`)
		b.WriteString(`<a class="rea-pg-item rea-pg-more" href="` + attr(r.pageURL(st, cur+1)) + `" rel="next" data-rea-more>` +
			html.EscapeString(r.pgLabel(n, "more")) + `</a>`)
	case "prevnext":
		b.WriteString(`>`)
		prev()
		b.WriteString(`<span class="rea-pg-status">` + strconv.Itoa(cur) + ` / ` + strconv.Itoa(last) + `</span>`)
		next()
	default:
		b.WriteString(`>`)
		win := 2
		if v, ok := getNum(n.Props, "window"); ok {
			win = max(0, min(10, int(v)))
		}
		prev()
		lo, hi := max(2, cur-win), min(last-1, cur+win)
		num := func(p int) {
			link(p, strconv.Itoa(p), map[bool]string{true: " rea-pg-active", false: ""}[p == cur], "", p == cur)
		}
		num(1)
		if lo > 2 {
			b.WriteString(`<span class="rea-pg-gap" aria-hidden="true">…</span>`)
		}
		for p := lo; p <= hi; p++ {
			num(p)
		}
		if hi < last-1 {
			b.WriteString(`<span class="rea-pg-gap" aria-hidden="true">…</span>`)
		}
		if last > 1 {
			num(last)
		}
		next()
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// headLinks: <link rel="prev|next"> of the first paginated collection.
func (r *renderer) headLinks() string {
	for _, p := range r.pags {
		_, st := r.pagColl(p.n)
		if st == nil || st.perPage <= 0 || st.total <= st.perPage {
			continue
		}
		cur, last := pageBounds(st)
		var b strings.Builder
		if cur > 1 {
			b.WriteString(`<link rel="prev" href="` + attr(r.pageURL(st, cur-1)) + `">`)
		}
		if cur < last {
			b.WriteString(`<link rel="next" href="` + attr(r.pageURL(st, cur+1)) + `">`)
		}
		return b.String()
	}
	return ""
}

// paginationDecls turns props.itemStyle / activeStyle / disabledStyle into CSS.
func paginationDecls(m map[string]any) []Decl {
	var d []Decl
	if v := safeCSS(getStr(m, "fill")); v != "" {
		d = append(d, Decl{"background", v})
	}
	if v := safeCSS(getStr(m, "color")); v != "" {
		d = append(d, Decl{"color", v})
	}
	if v := safeCSS(getStr(m, "stroke")); v != "" {
		d = append(d, Decl{"border-color", v})
	}
	if v, ok := getNum(m, "radius"); ok {
		d = append(d, Decl{"border-radius", px(v)})
	}
	switch p := m["padding"].(type) {
	case float64:
		d = append(d, Decl{"padding", px(p)})
	case []any:
		parts := make([]string, 0, 4)
		for _, x := range p {
			parts = append(parts, px(num(x)))
		}
		if len(parts) > 0 && len(parts) <= 4 {
			d = append(d, Decl{"padding", strings.Join(parts, " ")})
		}
	}
	if v, ok := getNum(m, "fontSize"); ok && v > 0 {
		d = append(d, Decl{"font-size", px(v)})
	}
	if v, ok := getNum(m, "fontWeight"); ok && v > 0 {
		d = append(d, Decl{"font-weight", fnum(v)})
	}
	if f := FontStack(getStr(m, "fontFamily")); f != "" {
		d = append(d, Decl{"font-family", f})
	}
	return d
}

func (r *renderer) paginationCSS(n *Node, sel string) {
	for _, x := range [][2]string{{"itemStyle", ".rea-pagination .rea-pg-item"},
		{"activeStyle", ".rea-pagination .rea-pg-item.rea-pg-active"}, {"disabledStyle", ".rea-pagination .rea-pg-item.is-disabled"}} {
		if m, ok := n.Props[x[0]].(map[string]any); ok {
			writeRule(&r.cssBuf, sel+x[1], paginationDecls(m))
			if f := PrimaryFont(getStr(m, "fontFamily")); f != "" {
				r.fonts[f] = true
			}
		}
	}
}

// ---------------------------------------------------------------- inputs and forms

var inputKinds = map[string]bool{"text": true, "search": true, "number": true, "email": true, "select": true,
	"checkbox": true, "textarea": true, "hidden": true}

// inputBoxed reports whether an input layer draws its own box (fill, border
// or radius, at any breakpoint). The design styles go on the wrapper, so the
// control inside then loses the browser's border, background and padding —
// the layer's padding is the only one (2.1.1; before, both boxes showed).
func inputBoxed(n *Node) bool {
	boxed := func(style map[string]any) bool {
		if getStr(style, "fill") != "" || getStr(style, "stroke") != "" {
			return true
		}
		r, _ := getNum(style, "radius")
		return r != 0
	}
	if boxed(n.Style) {
		return true
	}
	for _, o := range n.Responsive {
		if st, _ := o["style"].(map[string]any); boxed(st) {
			return true
		}
	}
	return false
}

// renderInput renders a form field:
//
//	<div class="n-<id> rea-field"><input class="rea-field-el" type="search" name="k_q" value="…" placeholder="…" data-rea-filter="<collection>" data-rea-reset="k_page">
//	<span class="rea-error" data-rea-error-for="k_q"></span></div>
//
// (checkbox: <label class="n-<id> rea-field rea-check">; hidden: the wrapper has the hidden attribute)
//
// A filter input outside a form frame is wrapped in a GET form
// (display:contents) so it works without JavaScript.
func (r *renderer) renderInput(n *Node, cls string, sc scope) {
	r.usedInput = true
	kind := getStr(n.Props, "kind")
	if !inputKinds[kind] {
		kind = "text"
	}
	env := r.env(sc)
	name := strings.TrimSpace(getStr(n.Props, "name"))
	var filterAttrs, reset string
	if f, ok := n.Props["filter"].(map[string]any); ok {
		param := getStr(f, "param")
		if collID, cn := r.collNode(getStr(f, "collection"), getStr(f, "collectionAlt")); collID != "" && paramNameRe.MatchString(param) {
			key, mapped := CollectionKey(&Node{ID: collID}), ""
			if cn != nil {
				key = CollectionKey(cn)
				if m, ok := cn.Props["params"].(map[string]any); ok {
					if s, ok := m[param].(string); ok {
						if mm := queryParamRe.FindStringSubmatch(s); mm != nil {
							mapped = mm[1]
						}
					}
				}
			}
			name = key + "_" + param
			if mapped != "" {
				name = mapped
			}
			reset = key + "_page"
			filterAttrs = ` data-rea-filter="` + attr(collID) + `" data-rea-reset="` + attr(reset) + `"`
			r.runtime = true
		}
	}
	value := ""
	if v, ok := n.Props["value"].(string); ok && v != "" && getBool(n.Props, "valueText") {
		value = v // plain text (2.2.0), not an expression
	} else if ok && v != "" {
		value = Stringify(EvalString(v, env))
	} else if filterAttrs != "" {
		value = r.ctx.Query.Get(name)
	}
	common := ""
	if name != "" {
		common += ` name="` + attr(name) + `"`
	}
	if getBool(n.Props, "required") && kind != "hidden" {
		common += ` required`
	}
	common += filterAttrs
	placeholder := r.bind(getStr(n.Props, "placeholder"), sc)
	// the field's accessible name (2.3.0, Qraft feedback 35): the label set in the
	// panel, else the placeholder, which disappears once the visitor types
	if kind != "checkbox" && kind != "hidden" {
		label := r.bind(getStr(n.Props, "label"), sc)
		if label == "" {
			label = placeholder
		}
		if label != "" {
			common += ` aria-label="` + attr(label) + `"`
		}
	}
	errSlot := ""
	if name != "" && kind != "hidden" {
		errSlot = `<span class="rea-error" data-rea-error-for="` + attr(name) + `"></span>`
	}
	if kind != "checkbox" && inputBoxed(n) {
		cls += " rea-field-box" // the layer draws the box: the control inside drops its own
	}
	var b strings.Builder
	switch kind {
	case "select":
		b.WriteString(`<div class="` + cls + ` rea-field"><select class="rea-field-el"` + common + `>`)
		if placeholder != "" {
			b.WriteString(`<option value="">` + html.EscapeString(placeholder) + `</option>`)
		}
		for _, o := range r.inputOptions(n) {
			b.WriteString(`<option value="` + attr(o[0]) + `"`)
			if o[0] == value {
				b.WriteString(` selected`)
			}
			b.WriteString(`>` + html.EscapeString(o[1]) + `</option>`)
		}
		b.WriteString(`</select>` + errSlot + `</div>`)
	case "checkbox":
		label := r.bind(getStr(n.Props, "label"), sc)
		if label == "" {
			label = placeholder
		}
		b.WriteString(`<label class="` + cls + ` rea-field rea-check"><input class="rea-field-el" type="checkbox" value="1"` + common)
		if Truthy(value) && value != "false" && value != "off" {
			b.WriteString(` checked`)
		}
		b.WriteString(`><span>` + html.EscapeString(label) + `</span>` + errSlot + `</label>`)
	case "textarea":
		b.WriteString(`<div class="` + cls + ` rea-field"><textarea class="rea-field-el"` + common)
		if placeholder != "" {
			b.WriteString(` placeholder="` + attr(placeholder) + `"`)
		}
		b.WriteString(`>` + html.EscapeString(value) + `</textarea>` + errSlot + `</div>`)
	default:
		b.WriteString(`<div class="` + cls + ` rea-field"`)
		if kind == "hidden" {
			b.WriteString(` hidden`)
		}
		b.WriteString(`><input class="rea-field-el" type="` + kind + `"` + common)
		if value != "" {
			b.WriteString(` value="` + attr(value) + `"`)
		}
		if placeholder != "" && kind != "hidden" {
			b.WriteString(` placeholder="` + attr(placeholder) + `"`)
		}
		b.WriteString(`>` + errSlot + `</div>`)
	}
	el := b.String()
	if n := len(r.forms); n > 0 {
		f := r.forms[n-1]
		if name != "" {
			f.names[name] = true
		}
		if reset != "" {
			f.filters = true
			f.resets[reset] = true
		}
		r.body.WriteString(el)
		return
	}
	if reset == "" {
		r.body.WriteString(el)
		return
	}
	w := r.body
	w.WriteString(`<form class="rea-filter-form" method="get"`)
	if r.ctx.Path != "" {
		w.WriteString(` action="` + attr(r.ctx.Path) + `"`)
	}
	w.WriteString(` style="display:contents" data-rea-filter-form>`)
	r.hiddenQuery(map[string]bool{name: true}, map[string]bool{reset: true})
	w.WriteString(el)
	w.WriteString(`</form>`)
}

// hiddenQuery keeps the other query parameters in a GET form.
func (r *renderer) hiddenQuery(names, resets map[string]bool) {
	keys := make([]string, 0, len(r.ctx.Query))
	for k := range r.ctx.Query {
		if !names[k] && !resets[k] && k != "page" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range r.ctx.Query[k] {
			r.body.WriteString(`<input type="hidden" name="` + attr(k) + `" value="` + attr(v) + `">`)
		}
	}
}

// inputOptions returns a select's options: props.options [{value,label}] or
// props.optionsFrom {collection: "code:x"|"table:x", value, label}.
func (r *renderer) inputOptions(n *Node) [][2]string {
	var out [][2]string
	if list, ok := n.Props["options"].([]any); ok {
		for _, x := range list {
			switch o := x.(type) {
			case map[string]any:
				v := Stringify(o["value"])
				l := Stringify(o["label"])
				if l == "" {
					l = v
				}
				out = append(out, [2]string{v, l})
			case string:
				out = append(out, [2]string{o, o})
			}
		}
	}
	if of, ok := n.Props["optionsFrom"].(map[string]any); ok && r.dyn != nil {
		src := getStr(of, "collection")
		vf, lf := getStr(of, "value"), getStr(of, "label")
		if IsCodeSource(src) && vf != "" {
			if lf == "" {
				lf = vf
			}
			res := r.dyn.LoadCollection(CollectionReq{Source: src, Page: 1, PerPage: 100, Params: map[string]any{}})
			for _, it := range res.Items {
				out = append(out, [2]string{Stringify(it[vf]), Stringify(it[lf])})
			}
		}
	}
	return out
}

// renderForm renders a frame with props.form: <form data-rea-form>. A form
// with filter inputs submits with GET (works without JavaScript); an action
// form is submitted by the client runtime only.
func (r *renderer) renderForm(n *Node, cls string, sc scope) {
	fs := &formState{names: map[string]bool{}, resets: map[string]bool{}}
	r.forms = append(r.forms, fs)
	modes := layoutModes(n)
	inner := r.capture(func() {
		for _, c := range n.Children {
			r.renderNode(c, modes, sc)
		}
	})
	r.forms = r.forms[:len(r.forms)-1]
	r.runtime = true
	w := r.body
	w.WriteString(`<form class="` + cls + `" data-rea-form`)
	if fs.filters {
		w.WriteString(` method="get"`)
		if r.ctx.Path != "" {
			w.WriteString(` action="` + attr(r.ctx.Path) + `"`)
		}
		w.WriteString(`>`)
		w.WriteString(inner)
		r.hiddenQuery(fs.names, fs.resets)
	} else {
		w.WriteString(` onsubmit="return false">`)
		w.WriteString(inner)
	}
	w.WriteString(`</form>`)
}

// ---------------------------------------------------------------- actions

// codeAttrs returns the data-rea-* attributes of a "call code" action
// (props.action {type:'code', name, input, confirm, update, success}); input
// expressions are evaluated here, so the browser never evaluates anything.
func (r *renderer) codeAttrs(n *Node, sc scope) string {
	m, ok := n.Props["action"].(map[string]any)
	if !ok || getStr(m, "type") != "code" {
		return ""
	}
	name := getStr(m, "name")
	if !QualifiedActionRe.MatchString(name) {
		return ""
	}
	r.runtime = true
	input := map[string]any{}
	if in, ok := m["input"].(map[string]any); ok {
		env := r.env(sc)
		for k, v := range in {
			if s, ok := v.(string); ok {
				input[k] = EvalString(s, env)
			} else {
				input[k] = v
			}
		}
	}
	var b strings.Builder
	b.WriteString(`data-rea-action="` + attr(name) + `" data-rea-input="` + attr(jsonText(input)) + `"`)
	if c := getStr(m, "confirm"); c != "" {
		b.WriteString(` data-rea-confirm="` + attr(r.bind(c, sc)) + `"`)
	}
	if ups, ok := m["update"].([]any); ok {
		var ids []string
		for _, u := range ups {
			if s, ok := u.(string); ok && s != "" && !strings.ContainsAny(s, " \t\n\"'<>&") {
				ids = append(ids, s)
			}
		}
		if len(ids) > 0 {
			b.WriteString(` data-rea-update="` + attr(strings.Join(ids, " ")) + `"`)
		}
	}
	if s, ok := m["success"].(map[string]any); ok {
		if msg := getStr(s, "message"); msg != "" {
			b.WriteString(` data-rea-success="` + attr(r.bind(msg, sc)) + `"`)
		}
		if red := r.bind(getStr(s, "redirect"), sc); SameSitePath(red) {
			b.WriteString(` data-rea-redirect="` + attr(red) + `"`)
		}
	}
	b.WriteString(" ")
	return b.String()
}

// ---------------------------------------------------------------- partial render

// FragmentReq selects the nodes of a partial re-render: node ids, layer
// names and collection sources ("code:events"); pagination nodes of every
// selected collection are included.
type FragmentReq struct {
	IDs     []string
	Names   []string
	Sources []string
}

// RenderFragments re-renders only the selected nodes of a page (their
// outerHTML in Output.Fragments, by node id). Nodes inside a collection's
// item template cannot be selected on their own.
func RenderFragments(root *Node, ctx Context, data DataSource, req FragmentReq) Output {
	r := newRenderer(root, ctx, data)
	set := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, s := range list {
			if s != "" {
				m[s] = true
			}
		}
		return m
	}
	r.frag, r.fragNames, r.fragOut = set(req.IDs), set(req.Names), map[string]string{}
	srcs := set(req.Sources)
	r.fragColls = map[string]bool{}
	for id, n := range r.pageScan().colls {
		if r.frag[id] || (n.Name != "" && r.fragNames[n.Name]) || srcs[getStr(n.Props, "source")] {
			r.fragColls[id] = true
		}
	}
	if root != nil {
		r.renderNode(root, nil, scope{post: ctx.Post})
	}
	for k, v := range r.fragOut {
		r.fragOut[k] = r.finishPagination(v)
	}
	return Output{Fragments: r.fragOut, Runtime: r.runtime}
}

func (r *renderer) isFragTarget(n *Node) bool {
	if r.frag[n.ID] || (n.Name != "" && r.fragNames[n.Name]) {
		return true
	}
	switch n.Type {
	case TypeCollection:
		return r.fragColls[n.ID]
	case TypePagination:
		return r.fragColls[getStr(n.Props, "collection")] || r.fragColls[getStr(n.Props, "collectionAlt")]
	}
	return false
}

// fragEnter handles a node outside the selected ones during a partial
// render: a selected node is rendered into Fragments, others are walked
// without output. It always returns false (the caller writes nothing).
func (r *renderer) fragEnter(n *Node, pm parentModes, sc scope) bool {
	if r.isFragTarget(n) {
		r.fragDepth++
		s := r.capture(func() { r.renderNode(n, pm, sc) })
		r.fragDepth--
		r.fragOut[n.ID] = s
		return false
	}
	switch n.Type {
	case TypeInstance:
		r.renderInstance(n, pm, sc)
	case TypeCollection, TypePostList:
		// item templates are not re-rendered on their own
	default:
		modes := layoutModes(n)
		for _, c := range n.Children {
			r.renderNode(c, modes, sc)
		}
	}
	return false
}

// ---------------------------------------------------------------- CSS

func (r *renderer) dynCSS(css *strings.Builder) {
	if r.usedPag {
		css.WriteString(PaginationCSS)
	}
	if r.usedInput || r.runtime {
		css.WriteString(RuntimeCSS)
	}
}

// PaginationCSS is added to pages with pagination nodes.
const PaginationCSS = `.rea-pagination{display:flex;flex-wrap:wrap;align-items:center;gap:6px}
.rea-pagination[hidden]{display:none!important}
.rea-pagination .rea-pg-item{display:inline-flex;align-items:center;justify-content:center;min-width:2.4em;padding:.45em .8em;border:1px solid #e5e7eb;border-radius:8px;color:inherit;text-decoration:none;line-height:1.2;background:transparent}
.rea-pagination a.rea-pg-item:hover{background:rgba(17,24,39,.06)}
.rea-pagination .rea-pg-item.rea-pg-active{background:#111827;border-color:#111827;color:#fff}
.rea-pagination .rea-pg-item.is-disabled{opacity:.45;pointer-events:none;cursor:default}
.rea-pg-gap{padding:0 .2em;opacity:.6}
.rea-pg-status{padding:0 .5em;opacity:.75}
`

// RuntimeCSS styles inputs, field errors, loading states and the toast.
const RuntimeCSS = `.rea-field{display:flex;flex-direction:column}
.rea-field[hidden]{display:none!important}
.rea-field-el{font:inherit;color:inherit;width:100%;min-width:0;box-sizing:border-box}
.rea-field.rea-check{flex-direction:row;flex-wrap:wrap;align-items:center;gap:.5em;cursor:pointer}
.rea-check .rea-field-el{width:auto}
.rea-field-box .rea-field-el{border:0;background:transparent;padding:0;outline:0;border-radius:0;box-shadow:none}
.rea-field-box .rea-field-el::placeholder{color:inherit;opacity:.55}
.rea-field-box:focus-within{outline:2px solid #2563eb;outline-offset:2px}
.rea-check .rea-error{flex-basis:100%}
.rea-error{display:block;color:#dc2626;font-size:.85em;margin-top:4px}
.rea-error:empty{display:none}
.rea-loading{opacity:.55;transition:opacity .15s;pointer-events:none}
[data-rea-action]{cursor:pointer}
.rea-toast{position:fixed;left:50%;bottom:24px;transform:translateX(-50%);z-index:2147483000;max-width:calc(100vw - 32px);padding:12px 18px;border-radius:10px;background:#111827;color:#fff;box-shadow:0 8px 24px rgba(0,0,0,.22);font-size:15px;line-height:1.4}
.rea-toast.rea-err{background:#b91c1c}
`
