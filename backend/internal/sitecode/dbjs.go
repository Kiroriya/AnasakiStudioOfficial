package sitecode

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/go-sql-driver/mysql"
)

// db.table / db.from / db.raw / db.transaction / db.query / db.docs inside
// handlers. Calls are synchronous; each statement gets min(5 s, the
// handler's remaining time) as a context deadline, SELECTs also a
// MAX_EXECUTION_TIME hint.

// runner is a *sql.DB or *sql.Tx.
type runner interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// fail throws a plain JS Error.
func (v *vm) fail(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	ctor, _ := goja.AssertConstructor(v.rt.Get("Error"))
	if ctor != nil {
		if obj, err := ctor(nil, v.rt.ToValue(msg)); err == nil {
			panic(obj)
		}
	}
	v.throw("%s", msg)
}

// dbErr turns a driver error into a script error.
func (v *vm) dbErr(ctx context.Context, err error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		v.fail("db: หมดเวลา query (สูงสุด 5 วินาที และไม่เกินเวลาของ handler)")
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		if me.Number == 3024 { // MAX_EXECUTION_TIME exceeded
			v.fail("db: หมดเวลา query (สูงสุด 5 วินาที)")
		}
		v.fail("db: %s", me.Message)
	}
	var tm *ErrTooMany
	if errors.As(err, &tm) {
		v.fail("db: %s", tm.Msg)
	}
	v.fail("db: %s", err.Error())
}

