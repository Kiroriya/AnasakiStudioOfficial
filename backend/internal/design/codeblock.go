package design

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TypeCodeBlock is the "Frontend code" block (Site Code phase 5, design
// §8.7): props {html, css, js}, added or changed only by users with
// edit_scripts (checked by the API on save). Rendered as
//
//	<div class="n-<id> rea-cb-<id>" data-rea-code-block="<id>">html</div>
//	<style>css, every selector inside .rea-cb-<id></style>
//	<script type="module">…(async (el, anasaki) => { js })(el, window.anasaki)…</script>
//
// <id> keeps only [A-Za-z0-9_-] of the node id. The JS runs once per
// rendered copy of the block (a block inside a collection repeats), after
// the page is parsed, with el = the block root. <style> elements inside the
// HTML are scoped like the block CSS.
const TypeCodeBlock = "codeblock"

// codeBlockID is the node id as used in the class and data-rea-code-block.
func codeBlockID(id string) string { return strings.TrimPrefix(ClassName(id), "n-") }

// CodeBlockClass is the scope class of a code block ("rea-cb-<id>").
func CodeBlockClass(id string) string { return "rea-cb-" + codeBlockID(id) }

var (
	closeStyleRe  = regexp.MustCompile(`(?i)</(style)`)
	closeScriptRe = regexp.MustCompile(`(?i)</(script)`)
	commentOpenRe = regexp.MustCompile(`<!--`)
)

// writeCodeBlock renders a code block node (cls = its layout class).
func (r *renderer) writeCodeBlock(n *Node, cls string) {
	w := r.body
	id := codeBlockID(n.ID)
	w.WriteString(`<div class="` + cls + ` ` + CodeBlockClass(n.ID) + `" data-rea-code-block="` + id + `">`)
	w.WriteString(scopeInlineStyles(getStr(n.Props, "html"), n.ID))
	w.WriteString(`</div>`)
	if r.codeBlocks == nil {
		r.codeBlocks = map[string]bool{}
	}
	if r.codeBlocks[id] { // repeated copies (collection items) share one style and script
		return
	}
	r.codeBlocks[id] = true
	if scoped := ScopeCSS(getStr(n.Props, "css"), n.ID); scoped != "" {
		w.WriteString(`<style>` + scoped + `</style>`)
	}
	if js := getStr(n.Props, "js"); strings.TrimSpace(js) != "" {
		r.runtime = true // window.anasaki
		js = closeScriptRe.ReplaceAllString(js, `<\/$1`)
		js = commentOpenRe.ReplaceAllString(js, `<\!--`)
		w.WriteString(`<script type="module">document.querySelectorAll('[data-rea-code-block="` + id + `"]').forEach(el => ` +
			`(async (el, anasaki) => {` + "\n" + js + "\n" + `})(el, window.anasaki).catch(e => console.error('code block ` + id + `:', e)));</script>`)
	}
}

// ---------------------------------------------------------------- CSS scoping

// ScopeCSS limits a stylesheet to the code block id. It mirrors scopeCss in
// frontend/src/app/editor/codeblock.ts byte for byte (shared vectors:
// testdata/codeblock_scope_vectors.json): comments removed, every selector
// prefixed with ".rea-cb-<id>" (`:root` / `:scope` become that class),
// @media / @supports / @container / @layer / @scope / @document /
// @starting-style scoped recursively (empty ones dropped), other block
// at-rules (@keyframes, @font-face…) kept verbatim, @import and @charset
// dropped, and `</style` written as `<\/style`.
func ScopeCSS(css, id string) string {
	p := &cssParser{s: stripCSSComments(css), prefix: "." + CodeBlockClass(id)}
	out := strings.Join(p.rules(false), "\n")
	return closeStyleRe.ReplaceAllString(out, `<\/$1`)
}

var (
	groupAt = map[string]bool{"media": true, "supports": true, "container": true, "layer": true,
		"scope": true, "document": true, "-moz-document": true, "starting-style": true}
	dropAt = map[string]bool{"import": true, "charset": true}
)

// isJSSpace is JavaScript's \s.
func isJSSpace(r rune) bool { return unicode.IsSpace(r) || r == 0xfeff }

func collapseSpace(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if isJSSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

func wrapRule(head, body string) string {
	b := strings.TrimFunc(body, isJSSpace)
	if b == "" {
		return head + " {}"
	}
	return head + " { " + b + " }"
}

// skipCSSString returns the index after the string starting at s[i]
// (unterminated: the end of the line or input).
func skipCSSString(s string, i int) int {
	q := s[i]
	j := i + 1
	for j < len(s) {
		switch s[j] {
		case '\\':
			j += 2
			continue
		case q:
			return j + 1
		case '\n':
			return j
		}
		j++
	}
	return len(s)
}

func stripCSSComments(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			end := min(skipCSSString(s, i), len(s))
			b.WriteString(s[i:end])
			i = end
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			if end := strings.Index(s[i+2:], "*/"); end < 0 {
				i = len(s)
			} else {
				i += 2 + end + 2
			}
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// splitSelectors splits at top-level commas (not inside (), [] or strings).
func splitSelectors(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			i = skipCSSString(s, i) - 1
		case c == '\\':
			i++
		case c == '(' || c == '[':
			depth++
		case (c == ')' || c == ']') && depth > 0:
			depth--
		case c == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[min(start, len(s)):])
}

var rootPseudoRe = regexp.MustCompile(`(?i):(?:root|scope)`)

func isWordByte(c byte) bool {
	return c == '_' || c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// replaceRoot replaces :root / :scope (not followed by [A-Za-z0-9_-]) with
// prefix; false when there is none.
func replaceRoot(sel, prefix string) (string, bool) {
	var b strings.Builder
	found, last := false, 0
	for _, m := range rootPseudoRe.FindAllStringIndex(sel, -1) {
		if m[1] < len(sel) && isWordByte(sel[m[1]]) {
			continue
		}
		found = true
		b.WriteString(sel[last:m[0]] + prefix)
		last = m[1]
	}
	if !found {
		return sel, false
	}
	b.WriteString(sel[last:])
	return b.String(), true
}

func scopeSelectorList(list, prefix string) string {
	var out []string
	for _, raw := range splitSelectors(collapseSpace(list)) {
		sel := strings.TrimFunc(raw, isJSSpace)
		if sel == "" {
			continue
		}
		if r, ok := replaceRoot(sel, prefix); ok {
			out = append(out, r)
		} else {
			out = append(out, prefix+" "+sel)
		}
	}
	return strings.Join(out, ", ")
}

var atNameRe = regexp.MustCompile(`^@([A-Za-z0-9_-]+)`)

type cssParser struct {
	s      string
	i      int
	prefix string
}

func (p *cssParser) ws() {
	for p.i < len(p.s) {
		r, n := utf8.DecodeRuneInString(p.s[p.i:])
		if r != ';' && !isJSSpace(r) {
			return
		}
		p.i += n
	}
}

// prelude reads up to a top-level {, ; or } (not consumed); stop 0 = end.
func (p *cssParser) prelude() (string, byte) {
	s, start, depth := p.s, p.i, 0
	for p.i < len(s) {
		c := s[p.i]
		switch {
		case c == '"' || c == '\'':
			p.i = skipCSSString(s, p.i)
			continue
		case c == '\\':
			p.i += 2
			continue
		case c == '(' || c == '[':
			depth++
		case (c == ')' || c == ']') && depth > 0:
			depth--
		case depth == 0 && (c == '{' || c == ';' || c == '}'):
			return s[start:p.i], c
		}
		p.i++
	}
	p.i = len(s)
	return s[start:], 0
}

// block returns the body after a { up to its } (consumed; missing = to the end).
func (p *cssParser) block() string {
	s, start, depth := p.s, p.i, 1
	for p.i < len(s) {
		c := s[p.i]
		switch {
		case c == '"' || c == '\'':
			p.i = skipCSSString(s, p.i)
			continue
		case c == '\\':
			p.i += 2
			continue
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				body := s[start:p.i]
				p.i++
				return body
			}
		}
		p.i++
	}
	p.i = len(s)
	return s[min(start, len(s)):]
}

func (p *cssParser) rules(nested bool) []string {
	var out []string
	for {
		p.ws()
		if p.i >= len(p.s) {
			return out
		}
		if p.s[p.i] == '}' {
			p.i++
			if nested {
				return out
			}
			continue // stray
		}
		text, stop := p.prelude()
		head := strings.TrimFunc(collapseSpace(text), isJSSpace)
		if strings.HasPrefix(head, "@") {
			name := ""
			if m := atNameRe.FindStringSubmatch(head); m != nil {
				name = strings.ToLower(m[1])
			}
			if stop != '{' {
				if stop == ';' {
					p.i++
				}
				if !dropAt[name] && name != "" {
					out = append(out, head+";")
				}
				continue
			}
			p.i++
			if groupAt[name] {
				if inner := p.rules(true); len(inner) > 0 {
					out = append(out, head+" {\n"+strings.Join(inner, "\n")+"\n}")
				}
			} else if body := p.block(); !dropAt[name] {
				out = append(out, wrapRule(head, body))
			}
			continue
		}
		if stop != '{' {
			if stop == ';' {
				p.i++ // a declaration outside a rule
			}
			continue
		}
		p.i++
		body := p.block()
		if sel := scopeSelectorList(head, p.prefix); sel != "" {
			out = append(out, wrapRule(sel, body))
		}
	}
}

var styleElemRe = regexp.MustCompile(`(?is)(<style\b[^>]*>)(.*?)(</style\s*>)`)

// scopeInlineStyles scopes the <style> elements inside the block HTML.
func scopeInlineStyles(html, id string) string {
	if !strings.Contains(strings.ToLower(html), "<style") {
		return html
	}
	return styleElemRe.ReplaceAllStringFunc(html, func(m string) string {
		g := styleElemRe.FindStringSubmatch(m)
		return g[1] + ScopeCSS(g[2], id) + g[3]
	})
}

// hasImport reports an @import (any case, or an escaped at-keyword that
// could spell one) outside strings and comments: refused on save.
func hasImport(css string) bool {
	s := stripCSSComments(css)
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"' || s[i] == '\'':
			i = skipCSSString(s, i) - 1
		case s[i] == '@':
			if i+1 < len(s) && s[i+1] == '\\' {
				return true
			}
			if len(s) >= i+7 && strings.EqualFold(s[i+1:i+7], "import") {
				return true
			}
		}
	}
	return false
}

