package design

import (
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PostCtx is the data a post exposes to bindings like {{post.title}}.
type PostCtx map[string]string

// Query mirrors the WordPress Query Loop block's settings.
type Query struct {
	PostType string
	Category string // term slug, or "current" for archive templates
	Limit    int
	Order    string // "newest" | "oldest" | "title"
	// On a taxonomy archive (2.1.0): the posts of these types having one of
	// these terms (the term and its children); PostType is "" then.
	PostTypes []string
	TermIDs   []int64
	// Search (2.2.0): words a search box sent to this list (?<key>_q=).
	Search string
}

// MenuItem is one navigation link.
type MenuItem struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// DataSource lets the renderer pull dynamic content (CMS side).
type DataSource interface {
	QueryPosts(q Query) []PostCtx
	MenuItems(menuID int64) []MenuItem
	Component(id int64) *Node
	// Collection returns the items of a plugin-provided collection (e.g.
	// "products") for a collection node; query is the node's props.query.
	Collection(source string, query map[string]any) []PostCtx
	// Media returns an uploaded file (download nodes); nil when missing.
	Media(id int64) *MediaInfo
	// PluginBlock renders a plugin block's inner HTML and its (scoped) CSS;
	// ok is false for an unknown or inactive plugin/block.
	PluginBlock(c BlockCall) (html, css string, ok bool)
}

// MediaInfo describes an uploaded file.
type MediaInfo struct {
	ID       int64
	URL      string
	Filename string
	Mime     string
	Size     int64
}

// BlockCall is a pluginBlock node to render.
type BlockCall struct {
	Plugin string
	Block  string
	Values map[string]any
	Site   map[string]string
	Post   PostCtx
	Item   PostCtx // current collection item, nil outside collections
}

// Context holds binding values for the current request.
type Context struct {
	Site     map[string]string
	Post     PostCtx // the post being rendered (single templates) or nil
	Category string  // current category slug on archive pages
	// TextStyles are the site's shared text styles by id (style.textStyle).
	TextStyles map[string]TextStyle

	// Site Code (all optional): {{page.*}} data, the URL query ({{query.*}},
	// collection paging/filters), the page path (pagination links), the
	// locale ("th"/"en", | date and default labels) and timezone.
	Page   map[string]any
	Query  url.Values
	Path   string
	Locale string
	Loc    *time.Location
	// MediaMax moves a breakpoint (2.2.0, theme): name → max-width px; a
	// missing name keeps Breakpoints' value.
	MediaMax map[string]int
	// Visitor resolves {{visitor.*}} (site.visitor, phase 4a) lazily: called
	// at most once per render by the provider; nil = no visitor.
	Visitor func() map[string]any
}

// Output is a rendered design.
type Output struct {
	HTML  string
	CSS   string
	Fonts []string
	Anim  bool // the page needs AnimScript (scroll-in animations)
	// Interact: the page needs InteractScript with Actions (popups, tabs, sliders…).
	Interact bool
	Actions  []Action
	// Runtime: the page needs the Site Code client runtime (RuntimeScript):
	// actions, filters, load-more or no-reload pagination.
	Runtime bool
	// Head is extra <head> HTML (rel prev/next of the first paginated collection).
	Head string
	// Fragments are the outerHTML of the requested nodes (RenderFragments).
	Fragments map[string]string
}

type renderer struct {
	data     DataSource
	dyn      Dynamic // nil unless data serves Site Code
	ctx      Context
	root     *Node
	body     *strings.Builder
	css      map[string]bool
	cssBuf   strings.Builder
	blockCSS strings.Builder
	media    map[string]*strings.Builder
	fonts    map[string]bool
	blocks   map[string]bool // plugin block CSS already emitted
	depth    int
	inComp   map[int64]bool // components being rendered (cycle guard)
	placed   int            // instances rendered so far (fan-out guard)
	anim     bool           // a node uses a scroll-in animation
	interact bool           // popups, collapsed layers, sliders or actions are used
	actions  []Action
	acted    map[Action]bool

	dynState // Site Code: collections, pagination, inputs, actions, fragments
}

func newRenderer(root *Node, ctx Context, data DataSource) *renderer {
	r := &renderer{data: data, ctx: ctx, root: root, body: &strings.Builder{}, css: map[string]bool{}, fonts: map[string]bool{},
		media: map[string]*strings.Builder{}, blocks: map[string]bool{}}
	if d, ok := data.(Dynamic); ok && d != nil {
		r.dyn = d
	}
	for _, b := range Breakpoints {
		r.media[b.Name] = &strings.Builder{}
	}
	return r
}

// Render turns a design tree into HTML and CSS.
func Render(root *Node, ctx Context, data DataSource) Output {
	r := newRenderer(root, ctx, data)
	r.renderNode(root, nil, scope{post: ctx.Post})
	bodyHTML := r.finishPagination(r.body.String())

	var css strings.Builder
	css.WriteString(r.blockCSS.String()) // first, so node styles win on equal specificity
	css.WriteString(r.cssBuf.String())
	for _, b := range Breakpoints {
		if m := r.media[b.Name]; m.Len() > 0 {
			max := b.MaxWidth
			if v := r.ctx.MediaMax[b.Name]; v > 0 {
				max = v
			}
			css.WriteString("@media (max-width:" + strconv.Itoa(max) + "px){\n")
			css.WriteString(m.String())
			css.WriteString("}\n")
		}
	}
	fonts := make([]string, 0, len(r.fonts))
	for f := range r.fonts {
		fonts = append(fonts, f)
	}
	sort.Strings(fonts)
	if r.interact {
		css.WriteString(InteractCSS)
	}
	if r.anim {
		css.WriteString(AnimCSS)
	}
	r.dynCSS(&css)
	return Output{HTML: bodyHTML, CSS: css.String(), Fonts: fonts, Anim: r.anim, Interact: r.interact, Actions: r.actions,
		Runtime: r.runtime, Head: r.headLinks()}
}

// parentModes holds the parent's layout mode at each breakpoint
// (index 0 = desktop, then Breakpoints order).
type parentModes []string

func (r *renderer) emitCSS(n *Node, pm parentModes) {
	if r.css[n.ID] {
		return
	}
	r.css[n.ID] = true
	sel := "." + ClassName(n.ID)
	prev := Decls(n, pm.at(0))
	writeRule(&r.cssBuf, sel, prev)
	if h := HoverDecls(n); len(h) > 0 {
		writeRule(&r.cssBuf, sel+":hover", h)
	}
	if a := ActiveDecls(n); len(a) > 0 { // active tab / open accordion trigger
		if n.Type == TypeMenu { // 2.2.0: a menu's active look is its link to the current page
			writeRule(&r.cssBuf, sel+` a[aria-current="page"]`, a)
		} else {
			writeRule(&r.cssBuf, sel+".rea-active", a)
		}
	}
	if n.Type == TypeMenu {
		writeRule(&r.cssBuf, sel+" a", menuLinkDecls(n))
	}
	if n.Type == TypePagination {
		r.paginationCSS(n, sel)
	}
	// plugin block parts: the designer's own look for elements inside the block
	// (doubled class: wins over the block's scoped CSS)
	for _, p := range BlockParts(n) {
		writeRule(&r.cssBuf, sel+sel+" ."+p.Class, p.Decls)
		if len(p.Hover) > 0 {
			writeRule(&r.cssBuf, sel+sel+" ."+p.Class+":hover", p.Hover)
		}
	}
	for i, b := range Breakpoints {
		cur := Decls(n.Effective(b.Name), pm.at(i+1))
		writeRule(r.media[b.Name], sel, diff(prev, cur))
		prev = cur
	}
	if f := PrimaryFont(getStr(n.Style, "fontFamily")); f != "" {
		r.fonts[f] = true
	}
	for _, o := range n.Responsive {
		if s, ok := o["style"].(map[string]any); ok {
			if f := PrimaryFont(getStr(s, "fontFamily")); f != "" {
				r.fonts[f] = true
			}
		}
	}
}

func (pm parentModes) at(i int) string {
	if pm == nil {
		return ""
	}
	return pm[i]
}

func layoutModes(n *Node) parentModes {
	out := make(parentModes, len(Breakpoints)+1)
	for i := range out {
		e := n
		if i > 0 {
			e = n.Effective(Breakpoints[i-1].Name)
		}
		m := getStr(e.Layout, "mode")
		if m == "" {
			m = "none"
		}
		out[i] = m
	}
	return out
}

// scope is the binding data visible to a node: the page/template post and,
// inside a collection node, the current collection item.
type scope struct {
	post   PostCtx
	item   map[string]any
	index  int
	inColl bool // inside a collection's item template ($index)
	empty  bool // a collection's empty-state children ($empty)
}

// env is the expression environment of a scope.
func (r *renderer) env(sc scope) *Env {
	e := &Env{Locale: r.ctx.Locale, Loc: r.ctx.Loc}
	if r.dyn != nil {
		e.Format = r.dyn.Format
	}
	e.Var = func(name string) any {
		switch name {
		case "item":
			if sc.item == nil {
				return nil
			}
			return sc.item
		case "post":
			if sc.post == nil {
				return nil
			}
			return postValue(sc.post)
		case "visitor":
			if r.ctx.Visitor == nil {
				return nil
			}
			if v := r.ctx.Visitor(); v != nil {
				return v
			}
			return nil
		case "site":
			if r.ctx.Site == nil {
				return nil
			}
			return r.ctx.Site
		case "page":
			if r.ctx.Page == nil {
				return nil
			}
			return r.ctx.Page
		case "query":
			return r.queryVars()
		case "$index":
			if sc.inColl {
				return float64(sc.index)
			}
		case "$empty":
			return sc.empty
		}
		return nil
	}
	return e
}

// bind evaluates the {{ expressions }} in s (values not escaped).
func (r *renderer) bind(s string, sc scope) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return ParseTemplate(s).Render(r.env(sc), nil)
}