// stmtCtx is the context of one statement and the hint in ms.
func (v *vm) stmtCtx() (context.Context, context.CancelFunc, int) {
	d := stmtTimeout
	if !v.deadline.IsZero() {
		rem := time.Until(v.deadline)
		if rem <= 10*time.Millisecond {
			v.fail("db: หมดเวลาของ handler แล้ว")
		}
		if rem < d {
			d = rem
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	return ctx, cancel, int(d / time.Millisecond)
}

func (v *vm) data() *Data {
	v.handlerOnly("db")
	v.need("db", "db")
	if v.h == nil || v.h.data == nil {
		v.throw("db ไม่พร้อมใช้งาน")
	}
	return v.h.data
}

// naming is the table naming of the VM's unit.
func (v *vm) naming(d *Data) Naming {
	if v.u.plugin() {
		return v.u.n
	}
	return d.N
}

// docsTable is the kv_docs table of the VM's unit.
func (v *vm) docsTable() string {
	d := v.data()
	if v.u.plugin() {
		return v.u.n.Prefix + "kv_docs"
	}
	return d.DocsTable()
}

// rawTableRe finds table references in a db.raw fragment of a plugin.
var rawTableRe = regexp.MustCompile(`(?i)\b(select|from|join|into|update|table|union)\b|;`)

// rawCommentRe finds SQL comments in a db.raw fragment of a plugin: MySQL
// runs the body of a versioned comment (/*!50000SELECT*/), which hides a
// keyword from rawTableRe's word boundaries (audit L-1), so a plugin's
// fragment may not contain comments at all (/* */, -- or #).
var rawCommentRe = regexp.MustCompile(`/\*|\*/|--|#`)

// pluginRawAllowed reports whether a plugin may use fragment s in db.raw:
// no table references and no comments.
func pluginRawAllowed(s string) bool {
	return !rawCommentRe.MatchString(s) && !rawTableRe.MatchString(s)
}

// installDB adds the phase-2 functions to the db global.
func (v *vm) installDB(db *goja.Object) {
	v.fillDBObject(db, nil)
	_ = db.Set("transaction", v.dbTransaction)
}

// fillDBObject sets table/from/raw/query/docs bound to r (nil = pool).
func (v *vm) fillDBObject(o *goja.Object, tx *sql.Tx) {
	run := func() runner {
		if tx != nil {
			return tx
		}
		return v.data().code
	}
	table := func(c goja.FunctionCall) goja.Value {
		d := v.data()
		name, ok := c.Argument(0).Export().(string)
		if !ok {
			v.throw("db.table: ต้องระบุชื่อตาราง เช่น db.table('orders')")
		}
		q, err := NewQuery(v.naming(d), name)
		if err != nil {
			v.throw("%s", err.Error())
		}
		return v.builder(q, run())
	}
	_ = o.Set("table", table)
	_ = o.Set("from", table)
	_ = o.Set("raw", func(c goja.FunctionCall) goja.Value {
		d := v.data()
		if d.N.Fallback {
			v.throw("db.raw ใช้ไม่ได้เมื่อยังไม่ได้ตั้งฐานข้อมูลแยก (CODE_DB_DSN)")
		}
		if v.u.plugin() && !pluginRawAllowed(c.Argument(0).String()) {
			v.throw("db.raw ในปลั๊กอินอ้างถึงตารางหรือใส่ comment ไม่ได้ (SELECT/FROM/JOIN…, /* */, --, #) — ใช้ query builder")
		}
		s, ok := c.Argument(0).Export().(string)
		if !ok || strings.TrimSpace(s) == "" {
			v.throw("db.raw: ต้องเป็นข้อความ SQL")
		}
		var args []any
		if p := c.Argument(1); !goja.IsUndefined(p) && !goja.IsNull(p) {
			args = v.valueList(p, "db.raw")
		}
		if strings.Count(s, "?") != len(args) {
			v.throw("db.raw: จำนวน ? (%d) ไม่ตรงกับจำนวนค่า (%d)", strings.Count(s, "?"), len(args))
		}
		return v.rt.ToValue(&Expr{SQL: s, Args: args})
	})
	_ = o.Set("query", func(c goja.FunctionCall) goja.Value {
		d := v.data()
		if v.u.plugin() {
			v.throw("db.query (SQL ตรง) ใช้ไม่ได้ในปลั๊กอิน — ใช้ query builder (db.table)")
		}
		if !d.Separate() {
			v.throw("db.query ใช้ไม่ได้เมื่อยังไม่ได้ตั้งฐานข้อมูลแยก (CODE_DB_DSN) — ใช้ query builder แทน")
		}
		s, ok := c.Argument(0).Export().(string)
		if !ok || strings.TrimSpace(s) == "" {
			v.throw("db.query: ต้องเป็นข้อความ SQL")
		}
		var args []any
		if p := c.Argument(1); !goja.IsUndefined(p) && !goja.IsNull(p) {
			args = v.valueList(p, "db.query")
		}
		return v.rawQuery(run(), s, args)
	})
	_ = o.Set("docs", func(c goja.FunctionCall) goja.Value {
		v.data()
		name, _ := c.Argument(0).Export().(string)
		if !identRe.MatchString(name) {
			v.throw("db.docs: ชื่อชุดข้อมูลต้องเป็น a-z 0-9 _ (ขึ้นต้นด้วยตัวอักษร)")
		}
		return v.docs(run(), name)
	})
}

// ---------------------------------------------------------------- values

func (v *vm) array(x goja.Value) ([]goja.Value, bool) {
	o, ok := x.(*goja.Object)
	if !ok || o.ClassName() != "Array" {
		return nil, false
	}
	n := int(o.Get("length").ToInteger())
	out := make([]goja.Value, n)
	for i := 0; i < n; i++ {
		out[i] = o.Get(strconv.Itoa(i))
	}
	return out, true
}

// bindVal converts a JS value to a bound parameter.
func (v *vm) bindVal(x goja.Value, what string) any {
	if x == nil || goja.IsUndefined(x) {
		v.throw("%s: ค่าเป็น undefined", what)
	}
	if goja.IsNull(x) {
		return nil
	}
	switch e := x.Export().(type) {
	case *Expr:
		return e
	case string:
		return BindValue(e)
	case bool, int64:
		return e
	case float64:
		if e == math.Trunc(e) && math.Abs(e) < 1<<53 {
			return int64(e)
		}
		if math.IsNaN(e) || math.IsInf(e, 0) {
			v.throw("%s: ตัวเลขไม่ถูกต้อง", what)
		}
		return e
	case time.Time:
		return e.UTC()
	case *big.Int:
		return e.String()
	}
	s, ok, err := v.toJSON(x)
	if err != nil || !ok {
		v.throw("%s: ค่าแปลงเป็น JSON ไม่ได้", what)
	}
	return s
}

func (v *vm) valueList(x goja.Value, what string) []any {
	items, ok := v.array(x)
	if !ok {
		v.throw("%s: ต้องเป็น array", what)
	}
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = v.bindVal(it, what)
	}
	return out
}

// kvs reads an object's own keys in order (undefined values skipped).
func (v *vm) kvs(x goja.Value, what string) []KV {
	o, ok := x.(*goja.Object)
	if !ok || goja.IsNull(x) {
		v.throw("%s: ต้องเป็น object", what)
	}
	var out []KV
	for _, k := range o.Keys() {
		val := o.Get(k)
		if goja.IsUndefined(val) {
			continue
		}
		out = append(out, KV{Key: k, Val: v.bindVal(val, what+"."+k)})
	}
	return out
}

func (v *vm) colArg(x goja.Value, what string) any {
	switch e := x.Export().(type) {
	case string:
		return e
	case *Expr:
		return e
	}
	v.throw("%s: ต้องเป็นชื่อคอลัมน์หรือ db.raw", what)
	return nil
}

// rowJS builds a JS object in column order.
func (v *vm) rowJS(r Row) goja.Value {
	o := v.rt.NewObject()
	for i, c := range r.Cols {
		_ = o.Set(c, v.valJS(r.Vals[i]))
	}
	return o
}

func (v *vm) valJS(x any) goja.Value {
	switch e := x.(type) {
	case nil:
		return goja.Null()
	case JSONText:
		out, err := v.parse(goja.Undefined(), v.rt.ToValue(string(e)))
		if err != nil {
			return v.rt.ToValue(string(e))
		}
		return out
	}
	return v.rt.ToValue(x)
}

func (v *vm) rowsJS(rows []Row) goja.Value {
	arr := make([]any, len(rows))
	for i, r := range rows {
		arr[i] = v.rowJS(r)
	}
	return v.rt.NewArray(arr...)
}

// ---------------------------------------------------------------- running

func (v *vm) selectRows(r runner, q string, args []any) []Row {
	ctx, cancel, _ := v.stmtCtx()
	defer cancel()
	rows, err := r.QueryContext(ctx, q, args...)
	if err != nil {
		v.dbErr(ctx, err)
	}
	_, out, _, err := ReadRows(rows, MaxResultRows, false)
	if err != nil {
		v.dbErr(ctx, err)
	}
	return out
}

func (v *vm) exec(r runner, q string, args []any) sql.Result {
	ctx, cancel, _ := v.stmtCtx()
	defer cancel()
	res, err := r.ExecContext(ctx, q, args...)
	if err != nil {
		v.dbErr(ctx, err)
	}
	if v.h != nil && v.h.e != nil {
		v.h.e.bumpGen(v.u.id()) // cache fingerprints (sitemap.urls, blocks)
	}
	return res
}

func hintMs(v *vm) int {
	d := stmtTimeout
	if !v.deadline.IsZero() {
		if rem := time.Until(v.deadline); rem < d {
			d = rem
		}
	}
	if d < time.Millisecond {
		return 1
	}
	return int(d / time.Millisecond)
}

var readSQLRe = regexp.MustCompile(`(?is)^\s*(select|with|show|describe|desc|explain|table|values)\b`)

// rawQuery runs db.query: rows for a SELECT, {affectedRows, insertId} else.
func (v *vm) rawQuery(r runner, s string, args []any) goja.Value {
	s = strings.TrimRight(strings.TrimSpace(s), "; \n\t")
	if strings.Count(s, "?") < len(args) {
		v.throw("db.query: จำนวนค่ามากกว่าจำนวน ?")
	}
	if readSQLRe.MatchString(s) {
		if strings.HasPrefix(strings.ToUpper(s), "SELECT") && !strings.Contains(s, "/*+") {
			s = s[:6] + " " + hint(hintMs(v)) + strings.TrimSpace(s[6:])
		}
		return v.rowsJS(v.selectRows(r, s, args))
	}
	res := v.exec(r, s, args)
	n, _ := res.RowsAffected()
	id, _ := res.LastInsertId()
	return v.toJS(map[string]any{"affectedRows": n, "insertId": id})
}

// ---------------------------------------------------------------- transactions

func (v *vm) dbTransaction(c goja.FunctionCall) goja.Value {
	d := v.data()
	fn, ok := goja.AssertFunction(c.Argument(0))
	if !ok {
		v.throw("db.transaction: ต้องส่งฟังก์ชัน (tx) => { ... }")
	}
	if v.inTx {
		v.throw("db.transaction ซ้อนกันไม่ได้ — ใช้ tx ที่ได้รับ")
	}
	limit := 30 * time.Second
	if !v.deadline.IsZero() {
		limit = time.Until(v.deadline)
		if limit <= 10*time.Millisecond {
			v.fail("db: หมดเวลาของ handler แล้ว")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	tx, err := d.code.BeginTx(ctx, nil)
	if err != nil {
		v.dbErr(ctx, err)
	}
	done := false
	v.inTx = true
	defer func() {
		v.inTx = false
		if !done {
			_ = tx.Rollback()
		}
	}()
	txObj := v.rt.NewObject()
	v.fillDBObject(txObj, tx)
	res, err := fn(goja.Undefined(), txObj)
	if err != nil {
		panic(err) // rolled back by the deferred function; rethrown to the script
	}
	if res != nil {
		if _, isPromise := res.Export().(*goja.Promise); isPromise {
			v.throw("db.transaction: ฟังก์ชันต้องทำงานแบบ synchronous (ไม่ใช่ async)")
		}
	}
	if err := tx.Commit(); err != nil {
		v.dbErr(ctx, err)
	}
	done = true
	if res == nil {
		return goja.Undefined()
	}
	return res
}

// ---------------------------------------------------------------- builder

// builder wraps a Query as a chainable JS object.
func (v *vm) builder(q *Query, r runner) *goja.Object {
	o := v.rt.NewObject()
	self := func(fn func(c goja.FunctionCall)) func(goja.FunctionCall) goja.Value {
		return func(c goja.FunctionCall) goja.Value {
			fn(c)
			return o
		}
	}
	check := func(err error) {
		if err != nil {
			v.throw("%s", err.Error())
		}
	}
	set := func(name string, fn func(goja.FunctionCall) goja.Value) { _ = o.Set(name, fn) }

	set("select", self(func(c goja.FunctionCall) {
		var items []any
		for _, a := range c.Arguments {
			if list, ok := v.array(a); ok {
				for _, it := range list {
					items = append(items, v.colArg(it, "select"))
				}
				continue
			}
			items = append(items, v.colArg(a, "select"))
		}
		check(q.Select(items...))
	}))
	set("distinct", self(func(goja.FunctionCall) { q.Distinct() }))
	// phase 7: aggregate columns valid in both database modes (no db.raw)
	selAgg := func(fn string, countStyle bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) {
			var col any = "*"
			alias := ""
			if countStyle { // selectCount(alias?, column?)
				if s, ok := c.Argument(0).Export().(string); ok {
					alias = s
				}
				if a := c.Argument(1); !goja.IsUndefined(a) && !goja.IsNull(a) {
					col = v.colArg(a, "select"+fn)
				}
			} else { // selectSum(column, alias?)
				col = v.colArg(c.Argument(0), "select"+fn)
				if s, ok := c.Argument(1).Export().(string); ok {
					alias = s
				}
			}
			check(q.SelectAggregate(fn, col, alias))
		})
	}
	set("selectCount", selAgg("COUNT", true))
	set("selectSum", selAgg("SUM", false))
	set("selectAvg", selAgg("AVG", false))
	set("selectMin", selAgg("MIN", false))
	set("selectMax", selAgg("MAX", false))
	set("selectCountDistinct", self(func(c goja.FunctionCall) {
		alias, _ := c.Argument(1).Export().(string)
		check(q.SelectAggregate("COUNT DISTINCT", v.colArg(c.Argument(0), "selectCountDistinct"), alias))
	}))
	join := func(kind string) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) {
			t, _ := c.Argument(0).Export().(string)
			var on []cond
			switch len(c.Arguments) {
			case 3, 4:
				a, op, b := c.Argument(1).String(), "=", c.Argument(2).String()
				if len(c.Arguments) == 4 {
					op, b = c.Argument(2).String(), c.Argument(3).String()
				}
				cd, err := OnCols(a, op, b)
				check(err)
				on = append(on, cd)
			default:
				v.throw("join(table, a, b) หรือ join(table, a, op, b)")
			}
			check(q.Join(kind, t, on))
		})
	}
	set("join", join("INNER"))
	set("innerJoin", join("INNER"))
	set("leftJoin", join("LEFT"))
	set("rightJoin", join("RIGHT"))

	where := func(or bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) {
			v.applyWhere(q, r, or, c.Arguments, false)
		})
	}
	set("where", where(false))
	set("orWhere", where(true))
	in := func(or, not bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) {
			check(q.WhereIn(or, v.colArg(c.Argument(0), "whereIn"), v.valueList(c.Argument(1), "whereIn"), not))
		})
	}
	set("whereIn", in(false, false))
	set("whereNotIn", in(false, true))
	set("orWhereIn", in(true, false))
	set("orWhereNotIn", in(true, true))
	null := func(or, not bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) { check(q.WhereNull(or, v.colArg(c.Argument(0), "whereNull"), not)) })
	}
	set("whereNull", null(false, false))
	set("whereNotNull", null(false, true))
	set("orWhereNull", null(true, false))
	set("orWhereNotNull", null(true, true))
	between := func(or, not bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) {
			list := v.valueList(c.Argument(1), "whereBetween")
			if len(list) != 2 {
				v.throw("whereBetween: ต้องเป็น [ค่าเริ่ม, ค่าสิ้นสุด]")
			}
			check(q.WhereBetween(or, v.colArg(c.Argument(0), "whereBetween"), list[0], list[1], not))
		})
	}
	set("whereBetween", between(false, false))
	set("whereNotBetween", between(false, true))
	set("orWhereBetween", between(true, false))
	like := func(or bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) {
			cd, err := Compare(v.colArg(c.Argument(0), "whereLike"), "like", v.bindVal(c.Argument(1), "whereLike"))
			check(err)
			q.Where(or, cd)
		})
	}
	set("whereLike", like(false))
	set("orWhereLike", like(true))
	set("groupBy", self(func(c goja.FunctionCall) {
		var items []any
		for _, a := range c.Arguments {
			items = append(items, v.colArg(a, "groupBy"))
		}
		check(q.GroupBy(items...))
	}))
	having := func(or bool) func(goja.FunctionCall) goja.Value {
		return self(func(c goja.FunctionCall) { v.applyWhere(q, r, or, c.Arguments, true) })
	}
	set("having", having(false))
	set("orHaving", having(true))
	set("orderBy", self(func(c goja.FunctionCall) {
		dir := ""
		if d := c.Argument(1); !goja.IsUndefined(d) {
			dir = d.String()
		}
		check(q.OrderBy(v.colArg(c.Argument(0), "orderBy"), dir))
	}))
	set("orderByDesc", self(func(c goja.FunctionCall) { check(q.OrderBy(v.colArg(c.Argument(0), "orderByDesc"), "desc")) }))
	set("latest", self(func(c goja.FunctionCall) {
		col := any("created_at")
		if !goja.IsUndefined(c.Argument(0)) {
			col = v.colArg(c.Argument(0), "latest")
		}
		check(q.OrderBy(col, "desc"))
	}))
	set("oldest", self(func(c goja.FunctionCall) {
		col := any("created_at")
		if !goja.IsUndefined(c.Argument(0)) {
			col = v.colArg(c.Argument(0), "oldest")
		}
		check(q.OrderBy(col, "asc"))
	}))
	num := func(c goja.FunctionCall, what string) int {
		x := c.Argument(0)
		if _, ok := x.Export().(int64); !ok {
			if f, ok := x.Export().(float64); !ok || f != math.Trunc(f) {
				v.throw("%s: ต้องเป็นจำนวนเต็ม", what)
			}
		}
		return int(x.ToInteger())
	}
	set("limit", self(func(c goja.FunctionCall) { check(q.Limit(num(c, "limit"))) }))
	set("take", self(func(c goja.FunctionCall) { check(q.Limit(num(c, "take"))) }))
	set("offset", self(func(c goja.FunctionCall) { check(q.Offset(num(c, "offset"))) }))
	set("skip", self(func(c goja.FunctionCall) { check(q.Offset(num(c, "skip"))) }))
	set("all", self(func(goja.FunctionCall) { q.All() }))

	// ---- terminal operations
	set("get", func(goja.FunctionCall) goja.Value {
		s, args := q.SelectSQL(hintMs(v))
		return v.rowsJS(v.selectRows(r, s, args))
	})
	set("first", func(goja.FunctionCall) goja.Value {
		c := *q
		c.limit = 1
		s, args := c.SelectSQL(hintMs(v))
		rows := v.selectRows(r, s, args)
		if len(rows) == 0 {
			return goja.Null()
		}
		return v.rowJS(rows[0])
	})
	set("pluck", func(c goja.FunctionCall) goja.Value {
		col, _ := c.Argument(0).Export().(string)
		cq := *q
		cq.cols = nil
		check(cq.Select(col))
		s, args := cq.SelectSQL(hintMs(v))
		rows := v.selectRows(r, s, args)
		out := make([]any, len(rows))
		for i, row := range rows {
			out[i] = v.valJS(row.Vals[0])
		}
		return v.rt.NewArray(out...)
	})
	set("paginate", func(c goja.FunctionCall) goja.Value {
		page, per := 1, 20
		if a := c.Argument(0); !goja.IsUndefined(a) && !goja.IsNull(a) {
			page = int(a.ToInteger())
		}
		if a := c.Argument(1); !goja.IsUndefined(a) && !goja.IsNull(a) {
			per = int(a.ToInteger())
		}
		if page < 1 || page > 1000000 {
			v.throw("paginate: page ต้องเป็นจำนวนเต็มตั้งแต่ 1")
		}
		if per < 1 || per > 100 {
			v.throw("paginate: perPage ต้องอยู่ระหว่าง 1 ถึง 100")
		}
		cs, cargs := q.CountSQL(hintMs(v))
		total := v.selectRows(r, cs, cargs)
		s, args := q.PageSQL(hintMs(v), page, per)
		items := v.rowsJS(v.selectRows(r, s, args))
		res := v.rt.NewObject()
		_ = res.Set("items", items)
		_ = res.Set("total", v.valJS(total[0].Vals[0]))
		_ = res.Set("page", page)
		_ = res.Set("perPage", per)
		return res
	})
	set("count", func(goja.FunctionCall) goja.Value {
		s, args := q.CountSQL(hintMs(v))
		return v.valJS(v.selectRows(r, s, args)[0].Vals[0])
	})
	set("exists", func(goja.FunctionCall) goja.Value {
		c := *q
		c.limit = 1
		s, args := c.SelectSQL(hintMs(v))
		return v.rt.ToValue(len(v.selectRows(r, s, args)) > 0)
	})
	agg := func(fn string) func(goja.FunctionCall) goja.Value {
		return func(c goja.FunctionCall) goja.Value {
			s, args, err := q.AggregateSQL(hintMs(v), fn, v.colArg(c.Argument(0), strings.ToLower(fn)))
			check(err)
			val := v.selectRows(r, s, args)[0].Vals[0]
			if fn == "SUM" || fn == "AVG" {
				if str, ok := val.(string); ok {
					if f, err := strconv.ParseFloat(str, 64); err == nil {
						return v.rt.ToValue(f)
					}
				}
			}
			return v.valJS(val)
		}
	}
	set("sum", agg("SUM"))
	set("avg", agg("AVG"))
	set("min", agg("MIN"))
	set("max", agg("MAX"))
	set("insert", func(c goja.FunctionCall) goja.Value {
		var rows [][]KV
		if list, ok := v.array(c.Argument(0)); ok {
			if len(list) == 0 {
				v.throw("insert: array ว่าง")
			}
			if len(list) > 1000 {
				v.throw("insert: ได้ครั้งละไม่เกิน 1000 แถว")
			}
			for _, it := range list {
				rows = append(rows, v.kvs(it, "insert"))
			}
		} else {
			rows = append(rows, v.kvs(c.Argument(0), "insert"))
		}
		s, args, err := q.InsertSQL(rows)
		check(err)
		res := v.exec(r, s, args)
		id, _ := res.LastInsertId()
		return v.rt.ToValue(id)
	})
	set("update", func(c goja.FunctionCall) goja.Value {
		s, args, err := q.UpdateSQL(v.kvs(c.Argument(0), "update"))
		check(err)
		n, _ := v.exec(r, s, args).RowsAffected()
		return v.rt.ToValue(n)
	})
	set("delete", func(goja.FunctionCall) goja.Value {
		s, args, err := q.DeleteSQL()
		check(err)
		n, _ := v.exec(r, s, args).RowsAffected()
		return v.rt.ToValue(n)
	})
	step := func(sign float64, what string) func(goja.FunctionCall) goja.Value {
		return func(c goja.FunctionCall) goja.Value {
			col, ok := c.Argument(0).Export().(string)
			if !ok {
				v.throw("%s: ต้องระบุชื่อคอลัมน์", what)
			}
			n := 1.0
			if a := c.Argument(1); !goja.IsUndefined(a) && !goja.IsNull(a) {
				n = a.ToFloat()
			}
			var extra []KV
			if a := c.Argument(2); !goja.IsUndefined(a) && !goja.IsNull(a) {
				extra = v.kvs(a, what)
			}
			s, args, err := q.IncrementSQL(col, sign*n, extra)
			check(err)
			cnt, _ := v.exec(r, s, args).RowsAffected()
			return v.rt.ToValue(cnt)
		}
	}
	set("increment", step(1, "increment"))
	set("decrement", step(-1, "decrement"))
	set("toSQL", func(goja.FunctionCall) goja.Value {
		s, args := q.SelectSQL(0)
		return v.toJS(map[string]any{"sql": s, "bindings": args})
	})
	return o
}

