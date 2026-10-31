package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Site Code admin API (/api/v1/code): draft files, upload/download, build,
// test runs, publish/versions, safe mode and logs. The runtime lives in
// internal/sitecode.

// SetSiteCode enables the Site Code endpoints (call before Routes).
func (a *API) SetSiteCode(e *sitecode.Engine) { a.code = e }

const msgCodeDemo = "ปิดในเว็บทดลอง"

func (a *API) codeRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1/code"
	// every endpoint but /status is off on the demo site
	c := func(pattern, cap string, fn http.HandlerFunc) {
		h(pattern, cap, func(w http.ResponseWriter, r *http.Request) {
			if a.code.Demo() {
				writeErr(w, http.StatusForbidden, msgCodeDemo)
				return
			}
			fn(w, r)
		})
	}
	h("GET "+p+"/status", auth.CapViewScripts, a.codeStatus)
	c("GET "+p+"/files", auth.CapViewScripts, a.codeListFiles)
	c("POST "+p+"/files/move", auth.CapEditScripts, a.codeMove)
	c("GET "+p+"/files/{path...}", auth.CapViewScripts, a.codeGetFile)
	c("PUT "+p+"/files/{path...}", auth.CapEditScripts, a.codePutFile)
	c("DELETE "+p+"/files/{path...}", auth.CapEditScripts, a.codeDeleteFile)
	c("POST "+p+"/upload", auth.CapEditScripts, a.codeUpload)
	c("GET "+p+"/download", auth.CapViewScripts, a.codeDownload)
	c("POST "+p+"/build", auth.CapEditScripts, a.codeBuild)
	c("POST "+p+"/run", auth.CapEditScripts, a.codeRun)
	c("POST "+p+"/publish", auth.CapPublishScripts, a.codePublish)
	c("GET "+p+"/versions", auth.CapViewScripts, a.codeVersions)
	c("GET "+p+"/versions/{id}", auth.CapViewScripts, a.codeVersion)
	c("GET "+p+"/versions/{id}/files/{path...}", auth.CapViewScripts, a.codeVersionFile)
	c("GET "+p+"/versions/{id}/download", auth.CapViewScripts, a.codeVersionDownload)
	c("POST "+p+"/versions/{id}/restore", auth.CapPublishScripts, a.codeRestore)
	c("POST "+p+"/safe-mode", auth.CapPublishScripts, a.codeSafeMode)
	c("GET "+p+"/logs", auth.CapViewScripts, a.codeLogs)
	c("GET "+p+"/logs/download", auth.CapViewScripts, a.codeLogsDownload)
	c("DELETE "+p+"/logs", auth.CapManageScripts, a.codeClearLogs)
	c("GET "+p+"/types", auth.CapViewScripts, a.codeTypes)
	c("GET "+p+"/hooks", auth.CapViewScripts, a.codeHooks)
	c("POST "+p+"/package", auth.CapPublishScripts, a.codePackage)
	a.codeDBRoutes(h, c)
	a.codeBindRoutes(h, c)
	a.codePhase4Routes(h, c)
	a.codePhase5Routes(c)
	a.codePhase7Routes(h)
}

// codeErr answers a sitecode validation error (400) or a server error.
func codeErr(w http.ResponseWriter, err error) {
	var pe *sitecode.PathError
	switch {
	case errors.As(err, &pe):
		writeErr(w, http.StatusBadRequest, pe.Msg)
	case sitecode.IsNotFound(err):
		writeErr(w, http.StatusNotFound, "ไม่พบไฟล์หรือเวอร์ชันที่ต้องการ")
	default:
		serverErr(w, err)
	}
}

func (a *API) codeAudit(r *http.Request, action string, detail any) {
	var s string
	switch d := detail.(type) {
	case string:
		s = d
	default:
		b, _ := json.Marshal(d)
		s = string(b)
	}
	if err := a.code.DB.Audit(auth.From(r).ID, action, s); err != nil {
		log.Printf("site code audit: %v", err)
	}
}

