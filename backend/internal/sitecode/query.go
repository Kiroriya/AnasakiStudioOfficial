package sitecode

import (
	"fmt"
	"regexp"
	"strings"
)

// Query builder: db.table(...) chains build a Query; every value is a bound
// parameter and every identifier is validated, anything else must go
// through db.raw (an Expr).

// Expr is a raw SQL fragment with its parameters (db.raw).
type Expr struct {
	SQL  string
	Args []any
}

var (
	refIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	asRe       = regexp.MustCompile(`(?i)^\s*(\S+)\s+as\s+(\S+)\s*$`)
)

// Naming maps logical table names to physical ones (fallback: sc_ prefix;
// a JS plugin: p_<id>_, fallback sc_p_<id>_).
type Naming struct {
	Prefix   string
	Fallback bool
	// Unit is the JS plugin the names belong to ("" = Site Code) and NoCore
	// refuses the core_* views (a plugin without the "content" scope).
	Unit   string
	NoCore bool
}

// tableRef is a FROM / JOIN table.
type tableRef struct{ name, alias string }

// ParseTable validates "orders" or "orders as o".
func (n Naming) ParseTable(s string) (tableRef, error) {
	name, alias := strings.TrimSpace(s), ""
	if m := asRe.FindStringSubmatch(s); m != nil {
		name, alias = m[1], m[2]
	}
	if !refIdentRe.MatchString(name) || (alias != "" && !refIdentRe.MatchString(alias)) {
		return tableRef{}, fmt.Errorf("ชื่อตาราง %q ไม่ถูกต้อง (ใช้ \"orders\" หรือ \"orders as o\")", s)
	}
	name = strings.ToLower(name)
	if err := n.readable(name); err != nil {
		return tableRef{}, err
	}
	return tableRef{name: name, alias: alias}, nil
}

// readable checks a table scripts may query.
func (n Naming) readable(name string) error {
	if strings.HasPrefix(name, "script_") || strings.HasPrefix(name, "bak_") || strings.HasPrefix(name, "sc_") {
		return fmt.Errorf("ตาราง %s สงวนไว้โดยระบบ", name)
	}
	if strings.HasPrefix(name, "p_") {
		return fmt.Errorf("ตาราง %s เป็นของปลั๊กอินอื่น — ใช้ได้ผ่าน export / hook ของปลั๊กอินนั้นเท่านั้น", name)
	}
	if strings.HasPrefix(name, "core_") {
		if n.NoCore {
			return fmt.Errorf("ไม่มีสิทธิ์ใช้ content (view %s)", name)
		}
		if _, ok := coreViews[name]; !ok {
			return fmt.Errorf("ไม่มี view %s (มี core_posts, core_users, core_terms, core_post_terms)", name)
		}
		if n.Fallback {
			return fmt.Errorf("%s ใช้ไม่ได้เมื่อยังไม่ได้ตั้งฐานข้อมูลแยก (CODE_DB_DSN) — ใช้ db.posts แทน", name)
		}
	}
	return nil
}

// Phys is the physical name of a logical table.
func (n Naming) Phys(name string) string {
	if strings.HasPrefix(name, "core_") {
		return name
	}
	return n.Prefix + name
}

// sql renders a table reference; in fallback mode the logical name stays
// usable as a qualifier through an alias.
func (n Naming) tableSQL(t tableRef) string {
	s := quoteIdent(n.Phys(t.name))
	switch {
	case t.alias != "":
		s += " AS " + quoteIdent(t.alias)
	case n.Phys(t.name) != t.name:
		s += " AS " + quoteIdent(t.name)
	}
	return s
}

// ColumnRef validates and quotes "col", "t.col", "t.*" or "*".
func ColumnRef(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "*" {
		return "*", nil
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return "", fmt.Errorf("ชื่อคอลัมน์ %q ไม่ถูกต้อง (ใช้ \"col\" หรือ \"table.col\" หรือ db.raw)", s)
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		if p == "*" && i == len(parts)-1 && len(parts) == 2 {
			out[i] = "*"
			continue
		}
		if !refIdentRe.MatchString(p) {
			return "", fmt.Errorf("ชื่อคอลัมน์ %q ไม่ถูกต้อง (ใช้ \"col\" หรือ \"table.col\" หรือ db.raw)", s)
		}
		out[i] = quoteIdent(p)
	}
	return strings.Join(out, "."), nil
}