// applyWhere handles where/orWhere/having argument shapes.
func (v *vm) applyWhere(q *Query, r runner, or bool, args []goja.Value, having bool) {
	add := func(c cond) {
		if having {
			q.Having(or, c)
		} else {
			q.Where(or, c)
		}
	}
	check := func(err error) {
		if err != nil {
			v.throw("%s", err.Error())
		}
	}
	if len(args) == 0 {
		v.throw("where: ต้องมีเงื่อนไข")
	}
	first := args[0]
	if len(args) == 1 {
		if fn, ok := goja.AssertFunction(first); ok && !having {
			sub := q.SubQuery()
			if _, err := fn(goja.Undefined(), v.builder(sub, r)); err != nil {
				panic(err)
			}
			q.WhereGroup(or, sub)
			return
		}
		if e, ok := first.Export().(*Expr); ok {
			add(cond{sql: "(" + e.SQL + ")", args: e.Args})
			return
		}
		if o, ok := first.(*goja.Object); ok && o.ClassName() == "Object" {
			var cs []cond
			for _, kv := range v.kvs(first, "where") {
				c, err := Compare(kv.Key, "=", kv.Val)
				check(err)
				cs = append(cs, c)
			}
			if len(cs) == 0 {
				return
			}
			s, a := condSQL(cs)
			add(cond{sql: "(" + s + ")", args: a})
			return
		}
		v.throw("where: ใช้ where(col, value), where(col, op, value), where({...}), where(q => ...) หรือ where(db.raw(...))")
	}
	col := v.colArg(first, "where")
	op, val := "=", args[1]
	if len(args) >= 3 {
		op, val = args[1].String(), args[2]
	}
	var bv any
	if list, ok := v.array(val); ok {
		vals := make([]any, len(list))
		for i, it := range list {
			vals[i] = v.bindVal(it, "where")
		}
		bv = vals
	} else {
		bv = v.bindVal(val, "where")
	}
	c, err := Compare(col, op, bv)
	check(err)
	add(c)
}

