package api

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Legacy migration of the former built-in plugins (phase 7): the banner
// state, Retry, the manual zip upload and "Clean up old data".

// SetLegacy attaches the migration task (before Routes).
func (a *API) SetLegacy(m *legacy.Migrator) { a.legacy = m }

// legacyWait bounds how long Retry / upload wait for the run they started.
var legacyWait = 10 * time.Second

func (a *API) legacyRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1/legacy-migrations"
	h("GET "+p, auth.CapManageOptions, a.legacyList)
	h("POST "+p+"/retry", auth.CapManageOptions, a.legacyRetry)
	h("POST "+p+"/upload", auth.CapManageOptions, a.legacyUpload)
	h("POST "+p+"/{id}/cleanup", auth.CapManageOptions, a.legacyCleanup)
}

func (a *API) writeLegacy(w http.ResponseWriter) {
	if a.legacy == nil {
		writeJSON(w, http.StatusOK, legacy.Summary{Items: []legacy.Item{}})
		return
	}
	s, err := a.legacy.List()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) legacyList(w http.ResponseWriter, r *http.Request) { a.writeLegacy(w) }

func waitFor(ch <-chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-time.After(legacyWait):
	}
}

// legacyRetry answers POST /legacy-migrations/retry {id?}: the waiting
// migrations (or one) run now; answers when done or after legacyWait.
func (a *API) legacyRetry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if r.ContentLength > 0 && !readJSON(w, r, &in) {
		return
	}
	if a.legacy == nil {
		writeErr(w, http.StatusNotFound, "ไม่มีปลั๊กอินที่รอย้าย")
		return
	}
	ch, err := a.legacy.Retry(in.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	waitFor(ch)
	a.writeLegacy(w)
}

// legacyUpload answers POST /legacy-migrations/upload (multipart "file"):
// the official zip of a waiting plugin, for sites without Store access.
func (a *API) legacyUpload(w http.ResponseWriter, r *http.Request) {
	if a.legacy == nil {
		writeErr(w, http.StatusNotFound, "ไม่มีปลั๊กอินที่รอย้าย")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, plugin.MaxPackageSize+(1<<20))
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "กรุณาแนบไฟล์ .zip ของปลั๊กอินทางการ")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, plugin.MaxPackageSize+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "อ่านไฟล์ไม่สำเร็จ")
		return
	}
	_, ch, err := a.legacy.AcceptZip(data)
	var pe *plugin.PackageError
	switch {
	case errors.As(err, &pe):
		writeErr(w, http.StatusBadRequest, "ไฟล์นี้ไม่ใช่ปลั๊กอินทางการที่รอย้าย: "+pe.Msg)
		return
	case errors.Is(err, legacy.ErrNotWaiting):
		writeErr(w, http.StatusBadRequest, "ปลั๊กอินนี้ไม่ได้รอย้ายข้อมูลอยู่")
		return
	case err != nil:
		serverErr(w, err)
		return
	}
	waitFor(ch)
	a.writeLegacy(w)
}

// legacyCleanup answers POST /legacy-migrations/{id}/cleanup {confirm: true}:
// drops the old tables and options of a migrated built-in.
func (a *API) legacyCleanup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !in.Confirm {
		writeErr(w, http.StatusBadRequest, "กรุณายืนยันการลบข้อมูลเก่า (confirm: true)")
		return
	}
	if a.legacy == nil {
		writeErr(w, http.StatusNotFound, "ไม่พบข้อมูลเก่าของปลั๊กอินนี้")
		return
	}
	err := a.legacy.Cleanup(r.PathValue("id"))
	switch {
	case errors.Is(err, legacy.ErrNotWaiting):
		writeErr(w, http.StatusNotFound, "ไม่พบข้อมูลเก่าของปลั๊กอินนี้")
		return
	case errors.Is(err, legacy.ErrNotDone):
		writeErr(w, http.StatusConflict, "ยังย้ายข้อมูลของปลั๊กอินนี้ไม่เสร็จ — ลบข้อมูลเก่าไม่ได้")
		return
	case err != nil:
		serverErr(w, err)
		return
	}
	a.writeLegacy(w)
}