// bindEscaped is bind for HTML templates (formatted text): the expression
// source is HTML (entities are decoded first) and values are escaped.
func (r *renderer) bindEscaped(s string, sc scope) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	var b strings.Builder
	var env *Env
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			b.WriteString(s)
			break
		}
		end := closeBraces(s, i+2)
		if end < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		if e, err := cachedExpr(html.UnescapeString(s[i+2 : end])); err == nil {
			if env == nil {
				env = r.env(sc)
			}
			b.WriteString(html.EscapeString(Stringify(e.Eval(env))))
		} else {
			b.WriteString(s[i : end+2])
		}
		s = s[end+2:]
	}
	return b.String()
}

var allowedTags = map[string]bool{"div": true, "section": true, "header": true, "footer": true, "nav": true,
	"main": true, "article": true, "aside": true, "p": true, "span": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "blockquote": true, "ul": true, "li": true}

func tagOr(n *Node, def string) string {
	if t := getStr(n.Props, "tag"); allowedTags[t] {
		return t
	}
	return def
}

func attr(s string) string { return html.EscapeString(s) }

// safeHref blocks javascript: and similar schemes in user-provided links.
// Browsers drop tabs, newlines and control characters in a URL
// ("java\tscript:"), so those are removed before the scheme is checked.
func safeHref(u string) string {
	l := strings.ToLower(strings.Map(func(r rune) rune {
		if r <= 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, u))
	if strings.HasPrefix(l, "javascript:") || strings.HasPrefix(l, "vbscript:") || strings.HasPrefix(l, "data:") {
		return "#"
	}
	return u
}

func (r *renderer) renderNode(n *Node, pm parentModes, sc scope) {
	if n == nil || r.depth > 40 {
		return
	}
	r.depth++
	defer func() { r.depth-- }()

	if sw := getStr(n.Props, "showWhen"); sw != "" && !Condition(sw, r.env(sc)) {
		return // hidden by its condition: nothing is sent
	}
	if r.frag != nil && r.fragDepth == 0 && !r.fragEnter(n, pm, sc) {
		return // partial render: only the requested nodes are written
	}
	if n.Type == TypeInstance {
		r.renderInstance(n, pm, sc)
		return
	}
	// Site Code action: data-rea-* on the element; a name for code: data-ref (2.2.0)
	if attrs := r.codeAttrs(n, sc) + RefAttr(n); attrs != "" {
		r.withAttrs(n, attrs, func() { r.renderElement(n, pm, sc) })
		return
	}
	r.renderElement(n, pm, sc)
}

// RefNameRe is a layer's "name for code" (props.ref): a-z first, then letters,
// digits, "_" or "-", at most 40.
var RefNameRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9_-]{0,39}$`)

// RefAttr is `data-ref="<name>"` for a layer with a name for code (2.2.0):
// code blocks find it with anasaki.ref(name) / [data-ref="name"], which keeps
// working when the layer's text or id changes. "" without one.
func RefAttr(n *Node) string {
	if ref := getStr(n.Props, "ref"); RefNameRe.MatchString(ref) {
		return `data-ref="` + ref + `" `
	}
	return ""
}

// renderElement writes one (non-instance) node.
func (r *renderer) renderElement(n *Node, pm parentModes, sc scope) {
	n = withTextStyle(n, r.ctx.TextStyles)
	r.emitCSS(n, pm)
	cls := ClassName(n.ID)
	if getBool(n.Props, "slideFill") {
		cls += " rea-slide-fill"
	}
	if a := AnimClasses(n); a != "" { // scroll-in animation (classes only, safe in every class="" below)
		cls += a
		r.anim = true
	}
	w := r.body
	if a, ok := actionOf(n); ok {
		r.interact = true
		if !r.acted[a] { // repeated layers (post lists) share one action
			if r.acted == nil {
				r.acted = map[Action]bool{}
			}
			r.acted[a] = true
			r.actions = append(r.actions, a)
		}
	}
	if getBool(n.Props, "collapsed") { // hidden until an action shows it
		cls += " rea-collapsed"
		r.interact = true
	}
	if getBool(n.Props, "popup") && n.Type == TypeFrame {
		r.interact = true
		w.WriteString(popupOpen(n))
		defer w.WriteString("</div>")
	}
	href := safeHref(r.bind(getStr(n.Props, "href"), sc))
	target := ""
	if getBool(n.Props, "newTab") {
		target = ` target="_blank" rel="noopener"`
	}

	switch n.Type {
	case TypeText:
		tag := tagOr(n, "p")
		text := html.EscapeString(r.bind(getStr(n.Props, "text"), sc))
		if rich := getStr(n.Props, "rich"); rich != "" {
			// formatted text (bold, colors, links…): whitelisted, bindings escaped
			text = SanitizeRich(r.bindEscaped(rich, sc))
		}
		w.WriteString("<" + tag + ` class="` + cls + `">`)
		if href != "" {
			w.WriteString(`<a href="` + attr(href) + `"` + target + ` style="color:inherit;text-decoration:inherit">` + text + `</a>`)
		} else {
			w.WriteString(text)
		}
		w.WriteString("</" + tag + ">")
	case TypeImage:
		src := r.bind(getStr(n.Props, "src"), sc)
		alt := r.bind(getStr(n.Props, "alt"), sc)
		if strings.TrimSpace(src) == "" {
			if strings.Contains(getStr(n.Props, "src"), "{{") {
				// Data-bound image with no value (e.g. a post without a cover): leave it
				// out entirely instead of showing an empty gray box.
				break
			}
			// Image the designer has not picked yet: keep the box as a placeholder.
			w.WriteString(`<div class="` + cls + `" role="img" aria-label="` + attr(alt) + `"></div>`)
			break
		}
		img := `<img class="` + cls + `" src="` + attr(safeHref(src)) + `" alt="` + attr(alt) + `" loading="lazy">`
		if href != "" {
			// wrap without changing layout: the link becomes display:contents
			img = `<a href="` + attr(href) + `"` + target + ` style="display:contents">` + img + `</a>`
		}
		w.WriteString(img)
	case TypeButton:
		if href == "" {
			href = "#"
		}
		label := html.EscapeString(r.bind(getStr(n.Props, "text"), sc))
		if ic := ButtonIcon(n); ic != "" { // 2.2.0: an icon before or after the text
			if getStr(n.Props, "iconPosition") == "end" {
				label = `<span>` + label + `</span>` + ic
			} else {
				label = ic + `<span>` + label + `</span>`
			}
		}
		w.WriteString(`<a class="` + cls + ` rea-btn" href="` + attr(href) + `"` + target + `>` + label + `</a>`)
	case TypeDownload:
		r.renderDownload(n, cls, sc)
	case TypeBreadcrumb:
		r.renderBreadcrumb(n, cls, sc)
	case TypePluginBlock:
		r.renderPluginBlock(n, cls, sc)
	case TypeRect:
		w.WriteString(`<div class="` + cls + `"></div>`)
	case TypeIcon:
		svg := IconSVG(n, cls)
		if href != "" {
			svg = `<a href="` + attr(href) + `"` + target + ` style="display:contents">` + svg + `</a>`
		}
		w.WriteString(svg)
	case TypeVector:
		svg := VectorSVG(n, cls)
		if href != "" {
			svg = `<a href="` + attr(href) + `"` + target + ` style="display:contents">` + svg + `</a>`
		}
		w.WriteString(svg)
	case TypeShape:
		w.WriteString(ShapeSVG(n, cls))
	case TypeEmbed:
		// Raw HTML: only editors/admins can save designs (see api role checks).
		w.WriteString(`<div class="` + cls + `">` + getStr(n.Props, "html") + `</div>`)
	case TypeCodeBlock: // Frontend code (codeblock.go): unfiltered_html, checked on save
		r.writeCodeBlock(n, cls)
	case TypeMenu:
		burger := getBool(n.Props, "burger")
		if burger {
			// phones: links fold into a ☰ button (CSS only, see BaseCSS .rea-burger)
			id := "rb-" + ClassName(n.ID)
			w.WriteString(`<nav class="` + cls + ` rea-menu rea-burger"><input type="checkbox" id="` + id + `" class="rea-bt" aria-label="Menu">` +
				`<label for="` + id + `" class="rea-bb" aria-hidden="true">` + BurgerSVG + `</label><span class="rea-bl">`)
		} else {
			w.WriteString(`<nav class="` + cls + ` rea-menu">`)
		}
		if r.data != nil {
			for _, it := range r.data.MenuItems(int64(num(n.Props["menuId"]))) {
				cur := ""
				if r.ctx.Path != "" && SamePage(it.URL, r.ctx.Path) {
					cur = ` aria-current="page"`
				}
				w.WriteString(`<a href="` + attr(safeHref(it.URL)) + `"` + cur + `>` + html.EscapeString(it.Label) + `</a>`)
			}
		}
		if burger {
			w.WriteString(`</span>`)
		}
		w.WriteString(`</nav>`)
	case TypePostContent:
		w.WriteString(`<div class="` + cls + ` rea-content">`)
		if sc.post != nil {
			content := sc.post["content"] // sanitized at save time by role (unfiltered_html analogue)
			if f, ok := r.data.(ContentFilter); ok && f != nil {
				content = f.FilterContent(content, sc.post) // Site Code post.content filters
			}
			w.WriteString(content)
		}
		w.WriteString(`</div>`)
	case TypePostList:
		// data-rea-collection: search boxes / selects bound to the list refresh it without a reload (2.2.0)
		w.WriteString(`<div class="` + cls + `" data-rea-collection="` + attr(n.ID) + `" data-rea-key="` + CollectionKey(n) + `">`)
		posts := r.queryPosts(n)
		modes := layoutModes(n)
		if len(posts) == 0 {
			if t := getStr(n.Props, "emptyText"); t != "" {
				w.WriteString(`<p class="rea-empty">` + html.EscapeString(t) + `</p>`)
			}
		}
		for _, p := range posts {
			for _, c := range n.Children {
				r.renderNode(c, modes, scope{post: p, item: sc.item, index: sc.index, inColl: sc.inColl})
			}
		}
		w.WriteString(`</div>`)
	case TypeCollection:
		r.renderCollection(n, cls, sc)
	case TypePagination:
		r.renderPagination(n, cls)
	case TypeInput:
		r.renderInput(n, cls, sc)
	default: // frame
		if getBool(n.Props, "form") && href == "" && !isCarousel(n) {
			r.renderForm(n, cls, sc)
			break
		}
		tag := tagOr(n, "div")
		if href != "" {
			w.WriteString(`<a class="` + cls + `" href="` + attr(href) + `"` + target + ` style="color:inherit;text-decoration:none">`)
		} else if isCarousel(n) {
			r.interact = true
			w.WriteString("<" + tag + ` class="` + cls + ` rea-car"`)
			if ap := carouselAutoplay(n); ap > 0 {
				w.WriteString(` data-autoplay="` + strconv.Itoa(ap) + `"`)
			}
			w.WriteString(`><div class="rea-track">`)
		} else {
			w.WriteString("<" + tag + ` class="` + cls + `">`)
		}
		modes := layoutModes(n)
		for _, c := range n.Children {
			if isCarousel(n) && c.Width.Mode == "fill" {
				fill := *c // one slide per view
				fill.Props = cloneMap(c.Props)
				fill.Props["slideFill"] = true
				r.renderNode(&fill, modes, sc)
				continue
			}
			r.renderNode(c, modes, sc)
		}
		if isCarousel(n) && href == "" {
			w.WriteString(carouselClose(n))
		}
		if href != "" {
			w.WriteString("</a>")
		} else {
			w.WriteString("</" + tag + ">")
		}
	}
}

// maxInstances caps the component placements rendered on one page.
const maxInstances = 5000

// renderInstance renders a component's tree in place of the instance. The
// instance keeps its own id, position, size and responsive overrides (so each
// placement can differ) while everything inside comes from the component.
func (r *renderer) renderInstance(n *Node, pm parentModes, sc scope) {
	if r.data == nil {
		return
	}
	id := int64(num(n.Props["componentId"]))
	// a component that contains itself, directly or through others, would
	// recurse until the depth cap with exponential fan-out: skip the repeat
	if r.inComp[id] || r.placed >= maxInstances {
		return
	}
	comp := r.data.Component(id)
	if comp == nil {
		return
	}
	r.placed++
	if r.inComp == nil {
		r.inComp = map[int64]bool{}
	}
	r.inComp[id] = true
	defer delete(r.inComp, id)
	if v := VariantNode(comp, getStr(n.Props, "variant")); v != nil {
		comp = v
	}
	merged := *comp
	merged.ID = n.ID
	merged.X, merged.Y = n.X, n.Y
	merged.Width, merged.Height = n.Width, n.Height
	merged.Visible = n.Visible
	merged.Absolute = n.Absolute
	merged.Responsive = mergeResponsive(comp.Responsive, n.Responsive)
	// this placement's own fill / stroke / effects / text style wins over the component's
	merged.Style = mergeMap(cloneMap(comp.Style), n.Style)
	// inner layers get ids unique to this placement (several instances on one
	// page never share CSS) and this placement's overrides (props.overrides)
	ov, _ := n.Props["overrides"].(map[string]any)
	merged.Children = instanceChildren(comp.Children, n.ID, ov)
	if n.Props["href"] != nil {
		merged.Props = cloneMap(comp.Props)
		merged.Props["href"] = n.Props["href"]
	}
	prefixActions([]*Node{&merged}, n.ID, comp.ID)
	prefixRefs([]*Node{&merged}, n.ID, comp.ID)
	r.renderNode(&merged, pm, sc)
}

// mergeResponsive combines a component's own breakpoint overrides (layout,
// style…) with the instance's (position/size/visibility); the instance wins.
func mergeResponsive(comp, inst map[string]map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, src := range []map[string]map[string]any{comp, inst} {
		for bp, o := range src {
			if out[bp] == nil {
				out[bp] = map[string]any{}
			}
			for k, v := range o {
				if m, ok := v.(map[string]any); ok && (k == "layout" || k == "style") {
					out[bp][k] = mergeMap(cloneAny(out[bp][k]), m)
				} else {
					out[bp][k] = v
				}
			}
		}
	}
	return out
}

func cloneAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	return cloneMap(m)
}

func (r *renderer) queryPosts(n *Node) []PostCtx {
	if r.data == nil {
		return nil
	}
	q := Query{PostType: "post", Limit: 6, Order: "newest"}
	if m, ok := n.Props["query"].(map[string]any); ok {
		if v := getStr(m, "postType"); v != "" {
			q.PostType = v
		}
		q.Category = getStr(m, "category")
		if v, ok := getNum(m, "limit"); ok && v > 0 {
			q.Limit = int(v)
		}
		if v := getStr(m, "order"); v != "" {
			q.Order = v
		}
	}
	// 2.2.0: a search box / category select of the page filters this list
	// (?<key>_q=words, ?<key>_cat=<category slug>), like a code collection
	if r.ctx.Query != nil {
		key := CollectionKey(n)
		if s := strings.TrimSpace(r.ctx.Query.Get(key + "_q")); s != "" {
			q.Search = truncRunes(s, 100)
		}
		if c := strings.TrimSpace(r.ctx.Query.Get(key + "_cat")); c != "" && len(c) <= 200 && !strings.ContainsAny(c, "/?#&") {
			q.Category = c
		}
	}
	// On a taxonomy term's archive (2.1.0) "this archive's posts" lists the
	// posts having the term (archiveTerms / archiveTypes, comma-separated).
	if terms := r.ctx.Post["archiveTerms"]; terms != "" && (q.PostType == "current" || q.Category == "current") {
		q.PostType, q.Category = "", ""
		for _, s := range strings.Split(terms, ",") {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				q.TermIDs = append(q.TermIDs, id)
			}
		}
		q.PostTypes = strings.Split(r.ctx.Post["archiveTypes"], ",")
		if q.Limit > 50 {
			q.Limit = 50
		}
		return r.data.QueryPosts(q)
	}
	// On a custom type's archive (/events) the template's "this archive's
	// posts" list (category "current", or postType "current") shows that type.
	if at := r.ctx.Post["archiveType"]; at != "" && (q.PostType == "current" || q.Category == "current") {
		q.PostType = at
		if q.Category == "current" {
			q.Category = ""
		}
	}
	if q.PostType == "current" {
		q.PostType = "post"
	}
	if q.Category == "current" {
		q.Category = r.ctx.Category
	}
	if q.Limit > 50 {
		q.Limit = 50
	}
	return r.data.QueryPosts(q)
}

// renderDownload renders a file download button:
// <a class="CLS rea-btn rea-download" href="/download/ID" download>TEXT <span class="rea-file-meta">PDF · 1.2 MB</span></a>
func (r *renderer) renderDownload(n *Node, cls string, sc scope) {
	w := r.body
	id := int64(num(r.bind(getStr(n.Props, "mediaId"), sc)))
	var m *MediaInfo
	if id > 0 && r.data != nil {
		m = r.data.Media(id)
	}
	text := r.bind(getStr(n.Props, "text"), sc)
	if text == "" {
		text = "ดาวน์โหลด"
		if m != nil && m.Filename != "" {
			text = m.Filename
		}
	}
	href := "#"
	if m != nil {
		href = "/download/" + strconv.FormatInt(m.ID, 10)
	}
	w.WriteString(`<a class="` + cls + ` rea-btn rea-download" href="` + attr(href) + `" download>` + html.EscapeString(text))
	if m != nil && getBool(n.Props, "showSize") {
		w.WriteString(` <span class="rea-file-meta">` + html.EscapeString(FileMeta(m)) + `</span>`)
	}
	w.WriteString(`</a>`)
}

// FileMeta formats "PDF · 1.2 MB" for a media file.
func FileMeta(m *MediaInfo) string {
	kind := ""
	if i := strings.LastIndex(m.Filename, "."); i >= 0 && i < len(m.Filename)-1 {
		kind = strings.ToUpper(m.Filename[i+1:])
	} else if _, sub, ok := strings.Cut(m.Mime, "/"); ok {
		kind = strings.ToUpper(sub)
	}
	size := HumanSize(m.Size)
	if kind == "" {
		return size
	}
	return kind + " · " + size
}

// HumanSize formats a byte count: 512 B, 12 KB, 1.2 MB, 3.4 GB.
func HumanSize(n int64) string {
	switch {
	case n < 1024:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1024*1024:
		return strconv.FormatFloat(float64(n)/1024, 'f', 0, 64) + " KB"
	case n < 1024*1024*1024:
		return strconv.FormatFloat(float64(n)/(1024*1024), 'f', 1, 64) + " MB"
	}
	return strconv.FormatFloat(float64(n)/(1024*1024*1024), 'f', 1, 64) + " GB"
}

// renderBreadcrumb renders Home › Category › Title from the page context:
// a post (single template) shows its first category, a category archive
// shows the category, a page shows its title; the home page shows Home only.
//
//	<nav class="CLS rea-breadcrumb" aria-label="Breadcrumb"><a href="/">Home</a><span class="rea-bc-sep">›</span>…<span class="rea-bc-current" aria-current="page">Title</span></nav>
func (r *renderer) renderBreadcrumb(n *Node, cls string, sc scope) {
	home := getStr(n.Props, "homeLabel")
	if home == "" {
		home = "หน้าแรก"
	}
	sep := getStr(n.Props, "separator")
	if sep == "" {
		sep = "›"
	}
	type crumb struct{ label, href string }
	crumbs := []crumb{{home, "/"}}
	p := sc.post
	if p != nil && p["isHome"] != "1" && !(p["type"] == "page" && p["slug"] == "home") {
		if p["type"] == "post" && p["category"] != "" && !getBool(n.Props, "hideCategory") {
			crumbs = append(crumbs, crumb{p["category"], p["categoryUrl"]})
		}
		if t := p["title"]; t != "" {
			crumbs = append(crumbs, crumb{t, ""})
		}
	}
	w := r.body
	w.WriteString(`<nav class="` + cls + ` rea-breadcrumb" aria-label="Breadcrumb">`)
	current := getStr(n.Props, "currentColor")
	for i, c := range crumbs {
		if i > 0 {
			w.WriteString(`<span class="rea-bc-sep" aria-hidden="true">` + html.EscapeString(sep) + `</span>`)
		}
		if i == len(crumbs)-1 && i > 0 {
			style := ""
			if current != "" {
				style = ` style="color:` + attr(safeCSS(current)) + `"`
			}
			w.WriteString(`<span class="rea-bc-current" aria-current="page"` + style + `>` + html.EscapeString(c.label) + `</span>`)
			continue
		}
		href := c.href
		if href == "" {
			href = "/"
		}
		w.WriteString(`<a href="` + attr(safeHref(href)) + `">` + html.EscapeString(c.label) + `</a>`)
	}
	w.WriteString(`</nav>`)
}

// renderPluginBlock renders a block contributed by a plugin inside
// <div class="CLS pb-<plugin>-<block>">. Unknown/inactive → empty div.
func (r *renderer) renderPluginBlock(n *Node, cls string, sc scope) {
	plugin, block := getStr(n.Props, "plugin"), getStr(n.Props, "block")
	w := r.body
	w.WriteString(`<div class="` + cls)
	if plugin != "" && block != "" {
		w.WriteString(" " + attr(pluginClass(plugin, block)))
	}
	w.WriteString(`">`)
	if r.data != nil && plugin != "" && block != "" {
		values, _ := n.Props["values"].(map[string]any)
		if values == nil {
			values = map[string]any{}
		}
		inner, css, ok := r.data.PluginBlock(BlockCall{Plugin: plugin, Block: block, Values: values,
			Site: r.ctx.Site, Post: sc.post, Item: itemPostCtx(sc.item)})
		if ok {
			w.WriteString(inner)
			if key := plugin + "/" + block; css != "" && !r.blocks[key] {
				r.blocks[key] = true
				r.blockCSS.WriteString(css)
				if !strings.HasSuffix(css, "\n") {
					r.blockCSS.WriteString("\n")
				}
			}
		}
	}
	w.WriteString(`</div>`)
}