// ---------------------------------------------------------------- docs

var docKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

func docPath(k string) string {
	parts := strings.Split(k, ".")
	for i, p := range parts {
		parts[i] = `"` + p + `"`
	}
	return "$." + strings.Join(parts, ".")
}

func (v *vm) docID(x goja.Value) int64 {
	switch e := x.Export().(type) {
	case int64:
		return e
	case float64:
		return int64(e)
	case string:
		n, err := strconv.ParseInt(e, 10, 64)
		if err == nil {
			return n
		}
	}
	v.throw("db.docs: id ต้องเป็นตัวเลข")
	return 0
}

func (v *vm) docJSON(x goja.Value, what string) string {
	o, ok := x.(*goja.Object)
	if !ok || o.ClassName() != "Object" {
		v.throw("%s: ต้องเป็น object", what)
	}
	s, ok, err := v.toJSON(x)
	if err != nil || !ok {
		v.throw("%s: แปลงเป็น JSON ไม่ได้", what)
	}
	return s
}

func (v *vm) docValue(id int64, data string) goja.Value {
	val, err := v.parse(goja.Undefined(), v.rt.ToValue(data))
	if err != nil {
		val = v.rt.NewObject()
	}
	o := val.ToObject(v.rt)
	_ = o.Set("id", id)
	return o
}

