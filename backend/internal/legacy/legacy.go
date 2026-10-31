// Package legacy reads what the former built-in (Go) plugins left in the
// database — their tables and options — so the official JS plugins that
// replace them can take the data over (phase 7). Everything here is
// read-only except the explicit clean-up (DropOld) an administrator asks for.
//
// The old table names and column lists are known here only; they are copied
// from the Migrate code of the removed plugins.
package legacy

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Column kinds: how a value of the old table is written in a rows payload.
const (
	KindInt     = "int"     // number
	KindStr     = "str"     // string
	KindNullStr = "nullstr" // string or null
	KindBool    = "bool"    // true / false
	KindJSON    = "json"    // the parsed JSON value
	KindTime    = "time"    // RFC 3339 UTC ("2024-05-01T10:00:00Z")
	KindDate    = "date"    // "2024-05-01"
)

// Column is a column of an old table.
type Column struct {
	Name string
	Kind string
}

// Table is an old table of a built-in plugin.
type Table struct {
	Name    string
	Key     []string // stable order for paging (the primary key)
	Columns []Column
}

// Builtin describes a former built-in plugin.
type Builtin struct {
	ID          string
	Name        string // Thai
	NameEn      string
	Description string
	DescEn      string
	Icon        string
	// Tables in dependency order (a table comes after the ones it references).
	Tables []Table
	// Options it stored besides its settings (OptionSettingsPrefix + id).
	Options []string
}

// OptionActive and OptionSettingsPrefix mirror the plugin platform's option
// names of the built-in era (internal/plugin).
const (
	OptionActive         = "active_plugins"
	OptionSettingsPrefix = "plugin_settings."
)

// Builtins are the seven built-ins moved to official JS plugins, in the order
// the admin listed them. (backup stays in core.)
var Builtins = []Builtin{
	{ID: "analytics", Name: "สถิติผู้เข้าชม", NameEn: "Analytics", Icon: "📈",
		Description: "นับยอดเข้าชมฝั่งเซิร์ฟเวอร์ ไม่ใช้คุกกี้ ไม่เก็บ IP", DescEn: "Server-side page views, no cookies, no IP addresses",
		Tables: []Table{
			{Name: "analytics_daily", Key: []string{"day", "path"}, Columns: []Column{
				{"day", KindDate}, {"path", KindStr}, {"title", KindStr}, {"views", KindInt}}},
			{Name: "analytics_visitors", Key: []string{"day", "hash"}, Columns: []Column{
				{"day", KindDate}, {"hash", KindStr}}},
			{Name: "analytics_page_visitors", Key: []string{"day", "path", "hash"}, Columns: []Column{
				{"day", KindDate}, {"path", KindStr}, {"hash", KindStr}}},
			{Name: "analytics_referrers", Key: []string{"day", "host"}, Columns: []Column{
				{"day", KindDate}, {"host", KindStr}, {"views", KindInt}}},
		},
		Options: []string{"analytics_salt"}},
	{ID: "seo", Name: "SEO", NameEn: "SEO", Icon: "🔍",
		Description: "ชื่อหน้า คำอธิบาย รูปแชร์ sitemap.xml และ robots.txt", DescEn: "Titles, descriptions, share images, sitemap.xml and robots.txt",
		Tables: []Table{
			{Name: "seo_meta", Key: []string{"post_id"}, Columns: []Column{
				{"post_id", KindInt}, {"title", KindStr}, {"description", KindNullStr}, {"image", KindStr},
				{"noindex", KindBool}, {"updated_at", KindTime}}},
		}},
	{ID: "forms", Name: "ฟอร์ม", NameEn: "Forms", Icon: "📝",
		Description: "สร้างฟอร์มติดต่อ เก็บข้อความ และแจ้งเตือนทางอีเมล", DescEn: "Contact forms, stored submissions and email notifications",
		Tables: []Table{
			{Name: "forms_forms", Key: []string{"id"}, Columns: []Column{
				{"id", KindInt}, {"name", KindStr}, {"fields", KindJSON}, {"submit_label", KindStr},
				{"success_message", KindStr}, {"redirect_url", KindStr}, {"webhook_url", KindStr}, {"created_at", KindTime}}},
			{Name: "forms_submissions", Key: []string{"id"}, Columns: []Column{
				{"id", KindInt}, {"form_id", KindInt}, {"data", KindJSON}, {"is_read", KindBool}, {"created_at", KindTime}}},
		}},
	{ID: "redirects", Name: "เปลี่ยนเส้นทาง", NameEn: "Redirects", Icon: "↪️",
		Description: "ย้ายลิงก์เก่าไปหน้าใหม่ (301/302) และดูลิงก์เสีย 404", DescEn: "Send old links to new pages (301/302) and see broken 404 links",
		Tables: []Table{
			{Name: "redirects_rules", Key: []string{"id"}, Columns: []Column{
				{"id", KindInt}, {"from_path", KindStr}, {"to_url", KindStr}, {"code", KindInt}, {"hits", KindInt}, {"created_at", KindTime}}},
			{Name: "redirects_404", Key: []string{"path"}, Columns: []Column{
				{"path", KindStr}, {"hits", KindInt}, {"last_seen", KindTime}}},
		}},
	{ID: "cookie-consent", Name: "คุกกี้ (PDPA)", NameEn: "Cookie consent", Icon: "🍪",
		Description: "แถบขอความยินยอมคุกกี้ตาม PDPA", DescEn: "A PDPA cookie consent banner"},
	{ID: "maintenance", Name: "โหมดปิดปรับปรุง", NameEn: "Maintenance", Icon: "🚧",
		Description: "ปิดเว็บชั่วคราวพร้อมหน้าแจ้งผู้เข้าชม", DescEn: "Temporarily close the site with a notice page"},
	{ID: "custom-code", Name: "โค้ดเพิ่มเติม", NameEn: "Custom code", Icon: "🧩",
		Description: "แทรกโค้ด HTML/JS ใน <head> หรือท้ายหน้า", DescEn: "Insert HTML/JS into <head> or the end of the page"},
}

