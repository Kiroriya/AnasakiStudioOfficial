package sitecode

// Phase 7 (B1): server-rendered designer blocks from JS plugins.
//
//	site.block('form', {
//	  label: { th: 'ฟอร์ม', en: 'Form' }, icon: '📝',
//	  props: [{ key: 'formId', type: 'select', label: 'Form' }],   // plugin.Field (manifest settings format)
//	  options: (ctx) => ({ formId: [{ value: '1', label: 'Contact' }] }), // dynamic select options
//	  render: (ctx, props) => '<form …>' | { html, css? },
//	  script: 'assets/form.js',   // optional package asset loaded with the block
//	  cache: 30,                  // optional seconds, per props / post
//	})
//
// The designer lists the block with the package blocks (GET /api/v1/blocks)
// and the node is the usual pluginBlock {plugin, block, values}. Rendering
// runs in the plugin unit (200 ms); the HTML is sanitised (forms, inputs and
// data-* attributes allowed; no scripts, styles or handlers) unless the
// plugin has the "head-scripts" scope. CSS is scoped under
// .pb-<plugin>-<block>.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// CodeBlockInfo is a registered site.block.
type CodeBlockInfo struct {
	Type    string
	Label   plugin.I18n
	Icon    string
	Props   []plugin.Field
	Dynamic bool   // options(ctx) fills select options
	Script  string // package asset (.js)
	Cache   int    // seconds
}

type regBlock struct {
	info    CodeBlockInfo
	render  goja.Callable
	options goja.Callable
	at      Problem
}

var blockTypeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,39}$`)

const (
	maxCodeBlocks       = 20
	defaultBlockTimeout = 200 * time.Millisecond
	blockOptionsTimeout = 500 * time.Millisecond
	maxBlockHTML        = 512 << 10
)

func (v *vm) siteBlock(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.block ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	if !v.u.plugin() {
		v.throw("site.block ใช้ได้เฉพาะปลั๊กอิน JavaScript (Site Code ใช้บล็อกโค้ดหน้าเว็บ หรือ collection)")
	}
	typ, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.block(%q)", typ)
	if !blockTypeRe.MatchString(typ) {
		v.throw("site.block: ชื่อ %q ไม่ถูกต้อง (a-z 0-9 - ยาว 2-40 ตัว)", c.Argument(0).String())
	}
	if len(v.blocks) >= maxCodeBlocks {
		v.throw("ลงทะเบียน site.block ได้ไม่เกิน %d รายการ", maxCodeBlocks)
	}
	o := optObject(v.rt, c.Argument(1))
	if o == nil {
		v.throw("%s: ต้องระบุ { label, render }", what)
	}
	lbl := v.i18nLabel(o.Get("label"), typ, what)
	info := CodeBlockInfo{Type: typ, Label: plugin.I18n{TH: lbl["th"], EN: lbl["en"]}, Icon: clip(optString(o, "icon"), 16)}
	rb := &regBlock{render: v.fnOpt(o, "render", what), at: v.caller()}
	if x := o.Get("options"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		fn, ok := goja.AssertFunction(x)
		if !ok {
			v.throw("%s: options ต้องเป็นฟังก์ชัน (ctx) => ({ <prop>: [{ value, label }] })", what)
		}
		rb.options, info.Dynamic = fn, true
	}
	if x := o.Get("props"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		s, ok, err := v.toJSON(x)
		if err != nil || !ok || json.Unmarshal([]byte(s), &info.Props) != nil {
			v.throw("%s: props ต้องเป็น array ของฟิลด์ เช่น [{ key: 'formId', type: 'select', label: 'Form' }]", what)
		}
		check := make([]plugin.Field, len(info.Props))
		copy(check, info.Props)
		for i := range check {
			if check[i].Type == plugin.FieldSelect && len(check[i].Options) == 0 && info.Dynamic {
				check[i].Options = []plugin.Option{{Value: ""}} // filled by options(ctx)
			}
		}
		if err := plugin.ValidateSchema(check); err != nil {
			v.throw("%s: props: %s", what, err.Error())
		}
		if err := plugin.NoSecrets(check); err != nil {
			v.throw("%s: props: %s", what, err.Error())
		}
	}
	if sc := optString(o, "script"); sc != "" {
		if !plugin.AssetPathOK(sc) || !strings.HasSuffix(strings.ToLower(sc), ".js") {
			v.throw("%s: script ต้องเป็นไฟล์ .js ในแพ็กเกจ (ไม่ใช่ code/ หรือ migrations/) เช่น 'assets/block.js'", what)
		}
		info.Script = sc
	}
	if x := o.Get("cache"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		n := int(x.ToInteger())
		if n < 0 || n > 3600 {
			v.throw("%s: cache ต้องอยู่ระหว่าง 0 ถึง 3600 วินาที", what)
		}
		info.Cache = n
	}
	rb.info = info
	v.blocks = append(v.blocks, rb)
	return goja.Undefined()
}

// registerJoints7 records site.adminNotice / dashboardWidget / block.
func (p *Program) registerJoints7(v *vm) []Problem {
	var probs []Problem
	for _, n := range v.notices {
		p.noticeAccess = append(p.noticeAccess, n.access)
	}
	seenW := map[string]bool{}
	for _, w := range v.widgets {
		if seenW[w.info.ID] {
			probs = append(probs, at(w.at, fmt.Sprintf("site.dashboardWidget %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", w.info.ID)))
			continue
		}
		seenW[w.info.ID] = true
		p.widgetList = append(p.widgetList, w.info)
	}
	p.blocks = map[string]*CodeBlockInfo{}
	for _, b := range v.blocks {
		if p.blocks[b.info.Type] != nil {
			probs = append(probs, at(b.at, fmt.Sprintf("site.block %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", b.info.Type)))
			continue
		}
		info := b.info
		p.blocks[info.Type] = &info
		p.blockList = append(p.blockList, &info)
	}
	return probs
}

func (e *Engine) blockTimeout() time.Duration {
	if e.BlockTimeout > 0 {
		return e.BlockTimeout
	}
	return defaultBlockTimeout
}

// systemSession is an anonymous session without a request (designer
// listings, block previews).
func (e *Engine) systemSession() *Session {
	req, _ := http.NewRequest(http.MethodGet, "http://site.local/", nil)
	return &Session{e: e, r: req, set: e.units(), setOK: true, lazy: true}
}

// CodeBlockInfos lists the site.block blocks of the running JS plugins for
// the designer; dynamic select options are resolved now (500 ms each).
func (e *Engine) CodeBlockInfos() []plugin.BlockInfo {
	var out []plugin.BlockInfo
	set := e.units()
	var s *Session
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	for _, lp := range set.loaded() {
		for _, b := range lp.prog.blockList {
			props := make([]plugin.Field, len(b.Props))
			copy(props, b.Props)
			if b.Dynamic {
				if s == nil {
					s = e.systemSession()
				}
				typ := b.Type
				res, err := s.callIn(lp.unit.ID, "block "+typ+" options", blockOptionsTimeout, func(v *vm) goja.Callable {
					for _, rb := range v.blocks {
						if rb.info.Type == typ {
							return rb.options
						}
					}
					return nil
				}, func(v *vm) []goja.Value { return []goja.Value{v.toJS(map[string]any{"locale": "th"})} })
				if m, ok := res.(map[string]any); ok && err == nil {
					for i := range props {
						list, ok := m[props[i].Key].([]any)
						if !ok || props[i].Type != plugin.FieldSelect {
							continue
						}
						props[i].Options = optionList(list)
					}
				}
			}
			for i := range props {
				if props[i].Options == nil && props[i].Type == plugin.FieldSelect {
					props[i].Options = []plugin.Option{}
				}
			}
			icon := b.Icon
			out = append(out, plugin.BlockInfo{Plugin: lp.unit.ID, PluginName: lp.unit.Name, Type: b.Type, Name: b.Label,
				Icon: icon, Props: props})
		}
	}
	return out
}

// optionList converts options(ctx) entries ({value, label} or plain values).
func optionList(list []any) []plugin.Option {
	out := []plugin.Option{}
	for _, x := range list {
		if len(out) >= 500 {
			break
		}
		var o plugin.Option
		switch t := x.(type) {
		case map[string]any:
			o.Value = plugin.ToString(t["value"])
			switch l := t["label"].(type) {
			case string:
				o.Label = plugin.I18n{TH: l, EN: l}
			case map[string]any:
				th, _ := l["th"].(string)
				en, _ := l["en"].(string)
				if th == "" {
					th = en
				}
				if en == "" {
					en = th
				}
				o.Label = plugin.I18n{TH: th, EN: en}
			}
		default:
			o.Value = plugin.ToString(t)
		}
		if o.Label.TH == "" {
			o.Label = plugin.I18n{TH: o.Value, EN: o.Value}
		}
		out = append(out, o)
	}
	return out
}

// RenderCodeBlock renders a site.block without a request (designer preview,
// pages rendered without a code session); ok=false when no running plugin
// registered it.
func (e *Engine) RenderCodeBlock(c design.BlockCall) (string, string, bool) {
	lp := e.units().get(c.Plugin)
	if lp == nil || lp.prog == nil || lp.prog.blocks[c.Block] == nil {
		return "", "", false
	}
	s := e.systemSession()
	defer s.Close()
	return s.CodeBlock(c)
}

// renderValues validates block values against the props; dynamic selects
// are checked as text (their options are only known to the designer).
func renderValues(b *CodeBlockInfo, values map[string]any) plugin.Values {
	props := make([]plugin.Field, len(b.Props))
	copy(props, b.Props)
	for i := range props {
		if props[i].Type == plugin.FieldSelect && (b.Dynamic || len(props[i].Options) == 0) {
			props[i].Type, props[i].Options, props[i].Default = plugin.FieldText, nil, plugin.ToString(props[i].Default)
		}
	}
	return plugin.Sanitize(props, values)
}

// CodeBlock renders a site.block in the session (public pages: the page's
// own code session); ok=false when no running plugin registered it.
func (s *Session) CodeBlock(c design.BlockCall) (string, string, bool) {
	lp := s.plugins().get(c.Plugin)
	if lp == nil || lp.prog == nil {
		return "", "", false
	}
	b := lp.prog.blocks[c.Block]
	if b == nil {
		return "", "", false
	}
	vals := renderValues(b, c.Values)
	var key string
	if b.Cache > 0 {
		kj, _ := json.Marshal([]any{lp.unit.ID, lp.unit.Version, b.Type, vals, c.Post["id"], c.Item["id"], s.e.gen(lp.unit.ID)})
		key = "block:" + string(kj)
		if hit, ok := s.e.blockCache.Get(key); ok {
			var r [2]string
			if json.Unmarshal([]byte(hit), &r) == nil {
				return r[0], r[1], true
			}
		}
	}
	ctx := map[string]any{"site": c.Site, "post": c.Post, "item": c.Item, "path": s.request().URL.Path}
	if b.Cache == 0 {
		// the query string (first value per key) — only for uncached blocks, a cached render would leak it
		q := map[string]string{}
		for k, vs := range s.request().URL.Query() {
			if len(vs) > 0 {
				q[k] = vs[0]
			}
		}
		ctx["query"] = q
	}
	out, err := s.callIn(lp.unit.ID, "block "+b.Type, s.e.blockTimeout(), func(v *vm) goja.Callable {
		for _, rb := range v.blocks {
			if rb.info.Type == b.Type {
				return rb.render
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		co := v.toJS(ctx).ToObject(v.rt)
		_ = co.Set("user", v.userValue())
		return []goja.Value{co, v.toJS(map[string]any(vals))}
	})
	if err != nil {
		return "", "", true // failed: an empty block (logged)
	}
	var html, css string
	switch x := out.(type) {
	case string:
		html = x
	case map[string]any:
		html, _ = x["html"].(string)
		css, _ = x["css"].(string)
	}
	if len(html) > maxBlockHTML {
		html = ""
	}
	if !lp.unit.can("head-scripts") {
		html = kses.SanitizeBlockHTML(html)
	}
	if css != "" {
		css = plugin.ScopeCSS(css, "."+plugin.BlockClass(lp.unit.ID, b.Type))
	}
	if b.Script != "" {
		if u := plugin.AssetURL(lp.unit.ID, lp.unit.Version, b.Script); u != "" {
			html += `<script defer src="` + htmlEscape(u) + `"></script>`
		}
	}
	if key != "" {
		j, _ := json.Marshal([2]string{html, css})
		_ = s.e.blockCache.Set(key, string(j), time.Duration(b.Cache)*time.Second)
	}
	return html, css, true
}

// pageResponse converts {page: {title, html}, status?, headers?}.
func (v *vm) pageResponse(o *goja.Object) (*Response, error) {
	res := &Response{Status: http.StatusOK, Headers: map[string]string{}}
	if st := o.Get("status"); st != nil && !goja.IsUndefined(st) && !goja.IsNull(st) {
		n := st.ToInteger()
		if n < 200 || n > 599 {
			return nil, &RunError{Message: "status " + strconv.FormatInt(n, 10) + " ไม่ถูกต้อง (หน้าในเลย์เอาต์: 200-599)"}
		}
		res.Status = int(n)
	}
	if h := o.Get("headers"); h != nil && !goja.IsUndefined(h) && !goja.IsNull(h) {
		ho := h.ToObject(v.rt)
		for _, k := range ho.Keys() {
			hv := ho.Get(k).String()
			if hopHeaders[strings.ToLower(k)] || strings.ContainsAny(k, "\r\n:") || strings.ContainsAny(hv, "\r\n") {
				continue
			}
			res.Headers[http.CanonicalHeaderKey(k)] = hv
		}
	}
	po := o.Get("page").ToObject(v.rt)
	pg := &PageResult{Title: clip(optString(po, "title"), 300), HTML: optString(po, "html")}
	if id := po.Get("postId"); id != nil && !goja.IsUndefined(id) && !goja.IsNull(id) {
		if n := id.ToInteger(); n > 0 {
			pg.PostID = n
		}
	}
	if len(pg.HTML) > MaxBinaryBody {
		return nil, &RunError{Message: "page.html ใหญ่เกินไป"}
	}
	res.Page = pg
	return res, nil
}