// CSSError is a code block stylesheet refused on save.
type CSSError struct{ Msg string }

func (e *CSSError) Error() string { return e.Msg }

// ---------------------------------------------------------------- saving

// CodeBlock is one code block of a design document.
type CodeBlock struct {
	ID, HTML, CSS, JS string
}

func (b CodeBlock) empty() bool {
	return strings.TrimSpace(b.HTML) == "" && strings.TrimSpace(b.CSS) == "" && strings.TrimSpace(b.JS) == ""
}

func (b CodeBlock) sig() string { return b.ID + "\x00" + b.HTML + "\x00" + b.CSS + "\x00" + b.JS }

// walkJSON calls fn for every object of a decoded JSON value.
func walkJSON(v any, fn func(m map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, c := range t {
			walkJSON(c, fn)
		}
	case []any:
		for _, c := range t {
			walkJSON(c, fn)
		}
	}
}

func blockOf(m map[string]any) (CodeBlock, bool) {
	if m["type"] != TypeCodeBlock {
		return CodeBlock{}, false
	}
	id, _ := m["id"].(string)
	p, _ := m["props"].(map[string]any)
	return CodeBlock{ID: id, HTML: getStr(p, "html"), CSS: getStr(p, "css"), JS: getStr(p, "js")}, true
}

