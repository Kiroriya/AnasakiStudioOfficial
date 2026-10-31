package design

import "encoding/json"

// Site Code data for the renderer. The design package never imports
// sitecode: a DataSource that can serve code:/table: collections and script
// formatters also implements Dynamic (site.Source does when Site Code is
// live), and the site wires the request-scoped CodeSession.

// Dynamic serves Site Code collections and formatters.
type Dynamic interface {
	// LoadCollection loads a "code:<name>" or "table:<name>" source.
	LoadCollection(req CollectionReq) CollectionRes
	// Format runs a site.formatter; ok=false when there is none.
	Format(name string, v any, args []any) (string, bool)
}

// CollectionReq asks for one page of a code:/table: collection.
type CollectionReq struct {
	Source  string         // "code:events" / "table:orders"
	Page    int            // 1-based
	PerPage int            // 0 = the collection's default
	Params  map[string]any // evaluated props.params (+ filter values from the URL)
	Table   *TableSpec     // table: sources (props.table)
}

// TableSpec is props.table of a table: collection.
type TableSpec struct {
	Columns []string    `json:"columns,omitempty"`
	Where   []TableCond `json:"where,omitempty"`
	OrderBy []TableSort `json:"orderBy,omitempty"`
}

// TableCond is one where condition; Value is already evaluated.
type TableCond struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  any    `json:"value,omitempty"`
}

// TableSort is one orderBy entry.
type TableSort struct {
	Column string `json:"column"`
	Dir    string `json:"dir"`
}

// CollectionRes is the loaded page.
type CollectionRes struct {
	Items   []map[string]any `json:"items"`
	Total   int              `json:"total"`
	PerPage int              `json:"perPage"` // effective page size
	Page    int              `json:"page"`
	// Error is set when loading failed (logged; the collection renders empty).
	Error string `json:"error,omitempty"`
	// Denied: the viewer may not use this source (renders empty).
	Denied bool `json:"denied,omitempty"`
}

// CodeSession is Site Code bound to one public request (the visitor's user
// and cookies); the site gets one per page render / partial / action.
type CodeSession interface {
	Dynamic
	// PageData runs the site.page handler matching path (nil when none).
	PageData(path string) map[string]any
	// RunAction validates input and runs site.action name.
	RunAction(name string, input map[string]any) *ActionOutcome
	// Cookies are the Set-Cookie values the code set (response.setCookie).
	Cookies() []string
	// Close releases the session's VM.
	Close()
}

// ActionOutcome is what POST /_a/{name} answers (before nodes are added).
type ActionOutcome struct {
	Status   int               // HTTP status (200 on success)
	Error    string            // error message for non-200
	Errors   map[string]string // per-field messages (validation or run result)
	Message  string
	Redirect string
	Refresh  []string       // collection names (site.collection) to re-render
	Update   []string       // layer names to re-render
	Page     map[string]any // merged into page data for the re-render
	Data     any
	// SetCookies are complete Set-Cookie header values (response.setCookie).
	SetCookies []string
}

// ContentFilter is implemented by a DataSource whose post content passes
// through Site Code's post.content filters (phase 4a). The result must be
// safe HTML (the implementation sanitizes it).
type ContentFilter interface {
	FilterContent(html string, post PostCtx) string
}

// Filterer is implemented by a CodeSession that runs Site Code filters
// (site.filter): ApplyFilter returns value unchanged when no filter is
// registered under name.
type Filterer interface {
	HasFilter(name string) bool
	ApplyFilter(name string, value any, ctx map[string]any) any
}

// VisitorSource is implemented by a CodeSession that resolves the visitor
// (site.visitor): nil = anonymous / none.
type VisitorSource interface {
	Visitor() map[string]any
}

// postValue exposes a post to expressions; custom fields (post.meta.<key>)
// travel as JSON in PostCtx["meta"] and are decoded here.
func postValue(p PostCtx) any {
	raw, ok := p["meta"]
	if !ok || raw == "" || raw[0] != '{' {
		return p
	}
	var meta map[string]any
	if json.Unmarshal([]byte(raw), &meta) != nil {
		return p
	}
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	out["meta"] = meta
	return out
}
