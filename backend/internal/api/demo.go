package api

import (
	"net/http"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/demo"
)

// SetDemo enables the demo-mode guard rails (nil = normal site).
func (a *API) SetDemo(d *demo.Demo) { a.demo = d }

// visitor reports whether the request comes from a demo visitor.
func (a *API) visitor(r *http.Request) bool { return a.demo.IsVisitor(auth.From(r)) }

// demoDenied answers 403 with the demo-mode explanation.
func demoDenied(w http.ResponseWriter) {
	writeErr(w, http.StatusForbidden, "ปิดไว้ในเว็บทดลอง (demo) — This is disabled on the demo site")
}

func (a *API) demoSnapshot(w http.ResponseWriter, r *http.Request) {
	if a.demo == nil {
		writeErr(w, http.StatusNotFound, "demo mode is off")
		return
	}
	if err := a.demo.Snapshot(); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.demo.Status())
}

func (a *API) demoReset(w http.ResponseWriter, r *http.Request) {
	if a.demo == nil {
		writeErr(w, http.StatusNotFound, "demo mode is off")
		return
	}
	if err := a.demo.Reset(); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.demo.Status())
}
