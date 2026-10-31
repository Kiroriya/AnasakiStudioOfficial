package api

import (
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/backup"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// System tools (phase 7): "Export / import site" — the former backup
// plugin, now part of core (internal/backup).

func (a *API) toolRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1"
	h("GET "+p+"/tools/export", auth.CapManageOptions, a.toolsExport)
	h("POST "+p+"/tools/import", auth.CapManageOptions, a.toolsImport)
}

// toolsExport answers GET /api/v1/tools/export: the site as a backup zip
// (content, media, installed packages list, Site Code draft).
func (a *API) toolsExport(w http.ResponseWriter, r *http.Request) {
	d, err := backup.Collect(a.st)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, p := range a.plugins.ListPackages() {
		d.Packages = append(d.Packages, backup.Package{ID: p.ID, Name: p.Name, Version: p.Version, Active: p.Active,
			Source: p.Source, Settings: backup.PublicSettings(p.Settings)})
	}
	if a.code != nil {
		if files, err := a.code.DB.Draft(); err == nil && len(files) > 0 {
			d.SiteCode = files
		}
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+backup.FileName(time.Now())+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_ = backup.WriteZip(w, d, a.uploadDir)
}

// PackageImport is a package of an imported backup and its state here.
type PackageImport struct {
	backup.Package
	// Status: "installed" (same id here — left as it is), "missing" (install
	// it from the Store; its settings are in Settings).
	Status           string `json:"status"`
	InstalledVersion string `json:"installedVersion,omitempty"`
}

var backupPkgID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}$`)

// toolsImport answers POST /api/v1/tools/import (multipart "file").
func (a *API) toolsImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, backup.MaxImport)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "กรุณาแนบไฟล์สำรองข้อมูล (.zip)")
		return
	}
	defer file.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	me := auth.From(r)
	canCode := me.Can(auth.CapEditScripts)
	res, d, err := backup.ImportData(a.st, a.uploadDir, file, hdr, me.ID, me.Can(auth.CapUnfilteredHTML), canCode)
	if err != nil {
		if backup.IsUserError(err) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		serverErr(w, err)
		return
	}
	// packages: report, never install or overwrite (installing needs the
	// administrator's consent for each plugin's permissions)
	pkgs := []PackageImport{}
	seen := map[string]bool{}
	for _, p := range d.Packages {
		if len(pkgs) >= 200 || !backupPkgID.MatchString(p.ID) || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		p.Settings = backup.PublicSettings(p.Settings)
		pi := PackageImport{Package: p, Status: "missing"}
		if v := a.plugins.InstalledVersion(p.ID); v != "" {
			pi.Status, pi.InstalledVersion = "installed", v
		}
		pkgs = append(pkgs, pi)
	}
	// Site Code: add files that do not exist yet to the draft (edit_scripts)
	added, skipped := []string{}, 0
	if len(d.SiteCode) > 0 && a.code != nil && canCode {
		a.codeMu.Lock()
		draft, err := a.code.DB.Draft()
		if err == nil {
			write := map[string]string{}
			merged := map[string]string{}
			for k, v := range draft {
				merged[k] = v
			}
			for p, src := range d.SiteCode {
				if _, exists := draft[p]; exists || sitecode.ValidPath(p) != nil || len(src) > sitecode.MaxFileBytes {
					skipped++
					continue
				}
				write[p], merged[p] = src, src
			}
			if len(write) > 0 && sitecode.CheckSet(merged) == nil {
				if err = a.code.DB.ApplyDraft(write, nil, me.ID); err == nil {
					for p := range write {
						added = append(added, p)
					}
					sort.Strings(added)
					a.codeAudit(r, "import", map[string]any{"source": "backup", "added": added})
				}
			} else if len(write) > 0 {
				skipped += len(write)
			}
		}
		a.codeMu.Unlock()
		if err != nil {
			serverErr(w, err)
			return
		}
	} else if len(d.SiteCode) > 0 {
		skipped = len(d.SiteCode)
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": res, "packages": pkgs,
		"siteCode": map[string]any{"added": added, "skipped": skipped}})
}
