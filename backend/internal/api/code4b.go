package api

import (
	"errors"
	"net/http"

	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Site Code phase 4b endpoints:
//
//	GET  /api/v1/code/hooks                 view_scripts     [{name, kind, declaredBy, subscribers:[{unit, priority, file, line}]}]
//	POST /api/v1/code/package?dryRun=1      publish_scripts  {id, name, version, description?, scopes?, routes?, library?, exports?}
//	                                                          → {manifest, warnings:[{message, file?, line?}], fileName} (dry run) or the zip

func (a *API) codeHooks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.code.Hooks())
}

func (a *API) codePackage(w http.ResponseWriter, r *http.Request) {
	var in sitecode.PackageRequest
	if !readJSON(w, r, &in) {
		return
	}
	dry := r.URL.Query().Get("dryRun") == "1"
	res, err := a.code.ExportPackage(in, dry)
	if err != nil {
		var bad *sitecode.ErrBadPackage
		if errors.As(err, &bad) {
			writeErr(w, http.StatusBadRequest, bad.Msg)
			return
		}
		serverErr(w, err)
		return
	}
	if dry {
		writeJSON(w, http.StatusOK, res)
		return
	}
	a.codeAudit(r, "package", map[string]any{"id": in.ID, "version": in.Version})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", ContentDisposition(res.FileName))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(res.Zip)
}

// packageErr maps the JS plugin / dependency errors of the package API;
// false when err is something else (platformErr handles it).
func packageErr(w http.ResponseWriter, err error) bool {
	var re *plugin.RequiresError
	var de *plugin.DependentsError
	var dx *plugin.DestructiveError
	var ae *plugin.ActivationError
	switch {
	case errors.As(err, &re):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": re.Error(), "missing": re.Missing})
	case errors.As(err, &de):
		writeJSON(w, http.StatusConflict, map[string]any{"error": de.Error(), "dependents": de.Dependents})
	case errors.As(err, &dx):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": dx.Error(), "destructive": dx.List})
	case errors.As(err, &ae):
		probs := ae.Problems
		if probs == nil {
			probs = []plugin.CodeProblem{}
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": ae.Msg, "activationError": ae.Msg, "errors": probs})
	default:
		return false
	}
	return true
}

// flag reads a boolean option from the query (?x=1) or a form field.
func flag(r *http.Request, name string) bool {
	v := r.URL.Query().Get(name)
	if v == "" && r.MultipartForm != nil {
		if l := r.MultipartForm.Value[name]; len(l) > 0 {
			v = l[0]
		}
	}
	return v == "1" || v == "true"
}