// selectRef also accepts "col as alias".
func selectRef(s string) (string, error) {
	if m := asRe.FindStringSubmatch(s); m != nil {
		c, err := ColumnRef(m[1])
		if err != nil {
			return "", err
		}
		if !refIdentRe.MatchString(m[2]) {
			return "", fmt.Errorf("ชื่อแทน (alias) %q ไม่ถูกต้อง", m[2])
		}
		return c + " AS " + quoteIdent(m[2]), nil
	}
	return ColumnRef(s)
}

// plainColumn validates a column name for insert/update.
func plainColumn(s string) (string, error) {
	if !refIdentRe.MatchString(s) {
		return "", fmt.Errorf("ชื่อคอลัมน์ %q ไม่ถูกต้อง", s)
	}
	return quoteIdent(s), nil
}

// cond is one WHERE / HAVING / ON condition.
type cond struct {
	or   bool
	sql  string
	args []any
}

type joinClause struct {
	kind  string
	table tableRef
	on    []cond
}

// Query is a db.table(...) chain.
type Query struct {
	N        Naming
	from     tableRef
	cols     []Expr
	distinct bool
	joins    []joinClause
	wheres   []cond
	groups   []Expr
	havings  []cond
	orders   []Expr
	limit    int
	offset   int
	all      bool
}

// NewQuery starts a query on table ("orders" or "orders as o").
func NewQuery(n Naming, table string) (*Query, error) {
	t, err := n.ParseTable(table)
	if err != nil {
		return nil, err
	}
	return &Query{N: n, from: t, limit: -1, offset: -1}, nil
}

// SubQuery is a condition group (orWhere(q => ...)).
func (q *Query) SubQuery() *Query { return &Query{N: q.N, limit: -1, offset: -1} }

// Select adds columns ("col", "t.col as x") or raw expressions.
func (q *Query) Select(items ...any) error {
	for _, it := range items {
		switch x := it.(type) {
		case string:
			s, err := selectRef(x)
			if err != nil {
				return err
			}
			q.cols = append(q.cols, Expr{SQL: s})
		case *Expr:
			q.cols = append(q.cols, *x)
		default:
			return fmt.Errorf("select(): ใช้ชื่อคอลัมน์หรือ db.raw")
		}
	}
	return nil
}

// SelectAggregate adds COUNT / SUM / AVG / MIN / MAX (or "COUNT DISTINCT")
// of col ("*" for COUNT) as alias (default: the function name in lower
// case). SUM and AVG are returned as numbers (DOUBLE), not DECIMAL strings.
func (q *Query) SelectAggregate(fn string, col any, alias string) error {
	if alias == "" {
		alias = strings.ToLower(strings.ReplaceAll(fn, " DISTINCT", ""))
	}
	if !refIdentRe.MatchString(alias) {
		return fmt.Errorf("ชื่อแทน (alias) %q ไม่ถูกต้อง", alias)
	}
	c, cargs, err := colOrExpr(col)
	if err != nil {
		return err
	}
	var sql string
	switch fn {
	case "COUNT":
		sql = "COUNT(" + c + ")"
	case "COUNT DISTINCT":
		if c == "*" {
			return fmt.Errorf("selectCountDistinct(): ต้องระบุคอลัมน์")
		}
		sql = "COUNT(DISTINCT " + c + ")"
	case "SUM", "AVG":
		if c == "*" {
			return fmt.Errorf("select%s(): ต้องระบุคอลัมน์", fn[:1]+strings.ToLower(fn[1:]))
		}
		sql = "(" + fn + "(" + c + ") + 0e0)"
	case "MIN", "MAX":
		if c == "*" {
			return fmt.Errorf("select%s(): ต้องระบุคอลัมน์", fn[:1]+strings.ToLower(fn[1:]))
		}
		sql = fn + "(" + c + ")"
	default:
		return fmt.Errorf("aggregate %q ไม่รองรับ", fn)
	}
	q.cols = append(q.cols, Expr{SQL: sql + " AS " + quoteIdent(alias), Args: cargs})
	return nil
}

// Distinct makes the select DISTINCT.
func (q *Query) Distinct() { q.distinct = true }

