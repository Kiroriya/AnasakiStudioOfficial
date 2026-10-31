package sitecode

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Admin screens of the code database (GET/POST /api/v1/code/db/...). They
// run as the CMS user on the code database, except the SQL console, which
// runs as the code user.

// Admin errors.
var (
	ErrReadOnly    = errors.New("read only")
	ErrFallbackSQL = errors.New("fallback")
	ErrNoPK        = errors.New("no primary key")
)

// BadInput is a 400 with a message.
type BadInput struct{ Msg string }

func (b *BadInput) Error() string { return b.Msg }

func bad(format string, a ...any) error { return &BadInput{Msg: fmt.Sprintf(format, a...)} }

// DBInfo is GET /db/status.
type DBInfo struct {
	Mode       string   `json:"mode"`
	Database   string   `json:"database"`
	Configured bool     `json:"configured"`
	SetupSQL   []string `json:"setupSql,omitempty"`
	SetupDSN   string   `json:"setupDsn,omitempty"`
	Error      string   `json:"error,omitempty"`
	SizeBytes  int64    `json:"sizeBytes"`
	Tables     int      `json:"tables"`
}

// TableInfo is an entry of GET /db/tables.
type TableInfo struct {
	Name      string `json:"name"`
	Rows      int64  `json:"rows"`
	SizeBytes int64  `json:"sizeBytes"`
	Comment   string `json:"comment"`
	View      bool   `json:"view"`
	System    bool   `json:"system"`
}

// tableKind resolves a logical name.
type tableRefInfo struct {
	Logical, Phys string
	View, System  bool
}

func (d *Data) objects(ctx context.Context) ([]TableInfo, map[string]tableRefInfo, error) {
	rows, err := d.admin.QueryContext(ctx, `SELECT TABLE_NAME, TABLE_TYPE, COALESCE(DATA_LENGTH,0) + COALESCE(INDEX_LENGTH,0), COALESCE(TABLE_COMMENT,'')
		FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME`, d.Database)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var list []TableInfo
	refs := map[string]tableRefInfo{}
	for rows.Next() {
		var name, typ, comment string
		var size int64
		if err := rows.Scan(&name, &typ, &size, &comment); err != nil {
			return nil, nil, err
		}
		view := typ == "VIEW"
		var l string
		if view {
			if _, ok := coreViews[name]; !ok || !d.Separate() {
				continue
			}
			l = name
		} else {
			var ok bool
			if l, ok = d.logical(name); !ok || strings.HasPrefix(l, "bak_") || strings.HasPrefix(l, "script_") || strings.HasPrefix(l, "core_") {
				continue
			}
			if d.Separate() && strings.HasPrefix(name, "script_") {
				continue
			}
		}
		ti := TableInfo{Name: l, SizeBytes: size, Comment: comment, View: view, System: l == "kv_docs"}
		list = append(list, ti)
		refs[l] = tableRefInfo{Logical: l, Phys: name, View: view, System: ti.System}
	}
	return list, refs, rows.Err()
}

func (d *Data) resolve(ctx context.Context, name string) (tableRefInfo, error) {
	if !refIdentRe.MatchString(name) {
		return tableRefInfo{}, ErrNotFound
	}
	_, refs, err := d.objects(ctx)
	if err != nil {
		return tableRefInfo{}, err
	}
	r, ok := refs[name]
	if !ok {
		return tableRefInfo{}, ErrNotFound
	}
	return r, nil
}

// Info is GET /db/status.
func (d *Data) Info(ctx context.Context) (*DBInfo, error) {
	info := &DBInfo{Mode: d.Mode, Database: d.Database, Configured: d.Configured, Error: d.Err}
	if d.Mode == ModeFallback {
		info.SetupSQL, info.SetupDSN = d.SetupSQL(), d.SetupDSN()
	}
	list, _, err := d.objects(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		if !t.View {
			info.Tables++
			info.SizeBytes += t.SizeBytes
		}
	}
	return info, nil
}