func (v *vm) docs(r runner, coll string) goja.Value {
	tbl := quoteIdent(v.docsTable())
	o := v.rt.NewObject()
	_ = o.Set("insert", func(c goja.FunctionCall) goja.Value {
		data := v.docJSON(c.Argument(0), "db.docs.insert")
		res := v.exec(r, "INSERT INTO "+tbl+" (collection, data) VALUES (?, ?)", []any{coll, data})
		id, _ := res.LastInsertId()
		return v.rt.ToValue(id)
	})
	_ = o.Set("get", func(c goja.FunctionCall) goja.Value {
		id := v.docID(c.Argument(0))
		rows := v.selectRows(r, "SELECT "+hint(hintMs(v))+"id, data FROM "+tbl+" WHERE collection = ? AND id = ?", []any{coll, id})
		if len(rows) == 0 {
			return goja.Null()
		}
		return v.docValue(id, string(rows[0].Vals[1].(JSONText)))
	})
	_ = o.Set("update", func(c goja.FunctionCall) goja.Value {
		id := v.docID(c.Argument(0))
		patch := v.docJSON(c.Argument(1), "db.docs.update")
		n, _ := v.exec(r, "UPDATE "+tbl+" SET data = JSON_MERGE_PATCH(data, CAST(? AS JSON)) WHERE collection = ? AND id = ?",
			[]any{patch, coll, id}).RowsAffected()
		if n == 0 { // unchanged values report 0 rows: check existence
			rows := v.selectRows(r, "SELECT id FROM "+tbl+" WHERE collection = ? AND id = ?", []any{coll, id})
			return v.rt.ToValue(len(rows) > 0)
		}
		return v.rt.ToValue(true)
	})
	_ = o.Set("delete", func(c goja.FunctionCall) goja.Value {
		id := v.docID(c.Argument(0))
		n, _ := v.exec(r, "DELETE FROM "+tbl+" WHERE collection = ? AND id = ?", []any{coll, id}).RowsAffected()
		return v.rt.ToValue(n > 0)
	})
	_ = o.Set("find", func(c goja.FunctionCall) goja.Value {
		opts := optObject(v.rt, c.Argument(0))
		where := " WHERE collection = ?"
		args := []any{coll}
		if opts != nil {
			if w := opts.Get("where"); w != nil && !goja.IsUndefined(w) && !goja.IsNull(w) {
				wo := w.ToObject(v.rt)
				for _, k := range wo.Keys() {
					if !docKeyRe.MatchString(k) {
						v.throw("db.docs.find: ชื่อฟิลด์ %q ไม่ถูกต้อง", k)
					}
					val := wo.Get(k)
					js, ok, err := v.toJSON(val)
					if err != nil || !ok {
						v.throw("db.docs.find: ค่าของ %s แปลงเป็น JSON ไม่ได้", k)
					}
					where += " AND JSON_EXTRACT(data, ?) = CAST(? AS JSON)"
					args = append(args, docPath(k), js)
				}
			}
		}
		order := " ORDER BY id ASC"
		var oargs []any
		if ob := optString(opts, "orderBy"); ob != "" {
			field, dir := strings.TrimSpace(ob), "ASC"
			if strings.HasPrefix(field, "-") {
				field, dir = field[1:], "DESC"
			}
			if f, d, ok := strings.Cut(field, " "); ok {
				field = f
				switch strings.ToLower(strings.TrimSpace(d)) {
				case "desc":
					dir = "DESC"
				case "asc":
				default:
					v.throw("db.docs.find: orderBy เช่น 'name', '-name' หรือ 'name desc'")
				}
			}
			switch field {
			case "id":
				order = " ORDER BY id " + dir
			case "createdAt":
				order = " ORDER BY created_at " + dir + ", id " + dir
			case "updatedAt":
				order = " ORDER BY updated_at " + dir + ", id " + dir
			default:
				if !docKeyRe.MatchString(field) {
					v.throw("db.docs.find: ชื่อฟิลด์ %q ไม่ถูกต้อง", field)
				}
				order = " ORDER BY JSON_EXTRACT(data, ?) " + dir + ", id " + dir
				oargs = []any{docPath(field)}
			}
		}
		page, per := 1, 20
		if s := optString(opts, "page"); s != "" {
			page, _ = strconv.Atoi(s)
		}
		if s := optString(opts, "perPage"); s != "" {
			per, _ = strconv.Atoi(s)
		}
		if page < 1 || per < 1 || per > 100 {
			v.throw("db.docs.find: page ≥ 1 และ perPage 1-100")
		}
		total := v.selectRows(r, "SELECT "+hint(hintMs(v))+"COUNT(*) FROM "+tbl+where, args)
		rows := v.selectRows(r, "SELECT "+hint(hintMs(v))+"id, data FROM "+tbl+where+order+fmt.Sprintf(" LIMIT %d OFFSET %d", per, (page-1)*per),
			append(append([]any{}, args...), oargs...))
		items := make([]any, len(rows))
		for i, row := range rows {
			id, _ := row.Vals[0].(int64)
			items[i] = v.docValue(id, string(row.Vals[1].(JSONText)))
		}
		res := v.rt.NewObject()
		_ = res.Set("items", v.rt.NewArray(items...))
		_ = res.Set("total", v.valJS(total[0].Vals[0]))
		_ = res.Set("page", page)
		_ = res.Set("perPage", per)
		return res
	})
	return o
}

// jsonOf is a helper for tests.
func jsonOf(x any) string { b, _ := json.Marshal(x); return string(b) }