var operators = map[string]string{"=": "=", "!=": "<>", "<>": "<>", "<": "<", "<=": "<=", ">": ">", ">=": ">=",
	"like": "LIKE", "not like": "NOT LIKE", "in": "IN", "not in": "NOT IN"}

// colOrExpr renders a column reference or a raw expression.
func colOrExpr(c any) (string, []any, error) {
	switch x := c.(type) {
	case string:
		s, err := ColumnRef(x)
		return s, nil, err
	case *Expr:
		return x.SQL, x.Args, nil
	}
	return "", nil, fmt.Errorf("ต้องเป็นชื่อคอลัมน์หรือ db.raw")
}

// Compare builds "col op ?" (null → IS NULL, arrays with in / not in).
func Compare(col any, op string, val any) (cond, error) {
	c, cargs, err := colOrExpr(col)
	if err != nil {
		return cond{}, err
	}
	o, ok := operators[strings.ToLower(strings.TrimSpace(op))]
	if !ok {
		return cond{}, fmt.Errorf("operator %q ไม่รองรับ (=, !=, <, <=, >, >=, like, not like, in, not in)", op)
	}
	if o == "IN" || o == "NOT IN" {
		list, ok := val.([]any)
		if !ok {
			return cond{}, fmt.Errorf("operator %s ต้องใช้ array", op)
		}
		return inCond(c, cargs, list, o == "NOT IN"), nil
	}
	if e, ok := val.(*Expr); ok {
		return cond{sql: c + " " + o + " " + e.SQL, args: append(cargs, e.Args...)}, nil
	}
	if val == nil {
		switch o {
		case "=":
			return cond{sql: c + " IS NULL", args: cargs}, nil
		case "<>":
			return cond{sql: c + " IS NOT NULL", args: cargs}, nil
		}
		return cond{}, fmt.Errorf("เทียบ null ได้เฉพาะ = และ != (หรือใช้ whereNull)")
	}
	if _, isList := val.([]any); isList {
		return cond{}, fmt.Errorf("ค่าที่เป็น array ใช้กับ whereIn หรือ operator 'in'")
	}
	return cond{sql: c + " " + o + " ?", args: append(cargs, val)}, nil
}

func inCond(c string, cargs []any, list []any, not bool) cond {
	if len(list) == 0 {
		if not {
			return cond{sql: "1 = 1"}
		}
		return cond{sql: "1 = 0"}
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(list)), ", ")
	kw := " IN ("
	if not {
		kw = " NOT IN ("
	}
	return cond{sql: c + kw + ph + ")", args: append(cargs, list...)}
}

// Where adds an AND (or OR) condition.
func (q *Query) Where(or bool, c cond) { c.or = or; q.wheres = append(q.wheres, c) }

// Having adds a HAVING condition.
func (q *Query) Having(or bool, c cond) { c.or = or; q.havings = append(q.havings, c) }

// WhereGroup nests a sub-query's conditions in parentheses.
func (q *Query) WhereGroup(or bool, sub *Query) {
	if len(sub.wheres) == 0 {
		return
	}
	s, args := condSQL(sub.wheres)
	q.wheres = append(q.wheres, cond{or: or, sql: "(" + s + ")", args: args})
}

// WhereIn adds col IN (...).
func (q *Query) WhereIn(or bool, col any, list []any, not bool) error {
	c, cargs, err := colOrExpr(col)
	if err != nil {
		return err
	}
	q.Where(or, inCond(c, cargs, list, not))
	return nil
}

// WhereNull adds col IS [NOT] NULL.
func (q *Query) WhereNull(or bool, col any, not bool) error {
	c, cargs, err := colOrExpr(col)
	if err != nil {
		return err
	}
	s := c + " IS NULL"
	if not {
		s = c + " IS NOT NULL"
	}
	q.Where(or, cond{sql: s, args: cargs})
	return nil
}

// WhereBetween adds col [NOT] BETWEEN ? AND ?.
func (q *Query) WhereBetween(or bool, col any, a, b any, not bool) error {
	c, cargs, err := colOrExpr(col)
	if err != nil {
		return err
	}
	kw := " BETWEEN ? AND ?"
	if not {
		kw = " NOT BETWEEN ? AND ?"
	}
	q.Where(or, cond{sql: c + kw, args: append(cargs, a, b)})
	return nil
}