// Tables is GET /db/tables (exact row counts).
func (d *Data) Tables(ctx context.Context) ([]TableInfo, error) {
	list, refs, err := d.objects(ctx)
	if err != nil {
		return nil, err
	}
	out := []TableInfo{}
	for _, t := range list {
		var n int64
		if err := d.admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdent(refs[t.Name].Phys)).Scan(&n); err == nil {
			t.Rows = n
		}
		out = append(out, t)
	}
	return out, nil
}

// TableDetail is GET /db/tables/{t}.
type TableDetail struct {
	Name        string     `json:"name"`
	View        bool       `json:"view"`
	System      bool       `json:"system"`
	PrimaryKey  *string    `json:"primaryKey"`
	Columns     []*SColumn `json:"columns"`
	Indexes     []*SIndex  `json:"indexes"`
	ForeignKeys []*SFK     `json:"foreignKeys"`
	Relations   []*SRel    `json:"relations"`
}

func (d *Data) columns(ctx context.Context, phys string) ([]*SColumn, error) {
	rows, err := d.admin.QueryContext(ctx, `SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_KEY, EXTRA, COLUMN_COMMENT
		FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`, d.Database, phys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*SColumn{}
	for rows.Next() {
		c := &SColumn{}
		var nullable string
		var def sql.NullString
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &def, &c.Key, &c.Extra, &c.Comment); err != nil {
			return nil, err
		}
		c.Type = strings.ToLower(c.Type)
		c.Nullable = nullable == "YES"
		if def.Valid {
			s := def.String
			c.Default = &s
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func primaryKey(cols []*SColumn) *string {
	var pk []string
	for _, c := range cols {
		if c.Key == "PRI" {
			pk = append(pk, c.Name)
		}
	}
	if len(pk) != 1 {
		return nil
	}
	return &pk[0]
}

// Describe is GET /db/tables/{t}.
func (d *Data) Describe(ctx context.Context, name string) (*TableDetail, error) {
	ref, err := d.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	cols, err := d.columns(ctx, ref.Phys)
	if err != nil {
		return nil, err
	}
	out := &TableDetail{Name: ref.Logical, View: ref.View, System: ref.System, Columns: cols, PrimaryKey: primaryKey(cols),
		Indexes: []*SIndex{}, ForeignKeys: []*SFK{}, Relations: []*SRel{}}
	if ref.View || ref.System {
		return out, nil
	}
	snap, err := d.LoadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if t := snap.Tables[ref.Logical]; t != nil {
		if t.Indexes != nil {
			out.Indexes = t.Indexes
		}
		if t.FKs != nil {
			out.ForeignKeys = t.FKs
		}
		if t.Relations != nil {
			out.Relations = t.Relations
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- rows

// RowsQuery is GET /db/tables/{t}/rows.
type RowsQuery struct {
	Page, PerPage int
	Q, Sort, Dir  string
	Filters       map[string]string
}

// RowsPage is the rows response.
type RowsPage struct {
	Items      []Row   `json:"items"`
	Total      int64   `json:"total"`
	PrimaryKey *string `json:"primaryKey"`
}

func isTextType(t string) bool {
	for _, p := range []string{"char", "varchar", "text", "tinytext", "mediumtext", "longtext", "enum", "set"} {
		if strings.HasPrefix(t, p+"(") || t == p {
			return true
		}
	}
	return false
}

func colByName(cols []*SColumn, name string) *SColumn {
	for _, c := range cols {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// whereFor builds the search / filter condition.
func whereFor(cols []*SColumn, q string, filters map[string]string) (string, []any, error) {
	var parts []string
	var args []any
	if q = strings.TrimSpace(q); q != "" {
		var or []string
		for _, c := range cols {
			if isTextType(c.Type) || c.Type == "json" {
				or = append(or, "CAST("+quoteIdent(c.Name)+" AS CHAR) LIKE ?")
				args = append(args, "%"+likeEscape(q)+"%")
			} else if _, err := strconv.ParseFloat(q, 64); err == nil && !strings.Contains(c.Type, "date") && !strings.Contains(c.Type, "time") {
				or = append(or, quoteIdent(c.Name)+" = ?")
				args = append(args, q)
			}
		}
		if len(or) == 0 {
			or = []string{"1 = 0"}
		}
		parts = append(parts, "("+strings.Join(or, " OR ")+")")
	}
	for k, v := range filters {
		c := colByName(cols, k)
		if c == nil {
			return "", nil, bad("ไม่มีคอลัมน์ %s", k)
		}
		if isTextType(c.Type) || c.Type == "json" {
			parts = append(parts, "CAST("+quoteIdent(c.Name)+" AS CHAR) LIKE ?")
			args = append(args, "%"+likeEscape(v)+"%")
		} else {
			parts = append(parts, quoteIdent(c.Name)+" = ?")
			args = append(args, v)
		}
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

// Rows lists a page of rows.
func (d *Data) Rows(ctx context.Context, name string, rq RowsQuery) (*RowsPage, error) {
	ref, err := d.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	cols, err := d.columns(ctx, ref.Phys)
	if err != nil {
		return nil, err
	}
	where, args, err := whereFor(cols, rq.Q, rq.Filters)
	if err != nil {
		return nil, err
	}
	out := &RowsPage{PrimaryKey: primaryKey(cols), Items: []Row{}}
	from := " FROM " + quoteIdent(ref.Phys)
	if err := d.admin.QueryRowContext(ctx, "SELECT COUNT(*)"+from+where, args...).Scan(&out.Total); err != nil {
		return nil, err
	}
	order := ""
	switch {
	case rq.Sort != "":
		if colByName(cols, rq.Sort) == nil {
			return nil, bad("ไม่มีคอลัมน์ %s", rq.Sort)
		}
		order = " ORDER BY " + quoteIdent(rq.Sort)
		if strings.EqualFold(rq.Dir, "desc") {
			order += " DESC"
		}
	case out.PrimaryKey != nil:
		order = " ORDER BY " + quoteIdent(*out.PrimaryKey)
	}
	rows, err := d.admin.QueryContext(ctx, "SELECT *"+from+where+order+fmt.Sprintf(" LIMIT %d OFFSET %d", rq.PerPage, (rq.Page-1)*rq.PerPage), args...)
	if err != nil {
		return nil, err
	}
	_, items, _, err := ReadRows(rows, rq.PerPage+1, true)
	if err != nil {
		return nil, err
	}
	out.Items = items
	return out, nil
}

// normalize converts a JSON / CSV value for column c.
func normalize(c *SColumn, v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case json.Number:
		return string(x), nil
	case float64:
		return x, nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case map[string]any, []any:
		b, err := json.Marshal(x)
		return string(b), err
	case string:
		t := c.Type
		switch {
		case strings.HasPrefix(t, "datetime") || strings.HasPrefix(t, "timestamp") || t == "date":
			if tm, err := time.Parse(time.RFC3339Nano, x); err == nil {
				if t == "date" {
					return tm.UTC().Format("2006-01-02"), nil
				}
				return tm.UTC().Format("2006-01-02 15:04:05.000000"), nil
			}
		case strings.HasPrefix(t, "tinyint"):
			switch strings.ToLower(x) {
			case "true":
				return 1, nil
			case "false":
				return 0, nil
			}
		}
		return x, nil
	}
	return nil, fmt.Errorf("ค่าของ %s ไม่รองรับ", c.Name)
}

func (d *Data) writable(ctx context.Context, name string) (tableRefInfo, []*SColumn, error) {
	ref, err := d.resolve(ctx, name)
	if err != nil {
		return ref, nil, err
	}
	if ref.View || ref.System {
		return ref, nil, ErrReadOnly
	}
	cols, err := d.columns(ctx, ref.Phys)
	return ref, cols, err
}

func (d *Data) rowBy(ctx context.Context, phys, pk string, val any) (*Row, error) {
	rows, err := d.admin.QueryContext(ctx, "SELECT * FROM "+quoteIdent(phys)+" WHERE "+quoteIdent(pk)+" = ? LIMIT 1", val)
	if err != nil {
		return nil, err
	}
	_, items, _, err := ReadRows(rows, 1, true)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

func setClause(cols []*SColumn, body map[string]any) ([]string, []any, error) {
	var names []string
	var args []any
	for _, c := range cols { // column order, not map order
		v, ok := body[c.Name]
		if !ok {
			continue
		}
		nv, err := normalize(c, v)
		if err != nil {
			return nil, nil, bad("%s", err.Error())
		}
		names = append(names, c.Name)
		args = append(args, nv)
	}
	for k := range body {
		if colByName(cols, k) == nil {
			return nil, nil, bad("ไม่มีคอลัมน์ %s", k)
		}
	}
	return names, args, nil
}

// InsertRow adds a row and returns it.
func (d *Data) InsertRow(ctx context.Context, name string, body map[string]any) (*Row, error) {
	ref, cols, err := d.writable(ctx, name)
	if err != nil {
		return nil, err
	}
	names, args, err := setClause(cols, body)
	if err != nil {
		return nil, err
	}
	q := make([]string, len(names))
	ph := make([]string, len(names))
	for i, n := range names {
		q[i], ph[i] = quoteIdent(n), "?"
	}
	res, err := d.admin.ExecContext(ctx, "INSERT INTO "+quoteIdent(ref.Phys)+" ("+strings.Join(q, ", ")+") VALUES ("+strings.Join(ph, ", ")+")", args...)
	if err != nil {
		return nil, sqlInput(err)
	}
	pk := primaryKey(cols)
	if pk == nil {
		return &Row{Cols: names, Vals: args}, nil
	}
	var key any
	if v, ok := body[*pk]; ok && v != nil {
		key, _ = normalize(colByName(cols, *pk), v)
	} else {
		key, _ = res.LastInsertId()
	}
	return d.rowBy(ctx, ref.Phys, *pk, key)
}

// sqlInput reports MySQL data errors as bad input.
func sqlInput(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return &BadInput{Msg: fmt.Sprintf("MySQL %d: %s", me.Number, me.Message)}
	}
	return err
}

// UpdateRow changes the row whose primary key is pk.
func (d *Data) UpdateRow(ctx context.Context, name, pkVal string, body map[string]any) (*Row, error) {
	ref, cols, err := d.writable(ctx, name)
	if err != nil {
		return nil, err
	}
	pk := primaryKey(cols)
	if pk == nil {
		return nil, ErrNoPK
	}
	names, args, err := setClause(cols, body)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return d.rowBy(ctx, ref.Phys, *pk, pkVal)
	}
	sets := make([]string, len(names))
	for i, n := range names {
		sets[i] = quoteIdent(n) + " = ?"
	}
	if _, err := d.rowBy(ctx, ref.Phys, *pk, pkVal); err != nil {
		return nil, err
	}
	if _, err := d.admin.ExecContext(ctx, "UPDATE "+quoteIdent(ref.Phys)+" SET "+strings.Join(sets, ", ")+" WHERE "+quoteIdent(*pk)+" = ?",
		append(args, pkVal)...); err != nil {
		return nil, sqlInput(err)
	}
	key := any(pkVal)
	if v, ok := body[*pk]; ok {
		key, _ = normalize(colByName(cols, *pk), v)
	}
	return d.rowBy(ctx, ref.Phys, *pk, key)
}

// DeleteRow removes the row whose primary key is pk.
func (d *Data) DeleteRow(ctx context.Context, name, pkVal string) error {
	ref, cols, err := d.writable(ctx, name)
	if err != nil {
		return err
	}
	pk := primaryKey(cols)
	if pk == nil {
		return ErrNoPK
	}
	res, err := d.admin.ExecContext(ctx, "DELETE FROM "+quoteIdent(ref.Phys)+" WHERE "+quoteIdent(*pk)+" = ?", pkVal)
	if err != nil {
		return sqlInput(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- CSV import / export

// ImportResult is POST /db/tables/{t}/import.
type ImportResult struct {
	Inserted int           `json:"inserted"`
	Errors   []ImportError `json:"errors"`
	DryRun   bool          `json:"dryRun"`
}

// ImportError is a refused CSV line.
type ImportError struct {
	Row     int    `json:"row"` // line number in the file (header = 1)
	Message string `json:"message"`
}

// Import limits.
const (
	maxImportRows   = 50000
	maxImportErrors = 100
)

// Import inserts CSV rows (header = column names) in one transaction;
// dryRun rolls back. Valid rows are kept when others fail.
func (d *Data) Import(ctx context.Context, name string, r io.Reader, dryRun bool) (*ImportResult, error) {
	ref, cols, err := d.writable(ctx, name)
	if err != nil {
		return nil, err
	}
	cr := csv.NewReader(bufio.NewReader(r))
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, bad("อ่านไฟล์ CSV ไม่ได้ (บรรทัดแรกต้องเป็นชื่อคอลัมน์)")
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], string(rune(0xFEFF)))
	}
	hcols := make([]*SColumn, len(header))
	seen := map[string]bool{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		c := colByName(cols, h)
		if c == nil {
			return nil, bad("ไม่มีคอลัมน์ %q ในตาราง %s", h, name)
		}
		if seen[h] {
			return nil, bad("คอลัมน์ %q ซ้ำในหัวตาราง", h)
		}
		seen[h] = true
		hcols[i] = c
	}
	q := make([]string, len(hcols))
	ph := make([]string, len(hcols))
	for i, c := range hcols {
		q[i], ph[i] = quoteIdent(c.Name), "?"
	}
	stmt := "INSERT INTO " + quoteIdent(ref.Phys) + " (" + strings.Join(q, ", ") + ") VALUES (" + strings.Join(ph, ", ") + ")"
	tx, err := d.admin.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res := &ImportResult{Errors: []ImportError{}, DryRun: dryRun}
	line := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line++
		if line-1 > maxImportRows {
			return nil, bad("ไฟล์มีเกิน %d แถว", maxImportRows)
		}
		addErr := func(msg string) {
			if len(res.Errors) < maxImportErrors {
				res.Errors = append(res.Errors, ImportError{Row: line, Message: msg})
			}
		}
		if err != nil {
			addErr("CSV ไม่ถูกต้อง: " + err.Error())
			continue
		}
		if len(rec) != len(hcols) {
			addErr(fmt.Sprintf("จำนวนช่อง (%d) ไม่ตรงกับหัวตาราง (%d)", len(rec), len(hcols)))
			continue
		}
		args := make([]any, len(rec))
		for i, s := range rec {
			c := hcols[i]
			if s == `\N` || (s == "" && (c.Nullable || strings.Contains(c.Extra, "auto_increment"))) {
				args[i] = nil
				continue
			}
			args[i], _ = normalize(c, s)
		}
		if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
			var me *mysql.MySQLError
			if errors.As(err, &me) {
				addErr(fmt.Sprintf("MySQL %d: %s", me.Number, me.Message))
				continue
			}
			return nil, err
		}
		res.Inserted++
	}
	if !dryRun {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// rawString is a raw value as text a MySQL import accepts.
func rawString(dbType string, v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case time.Time:
		if strings.ToUpper(dbType) == "DATE" {
			return x.UTC().Format("2006-01-02"), true
		}
		return x.UTC().Format("2006-01-02 15:04:05.000"), true
	case []byte:
		return string(x), true
	case string:
		return x, true
	}
	return fmt.Sprint(v), true
}

// Export writes the table (optionally searched) as CSV or JSON.
func (d *Data) Export(ctx context.Context, name, format, q string, w io.Writer) error {
	ref, err := d.resolve(ctx, name)
	if err != nil {
		return err
	}
	cols, err := d.columns(ctx, ref.Phys)
	if err != nil {
		return err
	}
	where, args, err := whereFor(cols, q, nil)
	if err != nil {
		return err
	}
	order := ""
	if pk := primaryKey(cols); pk != nil {
		order = " ORDER BY " + quoteIdent(*pk)
	}
	rows, err := d.admin.QueryContext(ctx, "SELECT * FROM "+quoteIdent(ref.Phys)+where+order, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	sc, err := newScanner(rows)
	if err != nil {
		return err
	}
	if format == "json" {
		bw := bufio.NewWriter(w)
		_, _ = bw.WriteString("[")
		n := 0
		for rows.Next() {
			vals, _, err := sc.raw(rows)
			if err != nil {
				return err
			}
			r := Row{Cols: sc.cols, Vals: make([]any, len(vals))}
			for i, v := range vals {
				r.Vals[i] = mapValue(sc.types[i], v)
			}
			b, err := json.Marshal(r)
			if err != nil {
				return err
			}
			if n > 0 {
				_, _ = bw.WriteString(",\n")
			}
			_, _ = bw.Write(b)
			n++
		}
		_, _ = bw.WriteString("]\n")
		if err := rows.Err(); err != nil {
			return err
		}
		return bw.Flush()
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(sc.cols); err != nil {
		return err
	}
	for rows.Next() {
		vals, _, err := sc.raw(rows)
		if err != nil {
			return err
		}
		rec := make([]string, len(vals))
		for i, v := range vals {
			if s, ok := rawString(sc.types[i], v); ok {
				rec[i] = s
			} else {
				rec[i] = `\N`
			}
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := rows.Err(); err != nil {
		return err
	}
	return cw.Error()
}

// ---------------------------------------------------------------- SQL dump

var numericTypes = map[string]bool{"TINYINT": true, "SMALLINT": true, "MEDIUMINT": true, "INT": true, "BIGINT": true, "DECIMAL": true,
	"FLOAT": true, "DOUBLE": true, "YEAR": true}

// sqlLiteral renders a raw value for an INSERT statement.
func sqlLiteral(dbType string, v any) string {
	t := strings.TrimPrefix(strings.ToUpper(dbType), "UNSIGNED ")
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case time.Time:
		if t == "DATE" {
			return quoteString(x.UTC().Format("2006-01-02"))
		}
		return quoteString(x.UTC().Format("2006-01-02 15:04:05.000000"))
	case []byte:
		switch {
		case numericTypes[t]:
			return string(x)
		case t == "BLOB" || t == "BINARY" || t == "VARBINARY" || t == "BIT" || t == "GEOMETRY":
			if len(x) == 0 {
				return "''"
			}
			return "X'" + hex.EncodeToString(x) + "'"
		}
		return quoteString(string(x))
	case string:
		if numericTypes[t] {
			return x
		}
		return quoteString(x)
	}
	return quoteString(fmt.Sprint(v))
}

func (d *Data) dumpTable(ctx context.Context, w *bufio.Writer, phys, as string) error {
	var name, create string
	if err := d.admin.QueryRowContext(ctx, "SHOW CREATE TABLE "+quoteIdent(phys)).Scan(&name, &create); err != nil {
		return err
	}
	if as != phys {
		create = strings.Replace(create, "CREATE TABLE "+quoteIdent(phys), "CREATE TABLE "+quoteIdent(as), 1)
	}
	fmt.Fprintf(w, "\n-- Table %s\nDROP TABLE IF EXISTS %s;\n%s;\n", as, quoteIdent(as), create)
	rows, err := d.admin.QueryContext(ctx, "SELECT * FROM "+quoteIdent(phys))
	if err != nil {
		return err
	}
	defer rows.Close()
	sc, err := newScanner(rows)
	if err != nil {
		return err
	}
	qc := make([]string, len(sc.cols))
	for i, c := range sc.cols {
		qc[i] = quoteIdent(c)
	}
	head := "INSERT INTO " + quoteIdent(as) + " (" + strings.Join(qc, ", ") + ") VALUES\n"
	n := 0
	for rows.Next() {
		vals, _, err := sc.raw(rows)
		if err != nil {
			return err
		}
		if n%100 == 0 {
			if n > 0 {
				_, _ = w.WriteString(";\n")
			}
			_, _ = w.WriteString(head)
		} else {
			_, _ = w.WriteString(",\n")
		}
		lits := make([]string, len(vals))
		for i, v := range vals {
			lits[i] = sqlLiteral(sc.types[i], v)
		}
		_, _ = w.WriteString("(" + strings.Join(lits, ", ") + ")")
		n++
	}
	if n > 0 {
		_, _ = w.WriteString(";\n")
	}
	return rows.Err()
}

func dumpHeader(w *bufio.Writer, what string) {
	fmt.Fprintf(w, "-- Anasaki Studio — Site Code database dump (%s)\n-- %s\nSET NAMES utf8mb4;\nSET FOREIGN_KEY_CHECKS = 0;\n",
		what, time.Now().UTC().Format(time.RFC3339))
}

// Dump writes every developer table (structure + data, kv_docs included,
// no views or backups) as SQL.
func (d *Data) Dump(ctx context.Context, w io.Writer) error {
	list, refs, err := d.objects(ctx)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	dumpHeader(bw, d.Database)
	for _, t := range list {
		if t.View {
			continue
		}
		if err := d.dumpTable(ctx, bw, refs[t.Name].Phys, refs[t.Name].Phys); err != nil {
			return err
		}
	}
	_, _ = bw.WriteString("\nSET FOREIGN_KEY_CHECKS = 1;\n")
	return bw.Flush()
}

// ---------------------------------------------------------------- backups

// BackupInfo is an entry of GET /db/backups.
type BackupInfo struct {
	ID          int64     `json:"id"`
	TableName   string    `json:"tableName"`
	BackupTable string    `json:"backupTable"`
	Migration   string    `json:"migration"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	SizeBytes   int64     `json:"sizeBytes"`
}

// Backups lists the backup copies, newest first.
func (d *Data) Backups(ctx context.Context) ([]BackupInfo, error) {
	rows, err := d.admin.QueryContext(ctx, `SELECT b.id, b.table_name, b.backup_table, b.migration, b.created_at, b.expires_at,
		COALESCE(t.DATA_LENGTH,0) + COALESCE(t.INDEX_LENGTH,0)
		FROM script_table_backups b LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = ? AND t.TABLE_NAME = CONCAT(?, b.backup_table)
		ORDER BY b.id DESC`, d.Database, d.N.Prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupInfo{}
	for rows.Next() {
		var b BackupInfo
		if err := rows.Scan(&b.ID, &b.TableName, &b.BackupTable, &b.Migration, &b.CreatedAt, &b.ExpiresAt, &b.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (d *Data) backup(ctx context.Context, id int64) (*BackupInfo, error) {
	var b BackupInfo
	err := d.admin.QueryRowContext(ctx, `SELECT id, table_name, backup_table, migration, created_at, expires_at FROM script_table_backups WHERE id = ?`, id).
		Scan(&b.ID, &b.TableName, &b.BackupTable, &b.Migration, &b.CreatedAt, &b.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

// DumpBackup writes one backup copy as SQL.
func (d *Data) DumpBackup(ctx context.Context, id int64, w io.Writer) (*BackupInfo, error) {
	b, err := d.backup(ctx, id)
	if err != nil {
		return nil, err
	}
	bw := bufio.NewWriter(w)
	dumpHeader(bw, "backup "+b.BackupTable+" of "+b.TableName)
	if err := d.dumpTable(ctx, bw, d.N.Prefix+b.BackupTable, d.N.Prefix+b.BackupTable); err != nil {
		return nil, err
	}
	_, _ = bw.WriteString("\nSET FOREIGN_KEY_CHECKS = 1;\n")
	return b, bw.Flush()
}

// DeleteBackup drops a backup copy.
func (d *Data) DeleteBackup(ctx context.Context, id int64) (*BackupInfo, error) {
	b, err := d.backup(ctx, id)
	if err != nil {
		return nil, err
	}
	return b, d.dropBackup(ctx, b.ID, b.BackupTable)
}

// ---------------------------------------------------------------- SQL console

// SQLResult is POST /db/sql.
type SQLResult struct {
	Columns      []string `json:"columns"`
	Rows         [][]any  `json:"rows"`
	AffectedRows int64    `json:"affectedRows"`
	InsertID     int64    `json:"insertId"`
	DurationMs   int64    `json:"durationMs"`
	Truncated    bool     `json:"truncated"`
}

// Console limits.
const consoleRows = 1000

var leadingComment = regexp.MustCompile(`^\s*(?:--[^\n]*\n|#[^\n]*\n|/\*[^!+][\s\S]*?\*/|/\*\*/|\s)*`)

// stripSQL trims comments at the start and semicolons at the end.
func stripSQL(s string) string {
	s = leadingComment.ReplaceAllString(s, "")
	return strings.TrimRight(strings.TrimSpace(s), "; \t\r\n")
}

// IsReadSQL reports a statement the read-only console accepts.
func IsReadSQL(s string) bool {
	if strings.Contains(s, "/*!") {
		return false
	}
	return readSQLRe.MatchString(stripSQL(s))
}

// RunSQL runs one statement as the code user. Read mode accepts SELECT-like
// statements only and runs them in a READ ONLY transaction.
func (d *Data) RunSQL(ctx context.Context, query string, params []any, write bool) (*SQLResult, error) {
	if !d.Separate() {
		return nil, ErrFallbackSQL
	}
	body := stripSQL(query)
	if body == "" {
		return nil, bad("กรุณาใส่คำสั่ง SQL")
	}
	if !write && !IsReadSQL(query) {
		return nil, bad("โหมดอ่านอย่างเดียวรับเฉพาะ SELECT, WITH, SHOW, DESCRIBE, EXPLAIN — เปิด \"อนุญาตให้เขียน\" เพื่อแก้ข้อมูล")
	}
	ctx, cancel := context.WithTimeout(ctx, consoleTimout)
	defer cancel()
	conn, err := d.code.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET SESSION MAX_EXECUTION_TIME = %d", consoleTimout.Milliseconds())); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "SET SESSION MAX_EXECUTION_TIME = DEFAULT") }()
	start := time.Now()
	res := &SQLResult{Columns: []string{}, Rows: [][]any{}}
	read := func(rows *sql.Rows) error {
		cols, items, trunc, err := ReadRows(rows, consoleRows, true)
		if err != nil {
			return err
		}
		res.Columns, res.Truncated = cols, trunc
		for _, r := range items {
			res.Rows = append(res.Rows, r.Vals)
		}
		return nil
	}
	if !write {
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		rows, err := tx.QueryContext(ctx, query, params...)
		if err != nil {
			return nil, sqlInput(err)
		}
		if err := read(rows); err != nil {
			return nil, sqlInput(err)
		}
	} else if readSQLRe.MatchString(body) {
		rows, err := conn.QueryContext(ctx, query, params...)
		if err != nil {
			return nil, sqlInput(err)
		}
		if err := read(rows); err != nil {
			return nil, sqlInput(err)
		}
	} else {
		r, err := conn.ExecContext(ctx, query, params...)
		if err != nil {
			return nil, sqlInput(err)
		}
		res.AffectedRows, _ = r.RowsAffected()
		res.InsertID, _ = r.LastInsertId()
	}
	res.DurationMs = time.Since(start).Milliseconds()
	if res.Columns == nil {
		res.Columns = []string{}
	}
	return res, nil
}
