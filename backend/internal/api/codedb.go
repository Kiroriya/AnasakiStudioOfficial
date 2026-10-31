package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// The site's own database (/api/v1/code/db): status, tables and rows,
// CSV import/export, SQL dump, SQL console, migrations and backups.

const maxImportBytes = 10 << 20

func (a *API) codeDBRoutes(h, c func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1/code/db"
	view, edit := auth.CapViewCodeData, auth.CapEditCodeData
	h("GET "+p+"/status", view, a.dbStatus)
	c("GET "+p+"/tables", view, a.dbTables)
	c("GET "+p+"/tables/{t}", view, a.dbTable)
	c("GET "+p+"/tables/{t}/rows", view, a.dbRows)
	c("POST "+p+"/tables/{t}/rows", edit, a.dbInsertRow)
	c("PUT "+p+"/tables/{t}/rows/{pk}", edit, a.dbUpdateRow)
	c("DELETE "+p+"/tables/{t}/rows/{pk}", edit, a.dbDeleteRow)
	c("POST "+p+"/tables/{t}/import", edit, a.dbImport)
	c("GET "+p+"/tables/{t}/export", view, a.dbExport)
	c("GET "+p+"/dump", view, a.dbDump)
	c("POST "+p+"/sql", view, a.dbSQL)
	c("GET "+p+"/migrations", view, a.dbMigrations)
	c("POST "+p+"/migrations/{name}/down", edit, a.dbRollback)
	c("GET "+p+"/backups", view, a.dbBackups)
	c("GET "+p+"/backups/{id}/download", view, a.dbBackupDownload)
	c("DELETE "+p+"/backups/{id}", edit, a.dbBackupDelete)
}

func dbCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 60*time.Second)
}

// dbErr maps code-database errors to responses.
func (a *API) dbErr(w http.ResponseWriter, err error) {
	var bi *sitecode.BadInput
	var mf *sitecode.MigrationFailed
	switch {
	case errors.As(err, &bi):
		writeErr(w, http.StatusBadRequest, bi.Msg)
	case errors.Is(err, sitecode.ErrNotFound):
		writeErr(w, http.StatusNotFound, "ไม่พบตาราง แถว หรือรายการที่ต้องการ")
	case errors.Is(err, sitecode.ErrReadOnly):
		writeErr(w, http.StatusBadRequest, "view ของระบบและตาราง kv_docs แก้ไขจากหน้านี้ไม่ได้ (อ่านได้อย่างเดียว)")
	case errors.Is(err, sitecode.ErrNoPK):
		writeErr(w, http.StatusBadRequest, "ตารางนี้ไม่มี primary key คอลัมน์เดียว จึงแก้ไข/ลบทีละแถวไม่ได้")
	case errors.Is(err, sitecode.ErrFallbackSQL):
		writeErr(w, http.StatusBadRequest, "SQL console ใช้ได้เมื่อตั้งฐานข้อมูลแยก (CODE_DB_DSN) แล้วเท่านั้น")
	case errors.Is(err, sitecode.ErrNotLatest):
		writeErr(w, http.StatusConflict, "ย้อนได้เฉพาะ migration ในรอบ (batch) ล่าสุด")
	case errors.As(err, &mf):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": mf.Error(), "migration": mf.Migration, "errors": mf.Errors})
	case errors.Is(err, context.DeadlineExceeded):
		writeErr(w, http.StatusGatewayTimeout, "หมดเวลา")
	default:
		serverErr(w, err)
	}
}

func (a *API) dbStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := dbCtx(r)
	defer cancel()
	info, err := a.code.Data().Info(ctx)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) dbTables(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := dbCtx(r)
	defer cancel()
	list, err := a.code.Data().Tables(ctx)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) dbTable(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := dbCtx(r)
	defer cancel()
	t, err := a.code.Data().Describe(ctx, r.PathValue("t"))
	if err != nil {
		a.dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *API) dbRows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rq := sitecode.RowsQuery{Page: max(queryInt(r, "page", 1), 1), PerPage: queryInt(r, "perPage", 50), Q: q.Get("q"),
		Sort: q.Get("sort"), Dir: q.Get("dir"), Filters: map[string]string{}}
	if rq.PerPage < 1 || rq.PerPage > 200 {
		rq.PerPage = 50
	}
	for k, vs := range q {
		if strings.HasPrefix(k, "filter[") && strings.HasSuffix(k, "]") && len(vs) > 0 && vs[0] != "" {
			rq.Filters[k[7:len(k)-1]] = vs[0]
		}
	}
	ctx, cancel := dbCtx(r)
	defer cancel()
	page, err := a.code.Data().Rows(ctx, r.PathValue("t"), rq)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func readRowBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil || body == nil {
		writeErr(w, http.StatusBadRequest, "ข้อมูลต้องเป็น JSON object ของคอลัมน์และค่า")
		return nil, false
	}
	return body, true
}

func (a *API) dbInsertRow(w http.ResponseWriter, r *http.Request) {
	body, ok := readRowBody(w, r)
	if !ok {
		return
	}
	ctx, cancel := dbCtx(r)
	defer cancel()
	row, err := a.code.Data().InsertRow(ctx, r.PathValue("t"), body)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	a.codeAudit(r, "db_insert", map[string]any{"table": r.PathValue("t"), "row": row})
	writeJSON(w, http.StatusCreated, row)
}