// WhereRaw adds a raw condition.
func (q *Query) WhereRaw(or bool, e *Expr) { q.Where(or, cond{sql: "(" + e.SQL + ")", args: e.Args}) }

func condSQL(cs []cond) (string, []any) {
	var b strings.Builder
	var args []any
	for i, c := range cs {
		if i > 0 {
			if c.or {
				b.WriteString(" OR ")
			} else {
				b.WriteString(" AND ")
			}
		}
		b.WriteString(c.sql)
		args = append(args, c.args...)
	}
	return b.String(), args
}

// Join adds [LEFT|RIGHT] JOIN table ON a op b.
func (q *Query) Join(kind, table string, on []cond) error {
	t, err := q.N.ParseTable(table)
	if err != nil {
		return err
	}
	if len(on) == 0 {
		return fmt.Errorf("join(%q): ต้องมีเงื่อนไข ON", table)
	}
	q.joins = append(q.joins, joinClause{kind: kind, table: t, on: on})
	return nil
}

// OnCols builds "a op b" between two columns.
func OnCols(a, op, b string) (cond, error) {
	ca, err := ColumnRef(a)
	if err != nil {
		return cond{}, err
	}
	cb, err := ColumnRef(b)
	if err != nil {
		return cond{}, err
	}
	o, ok := operators[strings.ToLower(op)]
	if !ok || o == "IN" || o == "NOT IN" || o == "LIKE" || o == "NOT LIKE" {
		return cond{}, fmt.Errorf("join: operator %q ไม่รองรับ", op)
	}
	return cond{sql: ca + " " + o + " " + cb}, nil
}

// GroupBy adds GROUP BY items.
func (q *Query) GroupBy(items ...any) error {
	for _, it := range items {
		c, args, err := colOrExpr(it)
		if err != nil {
			return err
		}
		q.groups = append(q.groups, Expr{SQL: c, Args: args})
	}
	return nil
}

// OrderBy adds ORDER BY col [asc|desc].
func (q *Query) OrderBy(col any, dir string) error {
	c, args, err := colOrExpr(col)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "", "asc":
		c += " ASC"
	case "desc":
		c += " DESC"
	default:
		return fmt.Errorf("orderBy: ทิศทางต้องเป็น 'asc' หรือ 'desc'")
	}
	q.orders = append(q.orders, Expr{SQL: c, Args: args})
	return nil
}

// Limit / Offset.
func (q *Query) Limit(n int) error {
	if n < 0 {
		return fmt.Errorf("limit ต้องไม่ติดลบ")
	}
	q.limit = n
	return nil
}

// Offset sets OFFSET.
func (q *Query) Offset(n int) error {
	if n < 0 {
		return fmt.Errorf("offset ต้องไม่ติดลบ")
	}
	q.offset = n
	return nil
}

// All allows update/delete without where.
func (q *Query) All() { q.all = true }

// fromSQL is FROM ... JOIN ... WHERE ... GROUP BY ... HAVING ...
func (q *Query) fromSQL() (string, []any) {
	var b strings.Builder
	var args []any
	b.WriteString(" FROM " + q.N.tableSQL(q.from))
	for _, j := range q.joins {
		s, a := condSQL(j.on)
		b.WriteString(" " + j.kind + " JOIN " + q.N.tableSQL(j.table) + " ON " + s)
		args = append(args, a...)
	}
	if len(q.wheres) > 0 {
		s, a := condSQL(q.wheres)
		b.WriteString(" WHERE " + s)
		args = append(args, a...)
	}
	if len(q.groups) > 0 {
		parts := make([]string, len(q.groups))
		for i, g := range q.groups {
			parts[i] = g.SQL
			args = append(args, g.Args...)
		}
		b.WriteString(" GROUP BY " + strings.Join(parts, ", "))
	}
	if len(q.havings) > 0 {
		s, a := condSQL(q.havings)
		b.WriteString(" HAVING " + s)
		args = append(args, a...)
	}
	return b.String(), args
}

func hint(ms int) string {
	if ms <= 0 {
		return ""
	}
	return fmt.Sprintf("/*+ MAX_EXECUTION_TIME(%d) */ ", ms)
}