// CodeBlocks lists the code blocks anywhere in a design document (also
// inside component variants).
func CodeBlocks(raw json.RawMessage) []CodeBlock {
	var doc any
	if len(raw) == 0 || json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []CodeBlock
	walkJSON(doc, func(m map[string]any) {
		if b, ok := blockOf(m); ok {
			out = append(out, b)
		}
	})
	return out
}

func trustedSigs(trusted []json.RawMessage) map[string]bool {
	sigs := map[string]bool{}
	for _, t := range trusted {
		for _, b := range CodeBlocks(t) {
			sigs[b.sig()] = true
		}
	}
	return sigs
}

// ChangedCodeBlocks returns the non-empty code blocks of raw that are not
// identical (id, html, css, js) to a block of one of the trusted documents
// (e.g. the stored version): what a save adds or changes.
func ChangedCodeBlocks(raw json.RawMessage, trusted ...json.RawMessage) []CodeBlock {
	blocks := CodeBlocks(raw)
	if len(blocks) == 0 {
		return nil
	}
	sigs := trustedSigs(trusted)
	var out []CodeBlock
	for _, b := range blocks {
		if !b.empty() && !sigs[b.sig()] {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CheckCodeBlockCSS refuses @import in the stylesheets of blocks (block CSS
// and <style> in the HTML; rendering would drop it); the error names the block.
func CheckCodeBlockCSS(blocks []CodeBlock) error {
	for _, b := range blocks {
		if hasImport(b.CSS) || (strings.Contains(strings.ToLower(b.HTML), "<style") && hasImport(b.HTML)) {
			return &CSSError{Msg: "บล็อกโค้ด " + b.ID + ": ใช้ @import ไม่ได้ (ใส่ CSS ในบล็อก หรือใช้ <link> ใน HTML) / @import is not allowed in a Frontend code block"}
		}
	}
	return nil
}

// StripCodeBlocks empties (html, css, js) every code block of raw that is
// not identical to one of the trusted documents: for callers that may not
// write code (preview, backup import by a user without edit_scripts).
// Unchanged blocks are kept byte for byte.
func StripCodeBlocks(raw json.RawMessage, trusted ...json.RawMessage) json.RawMessage {
	if len(ChangedCodeBlocks(raw, trusted...)) == 0 {
		return raw
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	sigs := trustedSigs(trusted)
	walkJSON(doc, func(m map[string]any) {
		if b, ok := blockOf(m); ok && !b.empty() && !sigs[b.sig()] {
			if p, ok := m["props"].(map[string]any); ok {
				p["html"], p["css"], p["js"] = "", "", ""
			}
		}
	})
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}
