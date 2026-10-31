package design

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Binding expressions: {{ item.price * item.qty | number:2 }}.
//
// A small, safe language (NOT JavaScript): literals (numbers, 'str', "str",
// true, false, null), dotted paths (item.title, page.total, $index,
// item.tags.length), unary - !, binary * / % + - < <= > >= == != && || ??,
// the ternary ?:, parentheses and pipes (| name, | name:arg1:arg2 with
// literal or path arguments). No calls, indexing or assignment.
//
// IMPORTANT: the editor (frontend) has a TypeScript twin of this evaluator;
// both must pass testdata/expr_vectors.json.

// Expr is a parsed expression.
type Expr struct{ root exprNode }

type exprNode interface{}

type (
	litNode  struct{ v any }
	pathNode struct {
		root string
		keys []string
	}
	unaryNode struct {
		op string
		x  exprNode
	}
	binaryNode struct {
		op   string
		a, b exprNode
	}
	condNode struct{ c, a, b exprNode }
	pipeNode struct {
		x     exprNode
		name  string
		args  []exprNode
		known bool // a core formatter
	}
)

// ---------------------------------------------------------------- lexer

type tokKind int

const (
	tEOF tokKind = iota
	tNum
	tStr
	tIdent
	tOp
)

type token struct {
	k   tokKind
	s   string
	num float64
	pos int
}

var exprOps = []string{"&&", "||", "??", "==", "!=", "<=", ">=", "<", ">", "+", "-", "*", "/", "%", "!", "?", ":", "|", ".", "(", ")"}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentPart(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }

func lex(s string) ([]token, error) {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
				k := j + 1
				if k < len(s) && (s[k] == '+' || s[k] == '-') {
					k++
				}
				if k < len(s) && s[k] >= '0' && s[k] <= '9' {
					j = k
					for j < len(s) && s[j] >= '0' && s[j] <= '9' {
						j++
					}
				}
			}
			f, err := strconv.ParseFloat(s[i:j], 64)
			if err != nil || j < len(s) && isIdentStart(s[j]) {
				return nil, fmt.Errorf("ตัวเลขไม่ถูกต้องที่ตำแหน่ง %d", i+1)
			}
			out = append(out, token{k: tNum, num: f, s: s[i:j], pos: i})
			i = j
		case c == '\'' || c == '"':
			var b strings.Builder
			j := i + 1
			closed := false
			for j < len(s) {
				d := s[j]
				if d == '\\' && j+1 < len(s) {
					switch e := s[j+1]; e {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					case 'r':
						b.WriteByte('\r')
					default:
						b.WriteByte(e)
					}
					j += 2
					continue
				}
				if d == c {
					closed = true
					j++
					break
				}
				b.WriteByte(d)
				j++
			}
			if !closed {
				return nil, fmt.Errorf("ข้อความไม่มีเครื่องหมายปิด %c", c)
			}
			out = append(out, token{k: tStr, s: b.String(), pos: i})
			i = j
		case isIdentStart(c):
			j := i + 1
			for j < len(s) && isIdentPart(s[j]) {
				j++
			}
			out = append(out, token{k: tIdent, s: s[i:j], pos: i})
			i = j
		default:
			matched := ""
			for _, op := range exprOps {
				if strings.HasPrefix(s[i:], op) {
					matched = op
					break
				}
			}
			// "=" alone (assignment), "===", "[", ",", "{" … are not part of the language
			if matched == "" || (matched == "==" || matched == "!=") && strings.HasPrefix(s[i+2:], "=") {
				r, _ := utf8.DecodeRuneInString(s[i:])
				return nil, fmt.Errorf("ใช้ %q ในนิพจน์ไม่ได้ (ตำแหน่ง %d)", string(r), i+1)
			}
			out = append(out, token{k: tOp, s: matched, pos: i})
			i += len(matched)
		}
	}
	return append(out, token{k: tEOF, pos: len(s)}), nil
}

// ---------------------------------------------------------------- parser