// SelectSQL renders the SELECT (hintMs > 0 adds MAX_EXECUTION_TIME).
func (q *Query) SelectSQL(hintMs int) (string, []any) {
	var b strings.Builder
	var args []any
	b.WriteString("SELECT " + hint(hintMs))
	if q.distinct {
		b.WriteString("DISTINCT ")
	}
	if len(q.cols) == 0 {
		b.WriteString("*")
	} else {
		parts := make([]string, len(q.cols))
		for i, c := range q.cols {
			parts[i] = c.SQL
			args = append(args, c.Args...)
		}
		b.WriteString(strings.Join(parts, ", "))
	}
	f, fa := q.fromSQL()
	b.WriteString(f)
	args = append(args, fa...)
	if len(q.orders) > 0 {
		parts := make([]string, len(q.orders))
		for i, o := range q.orders {
			parts[i] = o.SQL
			args = append(args, o.Args...)
		}
		b.WriteString(" ORDER BY " + strings.Join(parts, ", "))
	}
	switch {
	case q.limit >= 0 && q.offset >= 0:
		b.WriteString(fmt.Sprintf(" LIMIT %d OFFSET %d", q.limit, q.offset))
	case q.limit >= 0:
		b.WriteString(fmt.Sprintf(" LIMIT %d", q.limit))
	case q.offset >= 0:
		b.WriteString(fmt.Sprintf(" LIMIT 18446744073709551615 OFFSET %d", q.offset))
	}
	return b.String(), args
}

// PageSQL renders the page query of paginate(page, perPage).
func (q *Query) PageSQL(hintMs, page, perPage int) (string, []any) {
	c := *q
	c.limit, c.offset = perPage, (page-1)*perPage
	return c.SelectSQL(hintMs)
}

// CountSQL counts the rows the query would return (ignores order/limit).
func (q *Query) CountSQL(hintMs int) (string, []any) {
	if len(q.groups) > 0 || q.distinct {
		c := *q
		c.orders, c.limit, c.offset = nil, -1, -1
		inner, args := c.SelectSQL(0)
		return "SELECT " + hint(hintMs) + "COUNT(*) AS aggregate FROM (" + inner + ") AS t", args
	}
	f, args := q.fromSQL()
	return "SELECT " + hint(hintMs) + "COUNT(*) AS aggregate" + f, args
}

// AggregateSQL renders SUM/AVG/MIN/MAX(col).
func (q *Query) AggregateSQL(hintMs int, fn string, col any) (string, []any, error) {
	c, cargs, err := colOrExpr(col)
	if err != nil {
		return "", nil, err
	}
	if c == "*" {
		return "", nil, fmt.Errorf("%s(): ต้องระบุคอลัมน์", strings.ToLower(fn))
	}
	if len(q.groups) > 0 { // aggregate over the grouped rows: the column of the inner select
		if i := strings.LastIndex(c, "`.`"); i >= 0 {
			c = "`" + c[i+3:]
		}
		inner, args := q.SelectSQL(0)
		return "SELECT " + hint(hintMs) + fn + "(t0." + c + ") AS aggregate FROM (" + inner + ") AS t0", append(cargs, args...), nil
	}
	f, args := q.fromSQL()
	return "SELECT " + hint(hintMs) + fn + "(" + c + ") AS aggregate" + f, append(cargs, args...), nil
}

// writable checks the query targets a developer table without joins.
func (q *Query) writable(op string) error {
	if strings.HasPrefix(q.from.name, "core_") {
		return fmt.Errorf("%s: %s เป็นข้อมูลของระบบหลัก อ่านได้อย่างเดียว", op, q.from.name)
	}
	if len(q.joins) > 0 || len(q.groups) > 0 || len(q.havings) > 0 {
		return fmt.Errorf("%s: ใช้ร่วมกับ join/groupBy/having ไม่ได้", op)
	}
	return nil
}

// KV is an ordered column → value pair.
type KV struct {
	Key string
	Val any
}

