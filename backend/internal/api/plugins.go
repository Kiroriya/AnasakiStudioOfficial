package api

import (
	"errors"
	"github.com/anasaki/studio/backend/internal/design"
	"net/http"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/plugin"
)

func (a *API) listPlugins(w http.ResponseWriter, r *http.Request) {
	// built-ins, plus active service plugins (their admin menu and collection sources)
	all := append(a.plugins.List(), a.plugins.ServiceStatuses()...)
	writeJSON(w, http.StatusOK, visiblePlugins(auth.From(r), all))
}

// visiblePlugins is what a user sees of the plugin list. Every role builds
// its admin menu (and the dashboard its widgets) from it, so it stays open to
// all signed-in users; without manage_options (the plugins screen) only the
// active plugins are listed, with just the menu entries the user may open and,
// for the designer (edit_pages), the collection sources.
func visiblePlugins(me *auth.Principal, all []plugin.Status) []plugin.Status {
	if me.Can(auth.CapManageOptions) {
		return all
	}
	out := []plugin.Status{}
	for _, s := range all {
		if !s.Active {
			continue
		}
		menu := []plugin.MenuItem{}
		for _, m := range s.Menu {
			cap := m.Cap
			if cap == "" {
				cap = auth.CapManageOptions
			}
			if me.Can(cap) {
				menu = append(menu, m)
			}
		}
		s.Menu = menu
		if !me.Can(auth.CapEditPages) {
			s.Collections = []plugin.Collection{}
		}
		out = append(out, s)
	}
	return out
}

func (a *API) activatePlugin(w http.ResponseWriter, r *http.Request) {
	st, err := a.plugins.Activate(r.PathValue("id"), auth.From(r).ID)
	a.pluginResult(w, st, err)
}

func (a *API) deactivatePlugin(w http.ResponseWriter, r *http.Request) {
	st, err := a.plugins.Deactivate(r.PathValue("id"))
	a.pluginResult(w, st, err)
}

func (a *API) pluginResult(w http.ResponseWriter, st plugin.Status, err error) {
	if errors.Is(err, plugin.ErrUnknown) {
		writeErr(w, http.StatusNotFound, "ไม่พบปลั๊กอินนี้")
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// pluginRoute guards a plugin API route: 404 while the plugin is inactive,
// then authentication and the route's capabilities (any of them).
func (a *API) pluginRoute(rt plugin.Route) http.Handler {
	inner := a.protect("", func(w http.ResponseWriter, r *http.Request) {
		if !auth.From(r).CanAny(rt.Caps...) {
			writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
			return
		}
		rt.Handler(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.plugins.Active(rt.PluginID) {
			writeErr(w, http.StatusNotFound, "ปลั๊กอินนี้ยังไม่ได้เปิดใช้งาน")
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// collectionItems returns sample items of a plugin collection so the designer
// can preview collection blocks with real data.
func (a *API) collectionItems(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 6)
	if limit < 1 || limit > 50 {
		limit = 6
	}
	items := a.plugins.Collection(r.PathValue("source"), map[string]any{"limit": float64(limit)})
	if items == nil {
		items = []design.PostCtx{}
	}
	writeJSON(w, http.StatusOK, items)
}