type parser struct {
	toks []token
	i    int
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }
func (p *parser) isOp(s string) bool {
	t := p.peek()
	return t.k == tOp && t.s == s
}

func (p *parser) fail(t token) error {
	if t.k == tEOF {
		return fmt.Errorf("นิพจน์จบไม่สมบูรณ์")
	}
	s := t.s
	if t.k == tStr {
		s = "'" + s + "'"
	}
	return fmt.Errorf("ไม่คาดว่าจะพบ %q (ตำแหน่ง %d)", s, t.pos+1)
}

// maxExprLen bounds an expression's source.
const maxExprLen = 2000

// ParseExpr parses one expression (the text between {{ and }}).
func ParseExpr(src string) (*Expr, error) {
	if len(src) > maxExprLen {
		return nil, fmt.Errorf("นิพจน์ยาวเกิน %d ตัวอักษร", maxExprLen)
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	if len(toks) == 1 {
		return nil, fmt.Errorf("นิพจน์ว่าง")
	}
	p := &parser{toks: toks}
	n, err := p.pipeline(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.k != tEOF {
		return nil, p.fail(t)
	}
	return &Expr{root: n}, nil
}

const maxExprDepth = 64

func (p *parser) pipeline(depth int) (exprNode, error) {
	x, err := p.ternary(depth)
	if err != nil {
		return nil, err
	}
	for p.isOp("|") {
		p.next()
		t := p.next()
		if t.k != tIdent {
			return nil, p.fail(t)
		}
		pn := &pipeNode{x: x, name: t.s, known: coreFormatters[t.s]}
		// phase 6: plugin formatters "ns:name" (| thai:baht) — written without
		// spaces, the namespace is not a core formatter and the second
		// identifier is not a root or a path (else it is an argument). Same
		// rule as the editor's evaluator.
		if c, nx := p.peek(), p.toks[min(p.i+1, len(p.toks)-1)]; !pn.known && c.k == tOp && c.s == ":" && c.pos == t.pos+len(t.s) &&
			nx.k == tIdent && nx.pos == c.pos+1 && !exprRoots[nx.s] && !strings.Contains(nx.s, "$") {
			after := p.toks[min(p.i+2, len(p.toks)-1)]
			if !(after.k == tOp && after.s == ".") {
				pn.name = t.s + ":" + nx.s
				p.i += 2
			}
		}
		for p.isOp(":") {
			p.next()
			a, err := p.pipeArg()
			if err != nil {
				return nil, err
			}
			pn.args = append(pn.args, a)
		}
		x = pn
	}
	return x, nil
}

func (p *parser) pipeArg() (exprNode, error) {
	t := p.peek()
	if t.k == tOp && t.s == "-" && p.toks[p.i+1].k == tNum {
		p.next()
		n := p.next()
		return &litNode{v: -n.num}, nil
	}
	switch t.k {
	case tNum, tStr:
		return p.primary(0)
	case tIdent:
		return p.primary(0)
	}
	return nil, p.fail(t)
}

func (p *parser) ternary(depth int) (exprNode, error) {
	if depth > maxExprDepth {
		return nil, fmt.Errorf("นิพจน์ซ้อนกันลึกเกินไป")
	}
	c, err := p.binary(0, depth)
	if err != nil {
		return nil, err
	}
	if !p.isOp("?") {
		return c, nil
	}
	p.next()
	a, err := p.ternary(depth + 1)
	if err != nil {
		return nil, err
	}
	if !p.isOp(":") {
		return nil, p.fail(p.peek())
	}
	p.next()
	b, err := p.ternary(depth + 1)
	if err != nil {
		return nil, err
	}
	return &condNode{c: c, a: a, b: b}, nil
}

// binary operator levels, lowest first.
var binLevels = [][]string{{"??"}, {"||"}, {"&&"}, {"==", "!="}, {"<", "<=", ">", ">="}, {"+", "-"}, {"*", "/", "%"}}

func (p *parser) binary(level, depth int) (exprNode, error) {
	if level == len(binLevels) {
		return p.unary(depth)
	}
	a, err := p.binary(level+1, depth)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.k != tOp || !contains(binLevels[level], t.s) {
			return a, nil
		}
		p.next()
		b, err := p.binary(level+1, depth)
		if err != nil {
			return nil, err
		}
		a = &binaryNode{op: t.s, a: a, b: b}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (p *parser) unary(depth int) (exprNode, error) {
	if depth > maxExprDepth {
		return nil, fmt.Errorf("นิพจน์ซ้อนกันลึกเกินไป")
	}
	if p.isOp("-") || p.isOp("!") {
		op := p.next().s
		x, err := p.unary(depth + 1)
		if err != nil {
			return nil, err
		}
		return &unaryNode{op: op, x: x}, nil
	}
	return p.primary(depth)
}

func (p *parser) primary(depth int) (exprNode, error) {
	t := p.next()
	switch t.k {
	case tNum:
		return &litNode{v: t.num}, nil
	case tStr:
		return &litNode{v: t.s}, nil
	case tIdent:
		switch t.s {
		case "true":
			return &litNode{v: true}, nil
		case "false":
			return &litNode{v: false}, nil
		case "null", "undefined":
			return &litNode{v: nil}, nil
		}
		pn := &pathNode{root: t.s}
		for p.isOp(".") {
			p.next()
			k := p.next()
			if k.k != tIdent {
				return nil, p.fail(k)
			}
			pn.keys = append(pn.keys, k.s)
		}
		return pn, nil
	case tOp:
		if t.s == "(" {
			x, err := p.pipeline(depth + 1)
			if err != nil {
				return nil, err
			}
			if !p.isOp(")") {
				return nil, p.fail(p.peek())
			}
			p.next()
			return x, nil
		}
	}
	return nil, p.fail(t)
}

// ---------------------------------------------------------------- evaluation

// Env is what an expression can see.
type Env struct {
	// Var returns a root variable (item, page, post, site, query, $index,
	// $empty); nil when unknown.
	Var func(name string) any
	// Locale "th" uses Thai month names in | date; anything else English.
	Locale string
	// Loc is the timezone dates are shown in (nil = UTC).
	Loc *time.Location
	// Format runs a formatter that is not a core one (site.formatter);
	// ok=false when there is none.
	Format func(name string, v any, args []any) (string, bool)
}

// blockedKeys never resolve (prototype pollution / sandbox escapes in the
// TypeScript twin).
var blockedKeys = map[string]bool{"__proto__": true, "constructor": true, "prototype": true}

// Eval evaluates the expression.
func (e *Expr) Eval(env *Env) any {
	if e == nil {
		return nil
	}
	return eval(e.root, env)
}

func eval(n exprNode, env *Env) any {
	switch x := n.(type) {
	case *litNode:
		return x.v
	case *pathNode:
		if blockedKeys[x.root] || env == nil || env.Var == nil {
			return nil
		}
		if x.root == "visitor" && len(x.keys) >= 1 && x.keys[0] == "is" { // "is" is reserved: visitor.is alone is null
			if len(x.keys) == 1 {
				return nil
			}
			return visitorIs(env.Var("visitor"), x.keys[1:])
		}
		v := env.Var(x.root)
		for _, k := range x.keys {
			if v == nil {
				return nil
			}
			v = member(v, k)
		}
		return v
	case *unaryNode:
		v := eval(x.x, env)
		if x.op == "!" {
			return !Truthy(v)
		}
		f, ok := arith(v)
		if !ok {
			return nil
		}
		return -f
	case *condNode:
		if Truthy(eval(x.c, env)) {
			return eval(x.a, env)
		}
		return eval(x.b, env)
	case *binaryNode:
		switch x.op {
		case "&&":
			return Truthy(eval(x.a, env)) && Truthy(eval(x.b, env))
		case "||":
			return Truthy(eval(x.a, env)) || Truthy(eval(x.b, env))
		case "??":
			if a := eval(x.a, env); a != nil {
				return a
			}
			return eval(x.b, env)
		}
		return binop(x.op, eval(x.a, env), eval(x.b, env))
	case *pipeNode:
		v := eval(x.x, env)
		args := make([]any, len(x.args))
		for i, a := range x.args {
			args[i] = eval(a, env)
		}
		return applyFormatter(x.name, x.known, v, args, env)
	}
	return nil
}

// member reads key k of v (maps; "length" of arrays and strings).
func member(v any, k string) any {
	if blockedKeys[k] {
		return nil
	}
	switch m := v.(type) {
	case map[string]any:
		if x, ok := m[k]; ok {
			return x
		}
	case PostCtx:
		if x, ok := m[k]; ok {
			return x
		}
	case map[string]string:
		if x, ok := m[k]; ok {
			return x
		}
	case []any:
		if k == "length" {
			return float64(len(m))
		}
	case []string:
		if k == "length" {
			return float64(len(m))
		}
	case string:
		if k == "length" {
			return float64(utf8.RuneCountInString(m))
		}
	}
	return nil
}

var numericRe = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$`)

// toNum converts numbers and numeric strings.
func toNum(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x)
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(x)
		if !numericRe.MatchString(s) {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

// arith is toNum plus booleans (true = 1), for arithmetic.
func arith(v any) (float64, bool) {
	if b, ok := v.(bool); ok {
		if b {
			return 1, true
		}
		return 0, true
	}
	return toNum(v)
}

func isNonNumericString(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	_, num := toNum(s)
	return !num
}

func binop(op string, a, b any) any {
	switch op {
	case "==":
		return looseEq(a, b)
	case "!=":
		return !looseEq(a, b)
	case "<", "<=", ">", ">=":
		// numeric when both sides are numeric, as strings when both are
		// strings, otherwise false
		x, ok1 := toNum(a)
		y, ok2 := toNum(b)
		var c int
		as, aStr := a.(string)
		bs, bStr := b.(string)
		switch {
		case ok1 && ok2:
			switch {
			case x < y:
				c = -1
			case x > y:
				c = 1
			}
		case aStr && bStr:
			c = strings.Compare(as, bs)
		default:
			return false
		}
		switch op {
		case "<":
			return c < 0
		case "<=":
			return c <= 0
		case ">":
			return c > 0
		}
		return c >= 0
	case "+":
		if isNonNumericString(a) || isNonNumericString(b) {
			return Stringify(a) + Stringify(b)
		}
	}
	x, ok1 := arith(a)
	y, ok2 := arith(b)
	if !ok1 || !ok2 {
		return nil
	}
	var r float64
	switch op {
	case "+":
		r = x + y
	case "-":
		r = x - y
	case "*":
		r = x * y
	case "/":
		if y == 0 {
			return nil
		}
		r = x / y
	case "%":
		if y == 0 {
			return nil
		}
		r = math.Mod(x, y)
	}
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return nil
	}
	return r
}

// looseEq: numeric when one side is a number and both are numeric;
// otherwise the same type and strictly equal; null == null.
func looseEq(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	_, aStr := a.(string)
	_, bStr := b.(string)
	_, aBool := a.(bool)
	_, bBool := b.(bool)
	aNum := !aStr && !aBool
	bNum := !bStr && !bBool
	if x, ok1 := toNum(a); ok1 && (aNum || bNum) {
		if y, ok2 := toNum(b); ok2 {
			return x == y
		}
		return false
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	}
	return false
}

// Truthy: false, null, "", 0, "0" and empty arrays are false.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != "" && x != "0"
	case []any:
		return len(x) > 0
	case []string:
		return len(x) > 0
	}
	if f, ok := toNum(v); ok {
		return f != 0
	}
	if f, ok := v.(float64); ok && math.IsNaN(f) {
		return false
	}
	return true
}

// FormatNumber prints a number with up to 10 decimals, no trailing zeros.
func FormatNumber(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	if math.Abs(f) < 1e15 {
		f = math.Round(f*1e10) / 1e10
	}
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Stringify renders a value: null → "", booleans "true"/"false", numbers
// without trailing zeros; arrays and objects render "" (use | json).
func Stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []any, map[string]any, []string, PostCtx, map[string]string:
		return ""
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	}
	if f, ok := toNum(v); ok {
		return FormatNumber(f)
	}
	return fmt.Sprint(v)
}

// ---------------------------------------------------------------- formatters

var coreFormatters = map[string]bool{"number": true, "date": true, "upper": true, "lower": true,
	"truncate": true, "default": true, "json": true}

// exprRoots are the context roots (an identifier after "fmt:" that names one
// is an argument, not a formatter namespace).
var exprRoots = map[string]bool{"item": true, "page": true, "post": true, "site": true, "query": true, "visitor": true,
	"$index": true, "$empty": true}

// CoreFormatters lists the built-in formatter names.
func CoreFormatters() []string {
	return []string{"date", "default", "json", "lower", "number", "truncate", "upper"}
}

func applyFormatter(name string, known bool, v any, args []any, env *Env) any {
	arg := func(i int) any {
		if i < len(args) {
			return args[i]
		}
		return nil
	}
	if !known {
		if env != nil && env.Format != nil {
			if s, ok := env.Format(name, v, args); ok {
				return s
			}
		}
		return v // unknown formatter: unchanged
	}
	switch name {
	case "upper":
		return strings.ToUpper(Stringify(v))
	case "lower":
		return strings.ToLower(Stringify(v))
	case "default":
		if v == nil || v == "" {
			return arg(0)
		}
		return v
	case "truncate":
		n := 50
		if f, ok := toNum(arg(0)); ok {
			n = int(f)
		}
		s := Stringify(v)
		if n < 0 {
			n = 0
		}
		if utf8.RuneCountInString(s) <= n {
			return s
		}
		r := []rune(s)
		return string(r[:n]) + "…"
	case "json":
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return ""
		}
		return strings.TrimRight(b.String(), "\n")
	case "number":
		f, ok := toNum(v)
		if !ok {
			return v
		}
		dec := -1
		if d, ok := toNum(arg(0)); ok {
			dec = int(math.Max(0, math.Min(10, d)))
		}
		return groupNumber(f, dec)
	case "date":
		t, dateOnly, ok := toTime(v)
		if !ok {
			return v
		}
		locale := ""
		var loc *time.Location
		if env != nil {
			locale, loc = env.Locale, env.Loc
		}
		if !dateOnly && loc != nil {
			t = t.In(loc)
		}
		f, _ := arg(0).(string)
		if f == "" {
			f = "D MMM YYYY"
		}
		return FormatDateTokens(t, f, locale)
	}
	return v
}

// groupNumber formats f with thousands separators; dec < 0 = up to 10
// decimals without trailing zeros.
func groupNumber(f float64, dec int) string {
	var s string
	if dec < 0 {
		s = FormatNumber(f)
	} else {
		s = strconv.FormatFloat(f, 'f', dec, 64)
	}
	if s == "" {
		return ""
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if hasFrac {
		out += "." + frac
	}
	if neg && strings.Trim(out, "0.,") != "" {
		out = "-" + out
	}
	return out
}

var dateLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"}

// toTime reads a date value: ISO strings (no offset = UTC), "YYYY-MM-DD" (a
// calendar date, never shifted by the timezone) or a Unix time in seconds.
func toTime(v any) (time.Time, bool, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, false, true
	case string:
		s := strings.TrimSpace(x)
		if len(s) == 10 {
			if t, err := time.Parse("2006-01-02", s); err == nil {
				return t, true, true
			}
		}
		for _, l := range dateLayouts {
			if t, err := time.Parse(l, s); err == nil {
				return t, false, true
			}
		}
		return time.Time{}, false, false
	}
	if f, ok := toNum(v); ok {
		return time.Unix(int64(f), 0).UTC(), false, true
	}
	return time.Time{}, false, false
}

var (
	monthsEN      = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthsShortEN = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	monthsTH      = []string{"มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน", "กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"}
	monthsShortTH = []string{"ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
	dateTokens    = []string{"YYYY", "BBBB", "MMMM", "MMM", "MM", "M", "DD", "D", "YY", "HH", "H", "mm", "m"}
)

// FormatDateTokens formats t with D DD M MM MMM MMMM YY YYYY BBBB (Buddhist
// year) H HH m mm; text in [brackets] is kept literally. locale "th" uses
// Thai month names.
func FormatDateTokens(t time.Time, f, locale string) string {
	long, short := monthsEN, monthsShortEN
	if locale == "th" {
		long, short = monthsTH, monthsShortTH
	}
	two := func(n int) string {
		if n < 10 {
			return "0" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	var b strings.Builder
	for i := 0; i < len(f); {
		if f[i] == '[' {
			if j := strings.IndexByte(f[i:], ']'); j > 0 {
				b.WriteString(f[i+1 : i+j])
				i += j + 1
				continue
			}
		}
		tok := ""
		for _, k := range dateTokens {
			if strings.HasPrefix(f[i:], k) {
				tok = k
				break
			}
		}
		switch tok {
		case "YYYY":
			b.WriteString(strconv.Itoa(t.Year()))
		case "BBBB":
			b.WriteString(strconv.Itoa(t.Year() + 543))
		case "YY":
			b.WriteString(two(t.Year() % 100))
		case "MMMM":
			b.WriteString(long[t.Month()-1])
		case "MMM":
			b.WriteString(short[t.Month()-1])
		case "MM":
			b.WriteString(two(int(t.Month())))
		case "M":
			b.WriteString(strconv.Itoa(int(t.Month())))
		case "DD":
			b.WriteString(two(t.Day()))
		case "D":
			b.WriteString(strconv.Itoa(t.Day()))
		case "HH":
			b.WriteString(two(t.Hour()))
		case "H":
			b.WriteString(strconv.Itoa(t.Hour()))
		case "mm":
			b.WriteString(two(t.Minute()))
		case "m":
			b.WriteString(strconv.Itoa(t.Minute()))
		default:
			_, size := utf8.DecodeRuneInString(f[i:])
			b.WriteString(f[i : i+size])
			i += size
			continue
		}
		i += len(tok)
	}
	return b.String()
}

// ---------------------------------------------------------------- templates

// Template is a string with {{ expr }} parts.
type Template struct {
	parts  []tplPart
	single bool // exactly one expression and no text around it
}

type tplPart struct {
	text string
	expr *Expr
}

var (
	tplCache   sync.Map // string → *Template
	tplCacheN  int
	tplCacheMu sync.Mutex
)

const tplCacheMax = 20000

// ParseTemplate splits s into text and expressions. An expression that does
// not parse stays as literal text (the editor shows the error).
func ParseTemplate(s string) *Template {
	if t, ok := tplCache.Load(s); ok {
		return t.(*Template)
	}
	t := parseTemplate(s)
	tplCacheMu.Lock()
	if tplCacheN < tplCacheMax {
		tplCacheN++
		tplCache.Store(s, t)
	}
	tplCacheMu.Unlock()
	return t
}

func parseTemplate(s string) *Template {
	t := &Template{}
	lit := func(x string) {
		if x == "" {
			return
		}
		if n := len(t.parts); n > 0 && t.parts[n-1].expr == nil {
			t.parts[n-1].text += x
			return
		}
		t.parts = append(t.parts, tplPart{text: x})
	}
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			lit(s)
			break
		}
		lit(s[:i])
		end := closeBraces(s, i+2)
		if end < 0 {
			lit(s[i:])
			break
		}
		raw := s[i : end+2]
		e, err := ParseExpr(s[i+2 : end])
		if err != nil {
			lit(raw)
		} else {
			t.parts = append(t.parts, tplPart{expr: e})
		}
		s = s[end+2:]
	}
	t.single = len(t.parts) == 1 && t.parts[0].expr != nil
	return t
}

// closeBraces finds the "}}" closing an expression that starts at i,
// skipping quoted strings; -1 when there is none.
func closeBraces(s string, i int) int {
	var q byte
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case q != 0:
			if c == '\\' {
				i++
			} else if c == q {
				q = 0
			}
		case c == '\'' || c == '"':
			q = c
		case c == '}' && i+1 < len(s) && s[i+1] == '}':
			return i
		}
	}
	return -1
}

// HasExpr reports whether the template contains an expression.
func (t *Template) HasExpr() bool {
	for _, p := range t.parts {
		if p.expr != nil {
			return true
		}
	}
	return false
}

// Render evaluates the template to a string; esc (optional) is applied to
// every evaluated value (HTML templates).
func (t *Template) Render(env *Env, esc func(string) string) string {
	if len(t.parts) == 1 && t.parts[0].expr == nil {
		return t.parts[0].text
	}
	var b strings.Builder
	for _, p := range t.parts {
		if p.expr == nil {
			b.WriteString(p.text)
			continue
		}
		s := Stringify(p.expr.Eval(env))
		if esc != nil {
			s = esc(s)
		}
		b.WriteString(s)
	}
	return b.String()
}

// Value evaluates a template that is a single expression to its raw value
// (numbers stay numbers); otherwise the rendered string.
func (t *Template) Value(env *Env) any {
	if t.single {
		return t.parts[0].expr.Eval(env)
	}
	return t.Render(env, nil)
}

// EvalString evaluates s as a bare expression ("item.price * 2"), or as a
// template when it contains {{ }}; a bare expression that does not parse is
// returned as the literal text.
func EvalString(s string, env *Env) any {
	if strings.Contains(s, "{{") {
		return ParseTemplate(s).Value(env)
	}
	if strings.TrimSpace(s) == "" {
		return nil
	}
	e, err := cachedExpr(s)
	if err != nil {
		return s
	}
	return e.Eval(env)
}

var exprCache sync.Map // string → exprEntry

type exprEntry struct {
	e   *Expr
	err error
}

func cachedExpr(s string) (*Expr, error) {
	if v, ok := exprCache.Load(s); ok {
		en := v.(exprEntry)
		return en.e, en.err
	}
	e, err := ParseExpr(s)
	tplCacheMu.Lock()
	if tplCacheN < tplCacheMax {
		tplCacheN++
		exprCache.Store(s, exprEntry{e, err})
	}
	tplCacheMu.Unlock()
	return e, err
}

// Condition evaluates a showWhen expression: true when empty or when it does
// not parse (the layer is shown and the problem logged; the editor flags it).
func Condition(s string, env *Env) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") && closeBraces(s, 2) == len(s)-2 {
		s = s[2 : len(s)-2]
	}
	e, err := cachedExpr(s)
	if err != nil {
		badCondition(s, err)
		return true
	}
	return Truthy(e.Eval(env))
}

var loggedConds sync.Map

// badCondition logs a showWhen that does not parse (once per expression).
func badCondition(s string, err error) {
	if _, seen := loggedConds.LoadOrStore(s, true); !seen {
		log.Printf("design: showWhen %q does not parse: %v (layer shown)", s, err)
	}
}

// visitorIs evaluates visitor.is.<group>[.more]: true/false by membership of
// <group> in visitor.groups (false without a visitor); keys after the group
// read members of that boolean (always null).
func visitorIs(visitor any, keys []string) any {
	g := keys[0]
	if blockedKeys[g] {
		return nil
	}
	in := false
	{
		var groups any
		switch m := visitor.(type) {
		case map[string]any:
			groups = m["groups"]
		}
		switch l := groups.(type) {
		case []any:
			for _, x := range l {
				if s, ok := x.(string); ok && s == g {
					in = true
				}
			}
		case []string:
			for _, s := range l {
				if s == g {
					in = true
				}
			}
		}
	}
	if len(keys) > 1 {
		return nil
	}
	return in
}