func (a *API) dbUpdateRow(w http.ResponseWriter, r *http.Request) {
	body, ok := readRowBody(w, r)
	if !ok {
		return
	}
	ctx, cancel := dbCtx(r)
	defer cancel()
	row, err := a.code.Data().UpdateRow(ctx, r.PathValue("t"), r.PathValue("pk"), body)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	a.codeAudit(r, "db_update", map[string]any{"table": r.PathValue("t"), "pk": r.PathValue("pk"), "fields": body})
	writeJSON(w, http.StatusOK, row)
}

func (a *API) dbDeleteRow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := dbCtx(r)
	defer cancel()
	if err := a.code.Data().DeleteRow(ctx, r.PathValue("t"), r.PathValue("pk")); err != nil {
		a.dbErr(w, err)
		return
	}
	a.codeAudit(r, "db_delete", map[string]any{"table": r.PathValue("t"), "pk": r.PathValue("pk")})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) dbImport(w http.ResponseWriter, r *http.Request) {
	dryRun := r.URL.Query().Get("dryRun") == "1" || r.URL.Query().Get("dryRun") == "true"
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+1<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "กรุณาแนบไฟล์ CSV (ไม่เกิน 10 MB) ในช่อง file")
		return
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	res, err := a.code.Data().Import(ctx, r.PathValue("t"), f, dryRun)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	if !dryRun {
		a.codeAudit(r, "db_import", map[string]any{"table": r.PathValue("t"), "inserted": res.Inserted, "errors": len(res.Errors)})
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) dbExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeErr(w, http.StatusBadRequest, "format ต้องเป็น csv หรือ json")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	var buf bytes.Buffer
	if err := a.code.Data().Export(ctx, r.PathValue("t"), format, r.URL.Query().Get("q"), &buf); err != nil {
		a.dbErr(w, err)
		return
	}
	ct := "text/csv; charset=utf-8"
	if format == "json" {
		ct = "application/json; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", ContentDisposition(r.PathValue("t")+"."+format))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

func (a *API) dbDump(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	var buf bytes.Buffer
	if err := a.code.Data().Dump(ctx, &buf); err != nil {
		a.dbErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", ContentDisposition(a.code.Data().Database+"-"+time.Now().UTC().Format("20060102-150405")+".sql"))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

func (a *API) dbSQL(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.UseNumber()
	var in struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
		Write  bool   `json:"write"`
	}
	if err := dec.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ข้อมูลไม่ถูกต้อง")
		return
	}
	if in.Write && !auth.From(r).Can(auth.CapEditCodeData) {
		writeErr(w, http.StatusForbidden, "การเขียนข้อมูลผ่าน SQL ต้องมีสิทธิ์ edit_code_data")
		return
	}
	params := make([]any, len(in.Params))
	for i, p := range in.Params {
		switch x := p.(type) {
		case json.Number:
			params[i] = string(x)
		case map[string]any, []any:
			b, _ := json.Marshal(x)
			params[i] = string(b)
		default:
			params[i] = x
		}
	}
	res, err := a.code.Data().RunSQL(r.Context(), in.SQL, params, in.Write)
	if err != nil {
		if in.Write {
			a.codeAudit(r, "db_sql", map[string]any{"sql": in.SQL, "error": err.Error()})
		}
		a.dbErr(w, err)
		return
	}
	if in.Write {
		a.codeAudit(r, "db_sql", map[string]any{"sql": in.SQL, "affectedRows": res.AffectedRows})
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *API) dbMigrations(w http.ResponseWriter, r *http.Request) {
	list, err := a.code.Migrations()
	if err != nil {
		a.dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) dbRollback(w http.ResponseWriter, r *http.Request) {
	p := auth.From(r)
	if !p.Can(auth.CapPublishScripts) {
		writeErr(w, http.StatusForbidden, "การย้อน migration ต้องมีสิทธิ์ทั้ง edit_code_data และ publish_scripts")
		return
	}
	name := r.PathValue("name")
	if err := a.code.RollbackMigration(name, p.Name); err != nil {
		a.dbErr(w, err)
		return
	}
	a.codeAudit(r, "db_rollback", name)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) dbBackups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := dbCtx(r)
	defer cancel()
	list, err := a.code.Data().Backups(ctx)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func backupID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func (a *API) dbBackupDownload(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	var buf bytes.Buffer
	b, err := a.code.Data().DumpBackup(ctx, backupID(r), &buf)
	if err != nil {
		a.dbErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", ContentDisposition(b.BackupTable+".sql"))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

func (a *API) dbBackupDelete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := dbCtx(r)
	defer cancel()
	b, err := a.code.Data().DeleteBackup(ctx, backupID(r))
	if err != nil {
		a.dbErr(w, err)
		return
	}
	a.codeAudit(r, "db_backup_delete", fmt.Sprintf("%s (%s)", b.BackupTable, b.TableName))
	w.WriteHeader(http.StatusNoContent)
}
