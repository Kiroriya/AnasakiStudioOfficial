package plugin

import (
	"strings"

	"github.com/anasaki/studio/backend/internal/design"
)

// Block is a designer block contributed by a plugin. Built-in plugins render
// it with Go code; package plugins with a template (see packages.go).
type Block struct {
	Plugin string // filled by Registry.Block
	Type   string // ^[a-z0-9-]{2,40}$
	Name   I18n
	Icon   string
	Props  []Field
	// PropsFunc, when set, computes Props at request time (e.g. a select
	// listing the existing forms).
	PropsFunc func() []Field
	// CSS is emitted once per page; scope it under .pb-<plugin>-<type>.
	CSS string
	// Render returns the block's inner HTML. Values are already validated
	// against Props; escape everything you output.
	Render func(c BlockContext) string
}

// BlockContext is handed to Block.Render.
type BlockContext struct {
	Values Values
	Site   map[string]string
	Post   map[string]string // page / template post bindings
	Item   map[string]string // current collection item (inside collections)
}

func (b *Block) props() []Field {
	if b.PropsFunc != nil {
		return b.PropsFunc()
	}
	return b.Props
}

// BlockInfo is the API representation of a block (GET /api/v1/blocks).
type BlockInfo struct {
	Plugin     string  `json:"plugin"`
	PluginName string  `json:"pluginName"`
	Type       string  `json:"type"`
	Name       I18n    `json:"name"`
	Icon       string  `json:"icon"`
	Props      []Field `json:"props"`
}

// BlockClass is the wrapper class of a plugin block: pb-<plugin>-<block>.
func BlockClass(plugin, block string) string {
	return "pb-" + cleanIdent(plugin) + "-" + cleanIdent(block)
}

func cleanIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Blocks lists the blocks of active built-in plugins and active packages.
func (m *Manager) Blocks() []BlockInfo {
	out := []BlockInfo{}
	set := m.activeSet()
	for _, e := range m.entries {
		if !set[e.info.ID] {
			continue
		}
		for _, b := range e.reg.blocks {
			props := b.props()
			if props == nil {
				props = []Field{}
			}
			out = append(out, BlockInfo{Plugin: e.info.ID, PluginName: e.info.Name, Type: b.Type,
				Name: b.Name, Icon: b.Icon, Props: props})
		}
	}
	out = append(out, m.pkgs.blockInfos()...)
	if cb := m.codeBlocks(); cb != nil {
		out = mergeCodeBlocks(out, cb.CodeBlockInfos())
	}
	return out
}

// RenderBlock renders one plugin block; ok is false for an unknown or
// inactive plugin/block. html is the inner HTML (without the wrapper div).
func (m *Manager) RenderBlock(c design.BlockCall) (html, css string, ok bool) {
	if e := m.byID[c.Plugin]; e != nil {
		if !m.Active(c.Plugin) {
			return "", "", false
		}
		for _, b := range e.reg.blocks {
			if b.Type != c.Block || b.Render == nil {
				continue
			}
			ctx := BlockContext{Values: Sanitize(b.props(), c.Values), Site: c.Site, Post: c.Post, Item: c.Item}
			safeCall(c.Plugin, func() { html = b.Render(ctx) })
			return html, b.CSS, true
		}
		return "", "", false
	}
	if cb := m.codeBlocks(); cb != nil {
		if html, css, ok := cb.RenderCodeBlock(c); ok {
			return html, css, true
		}
	}
	return m.pkgs.renderBlock(c)
}

// ScopeCSS prefixes every selector of css with scope (".pb-x-y"). ":scope"
// and "&" refer to the wrapper itself; html/body/:root map to it too.
// @media/@supports/@container/@layer blocks are scoped recursively,
// @keyframes and @font-face are copied, @import/@charset are dropped.
//
// The input is first normalized so this parser and the browser's agree on
// where every rule starts and ends: comments are removed, unterminated
// strings are closed, stray backslashes dropped; braces, semicolons and
// commas inside strings or escapes (`\{`) are never structure. A rule whose
// prelude has unbalanced brackets is dropped (the browser would swallow the
// following rules into it).
func ScopeCSS(css, scope string) string {
	css = normalizeCSS(strings.ReplaceAll(css, "<", "")) // never close the <style> element
	var b strings.Builder
	scopeRules(&b, css, scope, 0)
	return b.String()
}

