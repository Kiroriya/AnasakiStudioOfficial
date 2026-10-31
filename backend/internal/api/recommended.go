package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Setup wizard step "Recommended plugins": the plugins the configured Store
// marks as recommended (store admins decide; the core never names a plugin),
// installed right after the site is created. Setup never depends on the
// Store: when it is unreachable or recommends nothing the wizard finishes and
// says "install later from the Plugins page".

// RecommendedItem is an entry of GET /api/v1/setup/recommended.
type RecommendedItem struct {
	ID string `json:"id"`
	// Name / Description as the Store sends them: a plain string or {th,en}.
	Name        any    `json:"name"`
	Description any    `json:"description"`
	Icon        string `json:"icon"`
	IconURL     string `json:"iconUrl,omitempty"`
	Checked     bool   `json:"checked"` // ticked by default (Store "recommendedChecked")
	Order       int    `json:"order"`   // Store "recommendedOrder"
	// Available: published in the Store (always true for a listed item).
	Available   bool     `json:"available"`
	Version     string   `json:"version"`
	Permissions []string `json:"permissions"`
	Scopes      []string `json:"scopes"`
	Hosts       []string `json:"hosts"`
	Installed   bool     `json:"installed"`
}

func (a *API) recommendedRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	h("GET /api/v1/setup/recommended", auth.CapManageOptions, a.recommendedList)
	h("POST /api/v1/setup/recommended/install", auth.CapManageOptions, a.recommendedInstall)
}

func strList(v any) []string {
	out := []string{}
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// textOf keeps a Store text field when it is a string or an object ({th,en}).
func textOf(v any) any {
	switch v.(type) {
	case string, map[string]any:
		return v
	}
	return ""
}

// sortName is the name used to sort recommendations (English first).
func sortName(o map[string]any) string {
	switch n := o["name"].(type) {
	case string:
		return strings.ToLower(n)
	case map[string]any:
		s, _ := n["en"].(string)
		if s == "" {
			s, _ = n["th"].(string)
		}
		return strings.ToLower(s)
	}
	return ""
}

// storeRecommended asks the Store for its recommended plugins
// (GET /api/v1/plugins?recommended=1&perPage=50) and keeps the official ones,
// sorted by recommendedOrder then name.
func (a *API) storeRecommended() ([]map[string]any, error) {
	q := url.Values{"recommended": {"1"}, "perPage": {"50"}}
	v, err := a.plugins.StoreJSON(http.MethodGet, "/api/v1/plugins", q, nil, plugin.StoreRaw, nil)
	if err != nil {
		return nil, err
	}
	var raw []any
	switch x := v.(type) {
	case map[string]any:
		raw, _ = x["items"].([]any)
	case []any:
		raw = x
	}
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, x := range raw {
		o, ok := x.(map[string]any)
		if !ok {
			continue
		}
		id, _ := o["id"].(string)
		// a Store that ignores the filter must not turn its whole catalogue
		// into recommendations: both flags are required.
		if !plugin.ValidPackageID(id) || seen[id] || o["recommended"] != true || o["official"] != true {
			continue
		}
		seen[id] = true
		out = append(out, o)
	}
	order := func(o map[string]any) float64 { f, _ := o["recommendedOrder"].(float64); return f }
	sort.SliceStable(out, func(i, j int) bool {
		if oi, oj := order(out[i]), order(out[j]); oi != oj {
			return oi < oj
		}
		if ni, nj := sortName(out[i]), sortName(out[j]); ni != nj {
			return ni < nj
		}
		return out[i]["id"].(string) < out[j]["id"].(string)
	})
	return out, nil
}

// recommendedList answers GET /api/v1/setup/recommended.
func (a *API) recommendedList(w http.ResponseWriter, r *http.Request) {
	items := []RecommendedItem{}
	list, err := a.storeRecommended()
	if err != nil {
		// unreachable, or a Store without the recommended list: nothing to offer
		writeJSON(w, http.StatusOK, map[string]any{"storeAvailable": false, "items": items})
		return
	}
	for _, o := range list {
		id := o["id"].(string)
		it := RecommendedItem{ID: id, Name: textOf(o["name"]), Description: textOf(o["description"]), Available: true,
			Permissions: []string{}, Scopes: []string{}, Hosts: []string{}, Installed: a.plugins.InstalledVersion(id) != "",
			Checked: o["recommendedChecked"] == true}
		it.Icon, _ = o["icon"].(string)
		it.IconURL, _ = o["iconUrl"].(string)
		it.Version, _ = o["version"].(string)
		if f, ok := o["recommendedOrder"].(float64); ok {
			it.Order = int(f)
		}
		code, _ := o["code"].(map[string]any)
		perms, hasPerms := o["permissions"]
		if !hasPerms {
			// list summaries carry no permissions, the detail does (the wizard
			// shows what each plugin asks for before installing it)
			if d, err := a.plugins.StoreJSON(http.MethodGet, "/api/v1/plugins/"+url.PathEscape(id), nil, nil, plugin.StoreRaw, nil); err == nil {
				if dm, ok := d.(map[string]any); ok {
					perms = dm["permissions"]
					if c, ok := dm["code"].(map[string]any); ok && code == nil {
						code = c
					}
				}
			}
		}
		it.Permissions = strList(perms)
		if code != nil {
			it.Scopes, it.Hosts = strList(code["scopes"]), strList(code["hosts"])
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"storeAvailable": true, "items": items})
}

// RecommendedResult is the outcome of one install.
type RecommendedResult struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// recommendedInstall answers POST /api/v1/setup/recommended/install {ids}:
// only ids the Store currently lists as recommended and official are
// installed, in the list order (other ids are ignored); each package is
// downloaded, checked to be official, installed and activated (the wizard
// showed its permissions).
func (a *API) recommendedInstall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	want := map[string]bool{}
	for _, id := range in.IDs {
		want[id] = true
	}
	results := []RecommendedResult{}
	list, err := a.storeRecommended()
	if err != nil {
		for id := range want {
			if plugin.ValidPackageID(id) {
				results = append(results, RecommendedResult{ID: id, Error: "store unavailable"})
			}
		}
		sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
		writeJSON(w, http.StatusOK, map[string]any{"storeAvailable": false, "results": results})
		return
	}
	storeOK := true
	for _, o := range list {
		id := o["id"].(string)
		if !want[id] {
			continue
		}
		res := RecommendedResult{ID: id}
		if !storeOK {
			res.Error = "store unavailable"
			results = append(results, res)
			continue
		}
		inst, err := a.installOfficial(id)
		switch {
		case storeDown(err):
			storeOK = false
			res.Error = "store unavailable"
		case err != nil:
			res.Error = err.Error()
		default:
			res.OK, res.Version = true, inst.Version
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"storeAvailable": storeOK, "results": results})
}