// pluginClass mirrors plugin.BlockClass: pb-<plugin>-<block> with only
// [a-z0-9-] kept.
func pluginClass(plugin, block string) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, c := range s {
			if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
				b.WriteRune(c)
			}
		}
		return b.String()
	}
	return "pb-" + clean(plugin) + "-" + clean(block)
}

// instanceChildren copies a component's layers for one placement: ids are
// prefixed with the instance id, and per-layer overrides (style, text,
// hidden) from the instance's props.overrides are applied.
func instanceChildren(children []*Node, prefix string, ov map[string]any) []*Node {
	out := make([]*Node, 0, len(children))
	for _, c := range children {
		if c == nil {
			continue
		}
		cp := *c
		cp.ID = prefix + "-" + c.ID
		if o, ok := ov[c.ID].(map[string]any); ok {
			if st, ok := o["style"].(map[string]any); ok {
				cp.Style = mergeMap(cloneMap(c.Style), st)
			}
			if t, ok := o["text"].(string); ok {
				cp.Props = cloneMap(c.Props)
				cp.Props["text"] = t
				delete(cp.Props, "rich") // the new plain text replaces formatted text
			}
			if getBool(o, "hidden") {
				hidden := false
				cp.Visible = &hidden
			}
		}
		cp.Children = instanceChildren(c.Children, prefix, ov)
		out = append(out, &cp)
	}
	return out
}