// InsertSQL renders a (multi-row) INSERT; missing keys use DEFAULT.
func (q *Query) InsertSQL(rows [][]KV) (string, []any, error) {
	if err := q.writable("insert"); err != nil {
		return "", nil, err
	}
	if len(rows) == 0 {
		return "", nil, fmt.Errorf("insert: ไม่มีข้อมูล")
	}
	var cols []string
	seen := map[string]bool{}
	for _, r := range rows {
		for _, kv := range r {
			if !seen[kv.Key] {
				if _, err := plainColumn(kv.Key); err != nil {
					return "", nil, err
				}
				seen[kv.Key] = true
				cols = append(cols, kv.Key)
			}
		}
	}
	var b strings.Builder
	var args []any
	qc := make([]string, len(cols))
	for i, c := range cols {
		qc[i] = quoteIdent(c)
	}
	b.WriteString("INSERT INTO " + quoteIdent(q.N.Phys(q.from.name)))
	if len(cols) == 0 {
		b.WriteString(" () VALUES ")
	} else {
		b.WriteString(" (" + strings.Join(qc, ", ") + ") VALUES ")
	}
	for i, r := range rows {
		if i > 0 {
			b.WriteString(", ")
		}
		m := map[string]any{}
		for _, kv := range r {
			m[kv.Key] = kv.Val
		}
		ph := make([]string, len(cols))
		for j, c := range cols {
			v, ok := m[c]
			switch {
			case !ok:
				ph[j] = "DEFAULT"
			default:
				if e, isExpr := v.(*Expr); isExpr {
					ph[j] = e.SQL
					args = append(args, e.Args...)
				} else {
					ph[j] = "?"
					args = append(args, v)
				}
			}
		}
		b.WriteString("(" + strings.Join(ph, ", ") + ")")
	}
	return b.String(), args, nil
}

func (q *Query) guardAll(op string) error {
	if err := q.writable(op); err != nil {
		return err
	}
	if len(q.wheres) == 0 && !q.all {
		return fmt.Errorf("%s ทั้งตารางโดยไม่มี where ไม่ได้ — ใส่ .where(...) หรือเรียก .all() ถ้าตั้งใจ", op)
	}
	return nil
}

func (q *Query) whereTail() (string, []any) {
	var b strings.Builder
	var args []any
	if len(q.wheres) > 0 {
		s, a := condSQL(q.wheres)
		b.WriteString(" WHERE " + s)
		args = a
	}
	if len(q.orders) > 0 {
		parts := make([]string, len(q.orders))
		for i, o := range q.orders {
			parts[i] = o.SQL
			args = append(args, o.Args...)
		}
		b.WriteString(" ORDER BY " + strings.Join(parts, ", "))
	}
	if q.limit >= 0 {
		b.WriteString(fmt.Sprintf(" LIMIT %d", q.limit))
	}
	return b.String(), args
}

// UpdateSQL renders UPDATE ... SET ... WHERE.
func (q *Query) UpdateSQL(fields []KV) (string, []any, error) {
	if err := q.guardAll("update"); err != nil {
		return "", nil, err
	}
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("update: ไม่มีคอลัมน์ที่จะแก้")
	}
	var sets []string
	var args []any
	for _, kv := range fields {
		c, err := plainColumn(kv.Key)
		if err != nil {
			return "", nil, err
		}
		if e, ok := kv.Val.(*Expr); ok {
			sets = append(sets, c+" = "+e.SQL)
			args = append(args, e.Args...)
		} else {
			sets = append(sets, c+" = ?")
			args = append(args, kv.Val)
		}
	}
	tail, ta := q.whereTail()
	return "UPDATE " + q.N.tableSQL(q.from) + " SET " + strings.Join(sets, ", ") + tail, append(args, ta...), nil
}

// IncrementSQL renders col = col + n.
func (q *Query) IncrementSQL(col string, n float64, extra []KV) (string, []any, error) {
	c, err := plainColumn(col)
	if err != nil {
		return "", nil, err
	}
	fields := append([]KV{{Key: col, Val: &Expr{SQL: c + " + ?", Args: []any{n}}}}, extra...)
	return q.UpdateSQL(fields)
}

// DeleteSQL renders DELETE ... WHERE.
func (q *Query) DeleteSQL() (string, []any, error) {
	if err := q.guardAll("delete"); err != nil {
		return "", nil, err
	}
	tail, args := q.whereTail()
	return "DELETE FROM " + q.N.tableSQL(q.from) + tail, args, nil
}