// installOfficial installs (unless installed) and activates official plugin id.
func (a *API) installOfficial(id string) (plugin.Installed, error) {
	if inst, err := a.plugins.GetPackage(id); err == nil {
		if inst.Active {
			return inst, nil
		}
		return a.plugins.SetPackageActive(id, true)
	}
	data, err := a.plugins.DownloadFromStore(id, "")
	if err != nil {
		var se *plugin.StoreError
		if errors.As(err, &se) && se.Status == http.StatusNotFound {
			return plugin.Installed{}, errors.New("not in the Store yet")
		}
		return plugin.Installed{}, err
	}
	if err := verifyOfficialPackage(data, id); err != nil {
		return plugin.Installed{}, err
	}
	if _, err := a.plugins.InstallPackage(data, "store", id); err != nil {
		return plugin.Installed{}, err
	}
	return a.plugins.SetPackageActive(id, true)
}

// verifyOfficialPackage checks a downloaded zip is plugin id by the official
// author (the Store's "official" flag was checked before the download).
func verifyOfficialPackage(data []byte, id string) error {
	mf, _, err := plugin.ParsePackage(data)
	if err != nil {
		return err
	}
	if mf.ID != id {
		return &plugin.PackageError{Msg: fmt.Sprintf("this is plugin %q, not %q", mf.ID, id)}
	}
	if mf.Author != plugin.OfficialAuthor {
		return &plugin.PackageError{Msg: fmt.Sprintf("%q is not an official %s plugin", id, plugin.OfficialAuthor)}
	}
	return nil
}

// storeDown reports an unreachable Store (network error or a 5xx answer).
func storeDown(err error) bool {
	var se *plugin.StoreError
	return errors.Is(err, plugin.ErrStoreUnavailable) || (errors.As(err, &se) && se.Status >= 500)
}