// Get returns the built-in with id (nil when it is not one of the seven).
func Get(id string) *Builtin {
	for i := range Builtins {
		if Builtins[i].ID == id {
			return &Builtins[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------- detection

// Detection is what a built-in left behind.
type Detection struct {
	Active      bool             `json:"active"`      // listed in active_plugins
	Rows        map[string]int64 `json:"rows"`        // table → row count (existing tables only)
	HasSettings bool             `json:"hasSettings"` // plugin_settings.<id> is stored
	HasOptions  bool             `json:"hasOptions"`  // other options (Builtin.Options)
}

// HasData reports rows in any old table or stored settings.
func (d Detection) HasData() bool {
	if d.HasSettings {
		return true
	}
	for _, n := range d.Rows {
		if n > 0 {
			return true
		}
	}
	return false
}

// Leftovers reports anything a clean-up would remove (tables, even empty, or options).
func (d Detection) Leftovers() bool { return len(d.Rows) > 0 || d.HasSettings || d.HasOptions }

// ActiveIDs reads the built-in era's active plugin list.
func ActiveIDs(db *sql.DB) (map[string]bool, error) {
	out := map[string]bool{}
	var raw string
	err := db.QueryRow(`SELECT value FROM options WHERE name = ?`, OptionActive).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	_ = json.Unmarshal([]byte(raw), &ids)
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// TableExists reports whether an old table is present.
func TableExists(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, name).Scan(&n)
	return n > 0, err
}

// Detect reports what built-in id left behind.
func Detect(db *sql.DB, id string) (Detection, error) {
	b := Get(id)
	if b == nil {
		return Detection{}, fmt.Errorf("legacy: unknown built-in %q", id)
	}
	d := Detection{Rows: map[string]int64{}}
	active, err := ActiveIDs(db)
	if err != nil {
		return d, err
	}
	d.Active = active[id]
	for _, t := range b.Tables {
		ok, err := TableExists(db, t.Name)
		if err != nil {
			return d, err
		}
		if !ok {
			continue
		}
		var n int64
		if err := db.QueryRow("SELECT COUNT(*) FROM `" + t.Name + "`").Scan(&n); err != nil {
			return d, err
		}
		d.Rows[t.Name] = n
	}
	if _, ok, err := Settings(db, id); err != nil {
		return d, err
	} else {
		d.HasSettings = ok
	}
	for _, o := range b.Options {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM options WHERE name = ?`, o).Scan(&n); err != nil {
			return d, err
		}
		d.HasOptions = d.HasOptions || n > 0
	}
	return d, nil
}

// Settings returns the built-in's stored settings (same keys as the official
// plugin's manifest settings); ok=false when none were saved.
func Settings(db *sql.DB, id string) (map[string]any, bool, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM options WHERE name = ?`, OptionSettingsPrefix+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}, true, nil // unreadable: nothing to map, still counts as data
	}
	return out, true, nil
}

// ---------------------------------------------------------------- rows

// MaxBatch is the largest rows batch handed to legacyImport.
const MaxBatch = 500

// ReadRows reads up to limit rows of an old table from offset, in key order,
// as payload values (see the Kind constants). A missing table reads as empty.
func ReadRows(db *sql.DB, t Table, offset int64, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > MaxBatch {
		limit = MaxBatch
	}
	ok, err := TableExists(db, t.Name)
	if err != nil || !ok {
		return []map[string]any{}, err
	}
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = "`" + c.Name + "`"
	}
	keys := make([]string, len(t.Key))
	for i, k := range t.Key {
		keys[i] = "`" + k + "`"
	}
	rows, err := db.Query("SELECT "+strings.Join(cols, ", ")+" FROM `"+t.Name+"` ORDER BY "+strings.Join(keys, ", ")+
		" LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(t.Columns))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(vals))
		for i, c := range t.Columns {
			row[c.Name] = convert(vals[i], c.Kind)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func asString(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case []byte:
		return string(x), true
	case string:
		return x, true
	case time.Time:
		return x.UTC().Format(time.RFC3339), true
	default:
		return fmt.Sprint(x), true
	}
}

// convert turns a scanned driver value into the payload value of kind.
func convert(v any, kind string) any {
	switch kind {
	case KindInt:
		switch x := v.(type) {
		case int64:
			return x
		case nil:
			return int64(0)
		}
		s, _ := asString(v)
		n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n
	case KindBool:
		switch x := v.(type) {
		case int64:
			return x != 0
		case bool:
			return x
		}
		s, _ := asString(v)
		return s == "1" || strings.EqualFold(s, "true")
	case KindJSON:
		s, ok := asString(v)
		if !ok {
			return nil
		}
		var out any
		if json.Unmarshal([]byte(s), &out) != nil {
			return s
		}
		return out
	case KindTime:
		if t, ok := v.(time.Time); ok {
			return t.UTC().Format(time.RFC3339)
		}
		s, ok := asString(v)
		if !ok {
			return nil
		}
		if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
		return s
	case KindDate:
		if t, ok := v.(time.Time); ok {
			return t.Format("2006-01-02")
		}
		s, ok := asString(v)
		if !ok {
			return nil
		}
		if len(s) > 10 {
			s = s[:10]
		}
		return s
	case KindNullStr:
		s, ok := asString(v)
		if !ok {
			return nil
		}
		return s
	default:
		s, _ := asString(v)
		return s
	}
}

// ---------------------------------------------------------------- clean-up

// DropOld removes what built-in id left behind: its tables, its settings and
// other options, and its entry in active_plugins. Only ever called on an
// administrator's explicit request ("Clean up old data").
func DropOld(db *sql.DB, id string) error {
	b := Get(id)
	if b == nil {
		return fmt.Errorf("legacy: unknown built-in %q", id)
	}
	for i := len(b.Tables) - 1; i >= 0; i-- {
		if _, err := db.Exec("DROP TABLE IF EXISTS `" + b.Tables[i].Name + "`"); err != nil {
			return err
		}
	}
	names := append([]string{OptionSettingsPrefix + id}, b.Options...)
	for _, n := range names {
		if _, err := db.Exec(`DELETE FROM options WHERE name = ?`, n); err != nil {
			return err
		}
	}
	var raw string
	if err := db.QueryRow(`SELECT value FROM options WHERE name = ?`, OptionActive).Scan(&raw); err == nil {
		var ids []string
		_ = json.Unmarshal([]byte(raw), &ids)
		keep := []string{}
		for _, x := range ids {
			if x != id {
				keep = append(keep, x)
			}
		}
		if len(keep) != len(ids) {
			nb, _ := json.Marshal(keep)
			if _, err := db.Exec(`UPDATE options SET value = ? WHERE name = ?`, string(nb), OptionActive); err != nil {
				return err
			}
		}
	}
	return nil
}
