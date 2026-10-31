package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// platformRoutes registers the plugin-platform endpoints (built-in plugin
// settings, package plugins, designer blocks, plugin store client). Owned by
// the plugin platform; kept separate from the core routes in api.go.
func (a *API) platformRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1"
	h("GET "+p+"/plugin-settings/{id}", auth.CapManageOptions, a.getPluginSettings)
	h("PUT "+p+"/plugin-settings/{id}", auth.CapManageOptions, a.putPluginSettings)

	h("GET "+p+"/package-plugins", auth.CapManageOptions, a.listPackages)
	h("POST "+p+"/package-plugins/upload", auth.CapManageOptions, a.uploadPackage)
	h("POST "+p+"/package-plugins/install", auth.CapManageOptions, a.installPackage)
	h("POST "+p+"/package-plugins/{id}/activate", auth.CapManageOptions, a.activatePackage)
	h("POST "+p+"/package-plugins/{id}/deactivate", auth.CapManageOptions, a.deactivatePackage)
	h("DELETE "+p+"/package-plugins/{id}", auth.CapManageOptions, a.deletePackage)
	h("PUT "+p+"/package-plugins/{id}/settings", auth.CapManageOptions, a.packageSettings)

	h("GET "+p+"/blocks", auth.CapEditPages, a.listBlocks)
	h("GET "+p+"/design/assets", auth.CapEditPages, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, design.AssetCatalog()) // built-in icons and shapes for the designer
	})
	h("POST "+p+"/blocks/render", auth.CapEditPages, a.renderBlock)

	h("GET "+p+"/store/categories", auth.CapManageOptions, a.storeProxy("/api/v1/categories", plugin.StoreRaw))
	h("GET "+p+"/store/plugins", auth.CapManageOptions, a.storeProxy("/api/v1/plugins", plugin.StoreList))
	h("GET "+p+"/store/plugins/{id}", auth.CapManageOptions, a.storeProxy("/api/v1/plugins/{id}", plugin.StoreDetail))
	h("GET "+p+"/store/rankings", auth.CapManageOptions, a.storeProxy("/api/v1/rankings", plugin.StoreRankings))
	h("GET "+p+"/store/plugins/{id}/reviews", auth.CapManageOptions, a.storeProxy("/api/v1/plugins/{id}/reviews", plugin.StoreRaw))
	h("POST "+p+"/store/plugins/{id}/reviews", auth.CapManageOptions, a.storeReview)
}

// platformErr maps plugin-platform errors to HTTP answers.
func platformErr(w http.ResponseWriter, err error) {
	if packageErr(w, err) {
		return
	}
	var verr plugin.ValidationError
	var perr *plugin.PackageError
	var serr *plugin.StoreError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ค่าการตั้งค่าไม่ถูกต้อง", "fields": verr})
	case errors.As(err, &perr):
		writeErr(w, http.StatusBadRequest, "แพ็กเกจปลั๊กอินไม่ถูกต้อง: "+perr.Msg)
	case errors.Is(err, plugin.ErrUnknown), errors.Is(err, plugin.ErrUnknownPackage):
		writeErr(w, http.StatusNotFound, "ไม่พบปลั๊กอินนี้")
	case errors.As(err, &serr):
		if json.Valid(serr.Body) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(serr.Status)
			_, _ = w.Write(serr.Body)
			return
		}
		writeErr(w, serr.Status, "Plugin Store ตอบกลับข้อผิดพลาด")
	case errors.Is(err, plugin.ErrStoreUnavailable):
		writeErr(w, http.StatusBadGateway, "ไม่สามารถเชื่อมต่อ Plugin Store ได้ กรุณาลองใหม่ภายหลัง")
	default:
		serverErr(w, err)
	}
}

// ---------------------------------------------------------------- built-in settings

func (a *API) getPluginSettings(w http.ResponseWriter, r *http.Request) {
	v, err := a.plugins.PluginSettings(r.PathValue("id"))
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) putPluginSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Values map[string]any `json:"values"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	v, err := a.plugins.SetPluginSettings(r.PathValue("id"), in.Values)
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ---------------------------------------------------------------- package plugins

func (a *API) listPackages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.plugins.ListPackages())
}

func (a *API) uploadPackage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, plugin.MaxPackageSize+(1<<20))
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "กรุณาแนบไฟล์ .zip ของปลั๊กอิน (ขนาดไม่เกิน 20 MB)")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, plugin.MaxPackageSize+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "อ่านไฟล์ไม่สำเร็จ")
		return
	}
	inst, err := a.plugins.InstallPackageWith(data, "upload", "", plugin.InstallOptions{ConfirmDestructive: flag(r, "confirmDestructive")})
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

func (a *API) installPackage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID                 string `json:"id"`
		Version            string `json:"version"`
		ConfirmDestructive bool   `json:"confirmDestructive"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	inst, err := a.plugins.InstallFromStoreWith(in.ID, in.Version, plugin.InstallOptions{ConfirmDestructive: in.ConfirmDestructive || flag(r, "confirmDestructive")})
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

