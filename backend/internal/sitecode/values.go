package sitecode

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Result values as scripts and the admin API see them: DECIMAL → string
// (exact), DATETIME/DATE → ISO string, JSON → parsed, BIGINT → number when
// safe else string, TINYINT → boolean.

// JSONText is a JSON column value (kept as text, parsed on the way out).
type JSONText string

// MarshalJSON emits the stored JSON as is.
func (j JSONText) MarshalJSON() ([]byte, error) {
	if json.Valid([]byte(j)) {
		return []byte(j), nil
	}
	return json.Marshal(string(j))
}

// Result limits.
const (
	MaxResultRows  = 10000
	MaxResultBytes = 10 << 20
	maxSafeInt     = 1<<53 - 1
)

// Row is a result row with ordered columns.
type Row struct {
	Cols []string
	Vals []any
}

// MarshalJSON keeps the column order.
func (r Row) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	seen := map[string]int{}
	for i, c := range r.Cols {
		seen[c] = i // the last duplicate wins
	}
	first := true
	for i, c := range r.Cols {
		if seen[c] != i {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(c)
		b.Write(k)
		b.WriteByte(':')
		v, err := json.Marshal(r.Vals[i])
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Get returns a column's value.
func (r Row) Get(col string) (any, bool) {
	for i := len(r.Cols) - 1; i >= 0; i-- {
		if r.Cols[i] == col {
			return r.Vals[i], true
		}
	}
	return nil, false
}

const isoMillis = "2006-01-02T15:04:05.000Z"

// mapValue converts a scanned value by its column type.
func mapValue(dbType string, v any) any {
	if v == nil {
		return nil
	}
	t := strings.ToUpper(dbType)
	if t == "UNSIGNED TINYINT" {
		t = "SMALLINT" // only signed TINYINT (boolean) maps to true/false
	}
	t = strings.TrimPrefix(t, "UNSIGNED ")
	switch x := v.(type) {
	case time.Time:
		if t == "DATE" {
			return x.UTC().Format("2006-01-02")
		}
		return x.UTC().Format(isoMillis)
	case int64:
		return mapInt(t, x)
	case uint64:
		if x > maxSafeInt {
			return strconv.FormatUint(x, 10)
		}
		return mapInt(t, int64(x))
	case float64:
		return x
	case float32:
		return float64(x)
	case bool:
		return x
	case []byte:
		return mapText(t, string(x), x)
	case string:
		return mapText(t, x, []byte(x))
	}
	return fmt.Sprint(v)
}

func mapInt(t string, x int64) any {
	if t == "TINYINT" {
		return x != 0
	}
	if x > maxSafeInt || x < -maxSafeInt {
		return strconv.FormatInt(x, 10)
	}
	return x
}

func mapText(t, s string, raw []byte) any {
	switch t {
	case "DECIMAL":
		return s
	case "TINYINT":
		return s != "0" && s != ""
	case "INT", "SMALLINT", "MEDIUMINT", "BIGINT", "YEAR":
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return mapInt(t, n)
		}
		return s
	case "FLOAT", "DOUBLE":
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	case "JSON":
		return JSONText(s)
	case "DATETIME", "TIMESTAMP":
		if tm, err := time.Parse("2006-01-02 15:04:05.999999", s); err == nil {
			return tm.UTC().Format(isoMillis)
		}
		return s
	case "BIT":
		var n int64
		for _, c := range raw {
			n = n<<8 | int64(c)
		}
		return n
	}
	if !utf8.Valid(raw) {
		return fmt.Sprintf("%x", raw)
	}
	return s
}

// rowScanner reads rows into raw values with the column types.
type rowScanner struct {
	cols  []string
	types []string
}

func newScanner(rows *sql.Rows) (*rowScanner, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	s := &rowScanner{cols: cols, types: make([]string, len(cts))}
	for i, ct := range cts {
		s.types[i] = ct.DatabaseTypeName()
	}
	return s, nil
}

// raw scans one row without mapping ([]byte copied).
func (s *rowScanner) raw(rows *sql.Rows) ([]any, int, error) {
	vals := make([]any, len(s.cols))
	ptrs := make([]any, len(s.cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, 0, err
	}
	size := 0
	for i, v := range vals {
		switch x := v.(type) {
		case []byte:
			c := make([]byte, len(x))
			copy(c, x)
			vals[i] = c
			size += len(x)
		case string:
			size += len(x)
		default:
			size += 8
		}
	}
	return vals, size, nil
}

// ErrTooMany is returned when a result exceeds the row or size cap.
type ErrTooMany struct{ Msg string }

func (e *ErrTooMany) Error() string { return e.Msg }

// ReadRows reads mapped rows with the caps (maxRows ≤ 0 = MaxResultRows).
func ReadRows(rows *sql.Rows, maxRows int, truncate bool) ([]string, []Row, bool, error) {
	defer rows.Close()
	if maxRows <= 0 {
		maxRows = MaxResultRows
	}
	sc, err := newScanner(rows)
	if err != nil {
		return nil, nil, false, err
	}
	out := []Row{}
	total := 0
	for rows.Next() {
		if len(out) >= maxRows {
			if truncate {
				return sc.cols, out, true, nil
			}
			return nil, nil, false, &ErrTooMany{Msg: fmt.Sprintf("ผลลัพธ์เกิน %d แถว — ใช้ paginate() หรือ limit()", maxRows)}
		}
		vals, size, err := sc.raw(rows)
		if err != nil {
			return nil, nil, false, err
		}
		total += size
		if total > MaxResultBytes {
			if truncate {
				return sc.cols, out, true, nil
			}
			return nil, nil, false, &ErrTooMany{Msg: "ผลลัพธ์ใหญ่เกิน 10 MB — ใช้ paginate() หรือ limit()"}
		}
		r := Row{Cols: sc.cols, Vals: make([]any, len(vals))}
		for i, v := range vals {
			r.Vals[i] = mapValue(sc.types[i], v)
		}
		out = append(out, r)
	}
	return sc.cols, out, false, rows.Err()
}

// isoRe matches Date.prototype.toISOString output, which MySQL does not
// accept for DATETIME: such strings are bound as times.
var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$`)

// BindValue normalises a bound parameter.
func BindValue(v any) any {
	if s, ok := v.(string); ok && isoRe.MatchString(s) {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
	}
	return v
}