// normalizeCSS removes comments, closes unterminated strings (at the end of
// the line, as a browser would end them) and drops backslashes that escape
// nothing, so the result tokenizes the same way for scopeRules and a browser.
func normalizeCSS(css string) string {
	var b strings.Builder
	b.Grow(len(css))
	for i := 0; i < len(css); {
		c := css[i]
		switch {
		case c == '/' && i+1 < len(css) && css[i+1] == '*':
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				return b.String() // an unterminated comment runs to the end
			}
			b.WriteByte(' ')
			i += end + 4
		case c == '"' || c == '\'':
			b.WriteByte(c)
			i++
			closed := false
			for i < len(css) && !closed {
				d := css[i]
				if d == '\\' {
					if i+1 < len(css) {
						b.WriteString(css[i : i+2]) // escaped char or line continuation
						i += 2
					} else {
						i++
					}
					continue
				}
				if d == '\n' || d == '\r' || d == '\f' {
					break // the browser ends a bad string here: close it
				}
				b.WriteByte(d)
				i++
				closed = d == c
			}
			if !closed {
				b.WriteByte(c)
			}
		case c == '\\':
			if i+1 < len(css) && css[i+1] != '\n' && css[i+1] != '\r' && css[i+1] != '\f' {
				b.WriteString(css[i : i+2])
				i += 2
			} else {
				i++
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// skipToken returns the index after the string or escape starting at i
// (normalized CSS), or i+1 for any other byte.
func skipToken(css string, i int) int {
	switch c := css[i]; c {
	case '\\':
		return min(i+2, len(css))
	case '"', '\'':
		for j := i + 1; j < len(css); j++ {
			switch css[j] {
			case '\\':
				j++
			case c:
				return j + 1
			}
		}
		return len(css)
	}
	return i + 1
}

// indexStructural is strings.IndexAny outside strings and escapes.
func indexStructural(css string, from int, chars string) int {
	for j := from; j < len(css); j = skipToken(css, j) {
		if strings.IndexByte(chars, css[j]) >= 0 {
			return j
		}
	}
	return -1
}

func scopeRules(b *strings.Builder, css, scope string, depth int) {
	i := 0
	for i < len(css) {
		for i < len(css) && (css[i] == ' ' || css[i] == '\n' || css[i] == '\t' || css[i] == '\r' || css[i] == '\f') {
			i++
		}
		if i >= len(css) {
			return
		}
		at := indexStructural(css, i, "{;}")
		if at < 0 {
			return
		}
		prelude := strings.TrimSpace(css[i:at])
		switch css[at] {
		case ';': // statement at-rule (@import, @charset) or junk
			i = at + 1
			continue
		case '}': // stray close
			i = at + 1
			continue
		}
		bodyStart := at + 1
		bodyEnd := matchBrace(css, bodyStart)
		body := css[bodyStart:bodyEnd]
		i = bodyEnd + 1
		if !balanced(prelude) {
			continue
		}
		if strings.HasPrefix(prelude, "@") {
			name := strings.ToLower(strings.Fields(prelude + " ")[0])
			switch name {
			case "@media", "@supports", "@container", "@layer":
				if depth > 5 {
					continue
				}
				b.WriteString(prelude + "{")
				scopeRules(b, body, scope, depth+1)
				b.WriteString("}\n")
			case "@keyframes", "@-webkit-keyframes", "@font-face":
				b.WriteString(prelude + "{" + body + "}\n")
			}
			continue
		}
		b.WriteString(scopeSelectors(prelude, scope) + "{" + strings.TrimSpace(body) + "}\n")
	}
}

// matchBrace returns the index of the '}' closing the block starting at i
// (braces inside strings and escapes don't count).
func matchBrace(css string, i int) int {
	depth := 1
	for j := i; j < len(css); j = skipToken(css, j) {
		switch css[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(css)
}

// balanced reports whether the ( ) and [ ] of a prelude pair up (outside
// strings and escapes).
func balanced(prelude string) bool {
	var stack []byte
	for j := 0; j < len(prelude); j = skipToken(prelude, j) {
		switch c := prelude[j]; c {
		case '(', '[':
			stack = append(stack, c)
		case ')', ']':
			open := byte('(')
			if c == ']' {
				open = '['
			}
			if len(stack) == 0 || stack[len(stack)-1] != open {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0
}

func scopeSelectors(prelude, scope string) string {
	var parts []string
	depth, start := 0, 0
	for j := 0; j < len(prelude); j = skipToken(prelude, j) {
		switch prelude[j] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, prelude[start:j])
				start = j + 1
			}
		}
	}
	parts = append(parts, prelude[start:])
	for k, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case p == "html" || p == "body" || p == ":root":
			p = scope
		case strings.HasPrefix(p, ":scope"):
			p = scope + p[len(":scope"):]
		case strings.HasPrefix(p, "&"):
			p = scope + p[1:]
		default:
			p = scope + " " + p
		}
		parts[k] = p
	}
	return strings.Join(parts, ",")
}