func (a *API) activatePackage(w http.ResponseWriter, r *http.Request) {
	a.setPackageActive(w, r, true)
}

func (a *API) deactivatePackage(w http.ResponseWriter, r *http.Request) {
	a.setPackageActive(w, r, false)
}

func (a *API) setPackageActive(w http.ResponseWriter, r *http.Request, on bool) {
	opts := plugin.ActiveOptions{ConfirmDestructive: flag(r, "confirmDestructive"), Force: flag(r, "force")}
	if r.ContentLength > 0 {
		var in struct {
			ConfirmDestructive bool `json:"confirmDestructive"`
			Force              bool `json:"force"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		opts.ConfirmDestructive = opts.ConfirmDestructive || in.ConfirmDestructive
		opts.Force = opts.Force || in.Force
	}
	inst, err := a.plugins.SetPackageActiveWith(r.PathValue("id"), on, opts)
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

func (a *API) deletePackage(w http.ResponseWriter, r *http.Request) {
	if err := a.plugins.DeletePackageWith(r.PathValue("id"), plugin.DeleteOptions{Force: flag(r, "force"), DeleteData: flag(r, "deleteData")}); err != nil {
		platformErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) packageSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Values map[string]any `json:"values"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	inst, err := a.plugins.SetPackageSettings(r.PathValue("id"), in.Values)
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

// ---------------------------------------------------------------- designer blocks

func (a *API) listBlocks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.plugins.Blocks())
}

// renderBlock previews a plugin block for the designer canvas. html is the
// wrapped block (<div class="pb-<plugin>-<block>">…</div>); css its scoped CSS.
func (a *API) renderBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Plugin string         `json:"plugin"`
		Block  string         `json:"block"`
		Values map[string]any `json:"values"`
		PostID int64          `json:"postId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Values == nil {
		in.Values = map[string]any{}
	}
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	call := design.BlockCall{Plugin: in.Plugin, Block: in.Block, Values: in.Values,
		Site: map[string]string{"title": opts["site_title"], "tagline": opts["tagline"], "url": "/"}}
	inner, css, ok := a.plugins.RenderBlock(call)
	if !ok {
		writeErr(w, http.StatusNotFound, "ไม่พบบล็อกนี้ หรือปลั๊กอินยังไม่ได้เปิดใช้งาน")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"html": `<div class="` + plugin.BlockClass(in.Plugin, in.Block) + `">` + inner + `</div>`,
		"css":  css,
	})
}

// ---------------------------------------------------------------- plugin store

// storeProxy forwards a GET to the plugin store ({id} is filled from the
// request path) and augments summaries with the local install state.
func (a *API) storeProxy(path, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := path
		if id := r.PathValue("id"); id != "" {
			p = replaceID(p, id)
		}
		q := url.Values{}
		for _, k := range []string{"search", "category", "sort", "page", "perPage", "period"} {
			if v := r.URL.Query().Get(k); v != "" {
				q.Set(k, v)
			}
		}
		v, err := a.plugins.StoreJSON(http.MethodGet, p, q, nil, kind, nil)
		if err != nil {
			platformErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (a *API) storeReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stars   int    `json:"stars"`
		Comment string `json:"comment"`
		Author  string `json:"author"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Stars < 1 || in.Stars > 5 {
		writeErr(w, http.StatusBadRequest, "กรุณาให้คะแนน 1–5 ดาว")
		return
	}
	if in.Author == "" {
		in.Author = auth.From(r).Name
	}
	body, _ := json.Marshal(in)
	hdr := http.Header{"X-Forwarded-For": {plugin.ClientIP(r)}}
	v, err := a.plugins.StoreJSON(http.MethodPost, replaceID("/api/v1/plugins/{id}/reviews", r.PathValue("id")),
		nil, body, plugin.StoreRaw, hdr)
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func replaceID(p, id string) string { return strings.ReplaceAll(p, "{id}", url.PathEscape(id)) }