// SamePage reports whether a menu link points at the page at path (2.2.0,
// aria-current): same path, ignoring the query, the fragment, a trailing
// slash and percent-encoding; external links never match.
func SamePage(link, path string) bool {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Host != "" || u.Scheme != "" || !strings.HasPrefix(u.Path, "/") {
		return false
	}
	norm := func(p string) string {
		if x, err := url.PathUnescape(p); err == nil {
			p = x
		}
		if len(p) > 1 {
			p = strings.TrimRight(p, "/")
		}
		return p
	}
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return norm(u.Path) == norm(path)
}

// menuLinkDecls is the look of every link of a menu (2.2.0): style.linkPadding
// [vertical, horizontal] and style.linkRadius, so the current page's link can
// be a pill.
func menuLinkDecls(n *Node) []Decl {
	var d []Decl
	if p, ok := n.Style["linkPadding"].([]any); ok && len(p) == 2 && (num(p[0]) > 0 || num(p[1]) > 0) {
		d = append(d, Decl{"padding", px(num(p[0])) + " " + px(num(p[1]))})
	}
	if v, ok := getNum(n.Style, "linkRadius"); ok && v > 0 {
		d = append(d, Decl{"border-radius", px(v)})
	}
	return d
}

// truncRunes cuts s to at most n characters (not bytes).
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