func (a *API) codeStatus(w http.ResponseWriter, r *http.Request) {
	s, err := a.code.StatusOf(r.URL.Query().Get("unit"))
	if sitecode.IsNotFound(err) {
		writeErr(w, http.StatusNotFound, "ไม่พบหน่วยโค้ด (ปลั๊กอิน) นี้")
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// ---------------------------------------------------------------- files

func (a *API) codeListFiles(w http.ResponseWriter, r *http.Request) {
	files, err := a.code.DB.ListFiles()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (a *API) codeGetFile(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := sitecode.ValidPath(p); err != nil {
		codeErr(w, err)
		return
	}
	f, err := a.code.DB.GetFile(p)
	if err != nil {
		codeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *API) codePutFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content       *string `json:"content"`
		BaseUpdatedAt string  `json:"baseUpdatedAt"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	p := r.PathValue("path")
	if err := sitecode.ValidPath(p); err != nil {
		codeErr(w, err)
		return
	}
	if in.Content == nil {
		writeErr(w, http.StatusBadRequest, "กรุณาส่ง content")
		return
	}
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	draft, err := a.code.DB.Draft()
	if err != nil {
		serverErr(w, err)
		return
	}
	if _, exists := draft[p]; exists {
		cur, err := a.code.DB.GetFile(p)
		if err != nil {
			serverErr(w, err)
			return
		}
		stale := in.BaseUpdatedAt == ""
		if !stale {
			base, err := time.Parse(time.RFC3339Nano, in.BaseUpdatedAt)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "baseUpdatedAt ต้องเป็นเวลาแบบ RFC3339")
				return
			}
			stale = base.Before(cur.UpdatedAt)
		}
		if stale {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "ไฟล์นี้ถูกแก้ไขโดยคนอื่นหลังจากที่คุณเปิด — โหลดใหม่หรือบันทึกทับ",
				"current": cur,
			})
			return
		}
	}
	draft[p] = *in.Content
	if err := sitecode.CheckSet(draft); err != nil {
		codeErr(w, err)
		return
	}
	f, err := a.code.DB.PutFile(p, *in.Content, auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "save", p)
	writeJSON(w, http.StatusOK, f)
}

// underDir returns the draft paths inside folder dir.
func underDir(draft map[string]string, dir string) []string {
	var out []string
	for _, p := range sitecode.Sorted(draft) {
		if dir == "" || strings.HasPrefix(p, dir+"/") {
			out = append(out, p)
		}
	}
	return out
}

func (a *API) codeDeleteFile(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	draft, err := a.code.DB.Draft()
	if err != nil {
		serverErr(w, err)
		return
	}
	var paths []string
	if r.URL.Query().Get("recursive") == "1" || strings.HasSuffix(p, "/") {
		dir, err := sitecode.CleanDir(p)
		if err != nil || dir == "" {
			writeErr(w, http.StatusBadRequest, "ระบุโฟลเดอร์ที่จะลบไม่ถูกต้อง")
			return
		}
		paths = underDir(draft, dir)
		if _, isFile := draft[dir]; isFile && r.URL.Query().Get("recursive") == "1" && !strings.HasSuffix(p, "/") {
			paths = append(paths, dir)
		}
	} else {
		if err := sitecode.ValidPath(p); err != nil {
			codeErr(w, err)
			return
		}
		if _, ok := draft[p]; ok {
			paths = []string{p}
		}
	}
	if len(paths) == 0 {
		writeErr(w, http.StatusNotFound, "ไม่พบไฟล์ที่ต้องการลบ")
		return
	}
	if err := a.code.DB.DeleteFiles(paths); err != nil {
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "delete", map[string]any{"paths": paths})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) codeMove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	draft, err := a.code.DB.Draft()
	if err != nil {
		serverErr(w, err)
		return
	}
	moves := map[string]string{}
	if _, isFile := draft[in.From]; isFile {
		if err := sitecode.ValidPath(in.To); err != nil {
			codeErr(w, err)
			return
		}
		moves[in.From] = in.To
	} else {
		from, err := sitecode.CleanDir(in.From)
		if err != nil || from == "" {
			writeErr(w, http.StatusNotFound, "ไม่พบไฟล์หรือโฟลเดอร์ที่จะย้าย")
			return
		}
		to, err := sitecode.CleanDir(in.To)
		if err != nil {
			codeErr(w, err)
			return
		}
		if to == from || strings.HasPrefix(to+"/", from+"/") {
			writeErr(w, http.StatusBadRequest, "ย้ายโฟลเดอร์เข้าไปในตัวเองไม่ได้")
			return
		}
		for _, p := range underDir(draft, from) {
			np := strings.TrimPrefix(p, from+"/")
			if to != "" {
				np = to + "/" + np
			}
			moves[p] = np
		}
		if len(moves) == 0 {
			writeErr(w, http.StatusNotFound, "ไม่พบไฟล์หรือโฟลเดอร์ที่จะย้าย")
			return
		}
	}
	next := map[string]string{}
	for p, c := range draft {
		if _, moving := moves[p]; !moving {
			next[p] = c
		}
	}
	for from, to := range moves {
		if _, taken := next[to]; taken {
			writeErr(w, http.StatusConflict, "มีไฟล์ชื่อนี้อยู่แล้ว: "+to)
			return
		}
		next[to] = draft[from]
	}
	if err := sitecode.CheckSet(next); err != nil {
		codeErr(w, err)
		return
	}
	if err := a.code.DB.MoveFiles(moves, auth.From(r).ID); err != nil {
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "move", in)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- upload / download

func (a *API) codeUpload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dryRun := q.Get("dryRun") == "1" || q.Get("dryRun") == "true"
	mode := q.Get("mode")
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		writeErr(w, http.StatusBadRequest, "mode ต้องเป็น merge หรือ replace")
		return
	}
	dir, err := sitecode.CleanDir(q.Get("dir"))
	if err != nil {
		codeErr(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, sitecode.MaxUpload+1<<20)
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("กรุณาแนบไฟล์ (.zip หรือไฟล์โค้ด ไม่เกิน %d MB)", sitecode.MaxUpload>>20))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, sitecode.MaxUpload+1))
	if err != nil || len(data) > sitecode.MaxUpload {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("ไฟล์ใหญ่เกิน %d MB", sitecode.MaxUpload>>20))
		return
	}
	incoming := map[string]string{}
	replace := false
	if strings.EqualFold(path.Ext(fh.Filename), ".zip") {
		files, err := sitecode.ReadZip(data)
		if err != nil {
			codeErr(w, err)
			return
		}
		for p, c := range files {
			if dir != "" {
				p = dir + "/" + p
			}
			incoming[p] = c
		}
		replace = mode == "replace"
	} else {
		name := path.Base(strings.ReplaceAll(fh.Filename, `\`, "/"))
		p := name
		if dir != "" {
			p = dir + "/" + name
		}
		if err := sitecode.ValidPath(p); err != nil {
			codeErr(w, err)
			return
		}
		if len(data) > sitecode.MaxFileBytes {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("ไฟล์ %s ใหญ่เกิน %d KB", p, sitecode.MaxFileBytes>>10))
			return
		}
		incoming[p] = string(data)
	}
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	draft, err := a.code.DB.Draft()
	if err != nil {
		serverErr(w, err)
		return
	}
	diff, result := sitecode.ComputeDiff(draft, incoming, replace, dir)
	if err := sitecode.CheckSet(result); err != nil {
		codeErr(w, err)
		return
	}
	if !dryRun {
		write := map[string]string{}
		for _, p := range append(append([]string{}, diff.Added...), diff.Changed...) {
			write[p] = incoming[p]
		}
		if err := a.code.DB.ApplyDraft(write, diff.Removed, auth.From(r).ID); err != nil {
			serverErr(w, err)
			return
		}
		a.codeAudit(r, "upload", map[string]any{"file": fh.Filename, "mode": mode, "dir": dir,
			"added": diff.Added, "changed": diff.Changed, "removed": diff.Removed})
	}
	writeJSON(w, http.StatusOK, diff)
}

// sendZip writes files as a zip attachment.
func sendZip(w http.ResponseWriter, name string, files map[string]string, prefix string) {
	var buf bytes.Buffer
	if err := sitecode.WriteZip(&buf, files, prefix); err != nil {
		serverErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", ContentDisposition(name))
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// withTypes adds anasaki.d.ts to a project download.
func withTypes(files map[string]string, types string) map[string]string {
	out := make(map[string]string, len(files)+1)
	for p, c := range files {
		out[p] = c
	}
	if _, ok := out[sitecode.TypesFile]; !ok {
		out[sitecode.TypesFile] = types
	}
	return out
}

func fileType(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".json":
		return "application/json; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

func (a *API) codeDownload(w http.ResponseWriter, r *http.Request) {
	draft, err := a.code.DB.Draft()
	if err != nil {
		serverErr(w, err)
		return
	}
	p := strings.TrimSuffix(r.URL.Query().Get("path"), "/")
	if p == "" {
		sendZip(w, "site-code.zip", withTypes(draft, a.code.TypesFile()), "")
		return
	}
	if c, ok := draft[p]; ok {
		w.Header().Set("Content-Type", fileType(p))
		w.Header().Set("Content-Disposition", ContentDisposition(path.Base(p)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, c)
		return
	}
	dir, err := sitecode.CleanDir(p)
	if err != nil {
		codeErr(w, err)
		return
	}
	files := map[string]string{}
	for _, fp := range underDir(draft, dir) {
		files[strings.TrimPrefix(fp, dir+"/")] = draft[fp]
	}
	if len(files) == 0 {
		writeErr(w, http.StatusNotFound, "ไม่พบไฟล์หรือโฟลเดอร์ที่ต้องการ")
		return
	}
	sendZip(w, path.Base(dir)+".zip", files, path.Base(dir)+"/")
}

// ---------------------------------------------------------------- build / run / publish

func (a *API) codeBuild(w http.ResponseWriter, r *http.Request) {
	out, _, err := a.code.BuildDraft()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) codeRun(w http.ResponseWriter, r *http.Request) {
	var in sitecode.RunRequest
	if !readJSON(w, r, &in) {
		return
	}
	u, err := a.st.UserByID(auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	res, err := a.code.Run(in, sitecode.UserFromModel(u))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// publishErr maps Publish/Restore failures.
func (a *API) publishErr(w http.ResponseWriter, err error) bool {
	var bf *sitecode.BuildFailed
	var dr *sitecode.DestructiveRefused
	var mf *sitecode.MigrationFailed
	switch {
	case err == nil:
		return false
	case errors.Is(err, sitecode.ErrConflict):
		live, _ := a.code.LiveVersion()
		var lv any
		if live > 0 {
			lv = live
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       "มีคนเผยแพร่เวอร์ชันใหม่ไปก่อนแล้ว — โหลดสถานะใหม่แล้วลองอีกครั้ง",
			"liveVersion": lv,
		})
	case errors.As(err, &bf):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "เผยแพร่ไม่สำเร็จ: โค้ดมีข้อผิดพลาด (เว็บยังใช้เวอร์ชันเดิม)",
			"errors": bf.Errors,
		})
	case errors.As(err, &dr):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":       "มี migration ที่ทำให้ข้อมูลหาย (ลบ/เปลี่ยนตารางหรือคอลัมน์) — ยืนยันด้วย confirmDestructive แล้วระบบจะสำรองตารางให้ก่อน",
			"destructive": dr.List,
		})
	case errors.As(err, &mf):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":     "เผยแพร่ไม่สำเร็จ: " + mf.Error() + " (เว็บยังใช้เวอร์ชันเดิม; migration ก่อนหน้าในรอบนี้ที่สำเร็จแล้วยังคงอยู่)",
			"migration": mf.Migration,
			"errors":    mf.Errors,
		})
	default:
		codeErr(w, err)
	}
	return true
}

func (a *API) codePublish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Message            string `json:"message"`
		BaseVersion        *int64 `json:"baseVersion"`
		ConfirmDestructive bool   `json:"confirmDestructive"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	v, err := a.code.Publish(in.Message, in.BaseVersion, auth.From(r).ID, in.ConfirmDestructive)
	if a.publishErr(w, err) {
		return
	}
	a.codeAudit(r, "publish", map[string]any{"version": v.ID, "message": v.Message})
	writeJSON(w, http.StatusOK, map[string]any{"version": v})
}

// ---------------------------------------------------------------- versions

func (a *API) codeVersions(w http.ResponseWriter, r *http.Request) {
	live, err := a.code.LiveVersion()
	if err != nil {
		serverErr(w, err)
		return
	}
	vs, err := a.code.DB.ListVersions(live)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

func (a *API) loadVersion(w http.ResponseWriter, r *http.Request) *sitecode.VersionRow {
	live, err := a.code.LiveVersion()
	if err != nil {
		serverErr(w, err)
		return nil
	}
	v, err := a.code.DB.GetVersion(pathID(r, "id"), live, false)
	if err != nil {
		codeErr(w, err)
		return nil
	}
	return v
}

func (a *API) codeVersion(w http.ResponseWriter, r *http.Request) {
	v := a.loadVersion(w, r)
	if v == nil {
		return
	}
	type fileRef struct {
		Path string `json:"path"`
		Size int    `json:"size"`
	}
	files := []fileRef{}
	for _, p := range sitecode.Sorted(v.FileMap) {
		files = append(files, fileRef{p, len(v.FileMap[p])})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": v.ID, "message": v.Message, "createdAt": v.CreatedAt,
		"createdBy": v.CreatedBy, "live": v.Live, "files": files, "registered": v.Manifest})
}

func (a *API) codeVersionFile(w http.ResponseWriter, r *http.Request) {
	v := a.loadVersion(w, r)
	if v == nil {
		return
	}
	p := r.PathValue("path")
	c, ok := v.FileMap[p]
	if !ok {
		writeErr(w, http.StatusNotFound, "ไม่พบไฟล์ในเวอร์ชันนี้")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": p, "content": c})
}

func (a *API) codeVersionDownload(w http.ResponseWriter, r *http.Request) {
	v := a.loadVersion(w, r)
	if v == nil {
		return
	}
	sendZip(w, fmt.Sprintf("site-code-v%d.zip", v.ID), withTypes(v.FileMap, a.code.TypesFile()), "")
}

func (a *API) codeRestore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AlsoDraft bool `json:"alsoDraft"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &in) {
		return
	}
	id := pathID(r, "id")
	if in.AlsoDraft {
		a.codeMu.Lock()
		defer a.codeMu.Unlock()
	}
	v, err := a.code.Restore(id, in.AlsoDraft, auth.From(r).ID)
	if a.publishErr(w, err) {
		return
	}
	a.codeAudit(r, "restore", map[string]any{"from": id, "version": v.ID, "alsoDraft": in.AlsoDraft})
	writeJSON(w, http.StatusOK, map[string]any{"version": v})
}

func (a *API) codeSafeMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := a.code.SetSafeMode(in.Enabled); err != nil {
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "safe_mode", map[string]bool{"enabled": in.Enabled})
	state := "ปิด"
	if in.Enabled {
		state = "เปิด"
	}
	a.code.Log("warn", "safe_mode", state+" safe mode โดย "+auth.From(r).Name, 0)
	a.codeStatus(w, r)
}

// ---------------------------------------------------------------- logs

var logLevels = map[string]bool{"": true, "debug": true, "info": true, "warn": true, "error": true}

// parseDay accepts YYYY-MM-DD or RFC3339; a day used as an upper bound
// includes the whole day.
func parseDay(s string, end bool) (time.Time, bool) {
	if s == "" {
		return time.Time{}, true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), true
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	if end {
		t = t.AddDate(0, 0, 1)
	}
	return t, true
}

func (a *API) logFilter(w http.ResponseWriter, r *http.Request) (sitecode.LogFilter, bool) {
	q := r.URL.Query()
	f := sitecode.LogFilter{Level: q.Get("level"), Source: strings.TrimSpace(q.Get("source")), Q: strings.TrimSpace(q.Get("q")),
		Unit: strings.TrimSpace(q.Get("unit"))}
	if !logLevels[f.Level] {
		writeErr(w, http.StatusBadRequest, "level ต้องเป็น debug, info, warn หรือ error")
		return f, false
	}
	var ok1, ok2 bool
	f.From, ok1 = parseDay(q.Get("from"), false)
	f.To, ok2 = parseDay(q.Get("to"), true)
	if !ok1 || !ok2 {
		writeErr(w, http.StatusBadRequest, "รูปแบบวันที่ต้องเป็น YYYY-MM-DD")
		return f, false
	}
	return f, true
}

func (a *API) codeLogs(w http.ResponseWriter, r *http.Request) {
	f, ok := a.logFilter(w, r)
	if !ok {
		return
	}
	per := queryInt(r, "perPage", 50)
	if per < 1 || per > 200 {
		per = 50
	}
	page := max(queryInt(r, "page", 1), 1)
	f.Limit, f.Offset = per, (page-1)*per
	items, total, err := a.code.DB.Logs(f)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) codeLogsDownload(w http.ResponseWriter, r *http.Request) {
	f, ok := a.logFilter(w, r)
	if !ok {
		return
	}
	f.Limit = 10000
	items, _, err := a.code.DB.Logs(f)
	if err != nil {
		serverErr(w, err)
		return
	}
	var b strings.Builder
	for i := len(items) - 1; i >= 0; i-- { // oldest first
		e := items[i]
		fmt.Fprintf(&b, "%s [%s] %s: %s\n", e.At.UTC().Format("2006-01-02 15:04:05.000"), e.Level, e.Source, e.Message)
		if e.Trace != nil && *e.Trace != "" {
			b.WriteString(*e.Trace + "\n")
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", ContentDisposition("site-code-logs.txt"))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, b.String())
}

func (a *API) codeClearLogs(w http.ResponseWriter, r *http.Request) {
	if err := a.code.DB.ClearLogs(); err != nil {
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "clear_logs", "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) codeTypes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="anasaki.d.ts"`)
	_, _ = io.WriteString(w, a.code.TypesFile())
}
