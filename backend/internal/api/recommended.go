package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Setup wizard step "Recommended plugins" (phase 7): the official plugins
// that replaced the built-ins, installed from the Store right after the site
// is created. Setup never depends on the Store: when it is unreachable the
// wizard finishes and says "install later from the Plugins page".

// recommendedPlugins are offered by the wizard; Checked = ticked by default.
var recommendedPlugins = []struct {
	ID      string
	Checked bool
}{
	{"seo", true}, {"redirects", true}, {"forms", true}, {"cookie-consent", true},
	{"analytics", false}, {"maintenance", false}, {"custom-code", false},
}

// RecommendedItem is an entry of GET /api/v1/setup/recommended.
type RecommendedItem struct {
	ID          string      `json:"id"`
	Name        plugin.I18n `json:"name"`
	Description plugin.I18n `json:"description"`
	Icon        string      `json:"icon"`
	Checked     bool        `json:"checked"`
	// From the Store (empty when it is unreachable):
	Available   bool     `json:"available"` // published in the Store
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

// recommendedList answers GET /api/v1/setup/recommended.
func (a *API) recommendedList(w http.ResponseWriter, r *http.Request) {
	items := []RecommendedItem{}
	storeOK := true
	for _, rp := range recommendedPlugins {
		b := legacy.Get(rp.ID)
		it := RecommendedItem{ID: rp.ID, Name: plugin.T(b.Name, b.NameEn), Description: plugin.T(b.Description, b.DescEn),
			Icon: b.Icon, Checked: rp.Checked, Permissions: []string{}, Scopes: []string{}, Hosts: []string{},
			Installed: a.plugins.InstalledVersion(rp.ID) != ""}
		if storeOK {
			v, err := a.plugins.StoreJSON(http.MethodGet, "/api/v1/plugins/"+url.PathEscape(rp.ID), nil, nil, plugin.StoreRaw, nil)
			switch {
			case storeDown(err):
				storeOK = false
			case err == nil:
				if o, ok := v.(map[string]any); ok {
					it.Available = true
					it.Version, _ = o["version"].(string)
					it.Permissions = strList(o["permissions"])
					if c, ok := o["code"].(map[string]any); ok {
						it.Scopes, it.Hosts = strList(c["scopes"]), strList(c["hosts"])
					}
				}
			}
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"storeAvailable": storeOK, "items": items})
}

// RecommendedResult is the outcome of one install.
type RecommendedResult struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// recommendedInstall answers POST /api/v1/setup/recommended/install {ids}:
// downloads each official plugin from the Store, checks it is official,
// installs and activates it (the wizard showed its permissions).
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
	storeOK := true
	for _, rp := range recommendedPlugins { // known ids only, in the list order
		if !want[rp.ID] {
			continue
		}
		res := RecommendedResult{ID: rp.ID}
		if !storeOK {
			res.Error = "store unavailable"
			results = append(results, res)
			continue
		}
		inst, err := a.installOfficial(rp.ID)
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
	if _, err := legacy.VerifyOfficial(data, id); err != nil {
		return plugin.Installed{}, err
	}
	if _, err := a.plugins.InstallPackage(data, "store", id); err != nil {
		return plugin.Installed{}, err
	}
	return a.plugins.SetPackageActive(id, true)
}

// storeDown reports an unreachable Store (network error or a 5xx answer).
func storeDown(err error) bool {
	var se *plugin.StoreError
	return errors.Is(err, plugin.ErrStoreUnavailable) || (errors.As(err, &se) && se.Status >= 500)
}
