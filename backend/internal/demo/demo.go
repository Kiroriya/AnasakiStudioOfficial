// Package demo turns an installation into a public playground (DEMO_MODE=true):
// a shared visitor account with a safe role, guard rails on everything that
// could lock others out or harm the server, and a periodic reset of the
// content back to a saved baseline.
//
// The baseline is a copy of the content tables kept in the same database
// (demo_snap_<table>); users, roles, installed plugins, password reset tokens
// and statistics are never part of it. Uploaded files newer than the
// baseline are deleted on reset.
package demo

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// Role is the visitors' role: content, design, media and categories — no
// settings, users, plugins or raw HTML.
const Role = "demo"

// RoleCaps are the demo role's capabilities.
var RoleCaps = []string{auth.CapRead, auth.CapEditOwnPosts, auth.CapEditOthersPosts, auth.CapPublishPosts,
	auth.CapEditPages, auth.CapUploadFiles, auth.CapManageTerms, auth.CapViewStats}

// Settings demo visitors may still change (the fun, harmless ones).
var VisitorSettings = map[string]bool{"theme": true, "site_title": true, "tagline": true, "site_logo": true, "site_icon": true}

// Upload limits for demo visitors.
const (
	MaxUploadBytes = 3 << 20
	UploadTypes    = "jpg,jpeg,png,gif,webp"
)

// UploadedFile reports whether s is a file in this site's media library
// ("/uploads/…"): the only images a demo visitor may use as the logo or
// icon (no other site's files, no tracking pixels).
func UploadedFile(s string) bool {
	if !strings.HasPrefix(s, "/uploads/") || len(s) > 512 || strings.Contains(s, "..") || strings.ContainsAny(s, "\\?#") {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// Config comes from the environment (see config.Config).
type Config struct {
	Enabled  bool
	Email    string
	Password string
	Every    time.Duration // 0 = no automatic reset
}

// Demo runs the playground.
type Demo struct {
	cfg       Config
	st        *store.Store
	uploadDir string

	mu       sync.Mutex
	snapAt   time.Time
	nextAt   time.Time
	resetErr error
}

// excluded tables are never snapshotted nor reset.
func excluded(t string) bool {
	switch t {
	case "users", "roles", "plugin_packages", "password_resets", "api_tokens", "legacy_migrations":
		return true
	}
	return strings.HasPrefix(t, "demo_") || strings.HasPrefix(t, "analytics_")
}

// New prepares demo mode: role, visitor account, baseline (taken now when
// none exists) and the reset timer. It returns nil when demo mode is off.
func New(cfg Config, st *store.Store, uploadDir string) (*Demo, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	d := &Demo{cfg: cfg, st: st, uploadDir: uploadDir} // demo_meta comes from schema.sql
	if err := d.ensureAccount(); err != nil {
		return nil, err
	}
	var at string
	if err := st.DB.QueryRow(`SELECT v FROM demo_meta WHERE k='snapshot_at'`).Scan(&at); err == nil {
		d.snapAt, _ = time.Parse(time.RFC3339, at)
	}
	if d.snapAt.IsZero() {
		if err := d.Snapshot(); err != nil {
			return nil, fmt.Errorf("demo: first baseline: %w", err)
		}
	}
	if cfg.Every > 0 {
		d.nextAt = time.Now().Add(cfg.Every).Truncate(time.Minute)
		go d.loop()
	}
	log.Printf("demo mode: visitor %s, reset every %s", cfg.Email, cfg.Every)
	return d, nil
}

// ensureAccount creates the demo role and the visitor account, and puts the
// visitor's password, role and status back to the configured values.
func (d *Demo) ensureAccount() error {
	if r, err := d.st.GetRole(Role); err == nil {
		r.Name, r.Caps = "ผู้ทดลองใช้ (Demo)", RoleCaps
		if err := d.st.UpdateRole(r); err != nil {
			return err
		}
	} else if err := d.st.CreateRole(&model.Role{Slug: Role, Name: "ผู้ทดลองใช้ (Demo)", Caps: RoleCaps}); err != nil {
		return err
	}
	if m, err := d.st.RoleCaps(); err == nil && len(m) > 0 {
		auth.SetRoles(m)
	}
	hash, err := auth.HashPassword(d.cfg.Password)
	if err != nil {
		return err
	}
	u, old, err := d.st.UserByEmail(d.cfg.Email)
	if err != nil {
		u = &model.User{Email: d.cfg.Email, Name: "ผู้ทดลองใช้", Role: Role, Status: model.UserActive, Locale: model.LocaleTH}
		return d.st.CreateUser(u, hash)
	}
	// back to a clean shared profile (the last visitor may have changed the language)
	u.Role, u.Status, u.Name, u.Locale = Role, model.UserActive, "ผู้ทดลองใช้", model.LocaleTH
	u.FirstName, u.LastName, u.Bio, u.Website, u.Avatar = "", "", "", "", ""
	if auth.CheckPassword(old, d.cfg.Password) {
		hash = "" // unchanged: keep visitors' sessions (a new hash would sign them out)
	}
	return d.st.UpdateUser(u, hash)
}

// IsVisitor reports whether p is the shared demo visitor (or any demo-role user).
func (d *Demo) IsVisitor(p *auth.Principal) bool {
	return d != nil && p != nil && p.Role == Role
}

// Status is shown on the login screen and in the admin banner.
type Status struct {
	Enabled      bool      `json:"enabled"`
	Email        string    `json:"email"`
	Password     string    `json:"password"`
	EveryMinutes int       `json:"everyMinutes"`
	NextReset    time.Time `json:"nextReset"`
	BaselineAt   time.Time `json:"baselineAt"`
}

// Status describes the demo for the UI.
func (d *Demo) Status() Status {
	if d == nil {
		return Status{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return Status{Enabled: true, Email: d.cfg.Email, Password: d.cfg.Password, EveryMinutes: int(d.cfg.Every / time.Minute),
		NextReset: d.nextAt, BaselineAt: d.snapAt}
}

func (d *Demo) loop() {
	for {
		d.mu.Lock()
		wait := time.Until(d.nextAt)
		d.mu.Unlock()
		time.Sleep(max(wait, time.Second))
		if err := d.Reset(); err != nil {
			log.Printf("demo reset: %v", err)
		}
	}
}

func (d *Demo) tables() ([]string, error) {
	rows, err := d.st.DB.Query(`SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil && !excluded(t) {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

// Snapshot saves the current content as the baseline that resets go back to.
// The new copies are built next to the old baseline (demo_new_<table>) and
// swapped in with one atomic RENAME TABLE, so a failure half-way keeps the
// previous baseline intact and a reset never sees a half-built one.
func (d *Demo) Snapshot() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tables, err := d.tables()
	if err != nil {
		return err
	}
	// leftovers of an interrupted snapshot
	for _, prefix := range []string{newPrefix, oldPrefix} {
		if err := d.dropPrefixed(prefix); err != nil {
			return err
		}
	}
	for _, t := range tables {
		tmp := newPrefix + t
		if _, err := d.st.DB.Exec("CREATE TABLE `" + tmp + "` LIKE `" + t + "`"); err == nil {
			_, err = d.st.DB.Exec("INSERT INTO `" + tmp + "` SELECT * FROM `" + t + "`")
		}
		if err != nil {
			_ = d.dropPrefixed(newPrefix)
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	old, err := d.prefixed(snapPrefix)
	if err != nil {
		_ = d.dropPrefixed(newPrefix)
		return err
	}
	var renames []string
	for _, s := range old { // the old baseline (also of tables that no longer exist) moves aside…
		renames = append(renames, "`"+s+"` TO `"+oldPrefix+strings.TrimPrefix(s, snapPrefix)+"`")
	}
	for _, t := range tables { // …and the new one takes its place, in the same statement
		renames = append(renames, "`"+newPrefix+t+"` TO `"+snapPrefix+t+"`")
	}
	if len(renames) > 0 {
		if _, err := d.st.DB.Exec("RENAME TABLE " + strings.Join(renames, ", ")); err != nil {
			_ = d.dropPrefixed(newPrefix)
			return err
		}
	}
	if err := d.dropPrefixed(oldPrefix); err != nil {
		log.Printf("demo: drop old baseline: %v", err) // harmless: dropped on the next snapshot
	}
	now := time.Now().UTC()
	if _, err := d.st.DB.Exec(`INSERT INTO demo_meta (k, v) VALUES ('snapshot_at', ?) ON DUPLICATE KEY UPDATE v = VALUES(v)`,
		now.Format(time.RFC3339)); err != nil {
		return err
	}
	d.snapAt = now
	log.Printf("demo: baseline saved (%d tables)", len(tables))
	return nil
}

// Table name prefixes of the baseline copies (all start with "demo_", so
// they are never snapshotted themselves).
const (
	snapPrefix = "demo_snap_"
	newPrefix  = "demo_new_"
	oldPrefix  = "demo_old_"
)

// prefixed lists the tables whose name starts with prefix.
func (d *Demo) prefixed(prefix string) ([]string, error) {
	like := strings.ReplaceAll(prefix, "_", `\_`) + "%"
	rows, err := d.st.DB.Query(`SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE ?`, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

func (d *Demo) dropPrefixed(prefix string) error {
	list, err := d.prefixed(prefix)
	if err != nil {
		return err
	}
	for _, t := range list {
		if _, err := d.st.DB.Exec("DROP TABLE IF EXISTS `" + t + "`"); err != nil {
			return err
		}
	}
	return nil
}

// Reset puts the content back to the baseline and the visitor account back
// to its configured password. All tables are restored in one transaction:
// if anything fails, the content stays exactly as it was.
func (d *Demo) Reset() error {
	d.mu.Lock()
	defer func() {
		if d.cfg.Every > 0 {
			d.nextAt = time.Now().Add(d.cfg.Every).Truncate(time.Minute)
		}
		d.mu.Unlock()
	}()
	tables, err := d.tables()
	if err != nil {
		return err
	}
	snaps, err := d.prefixed(snapPrefix)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, s := range snaps {
		have[strings.TrimPrefix(s, snapPrefix)] = true
	}
	ctx := contextBG()
	conn, err := d.st.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// the tables reference each other: rows go back in table order, so the
	// checks are off for this connection only, and on again before it is reused
	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1") //nolint:errcheck
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, t := range tables {
		if !have[t] {
			continue // a table created after the baseline (new plugin): left alone
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM `"+t+"`"); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO `"+t+"` SELECT * FROM `"+snapPrefix+t+"`"); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	d.st.InvalidateCache() // the options came back too
	d.removeNewUploads()
	if err := d.ensureAccount(); err != nil {
		return err
	}
	log.Print("demo: content reset to the baseline")
	return nil
}

// removeNewUploads deletes uploaded files created after the baseline.
func (d *Demo) removeNewUploads() {
	if d.snapAt.IsZero() || d.uploadDir == "" {
		return
	}
	n := 0
	_ = filepath.WalkDir(d.uploadDir, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return nil
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(d.snapAt) {
			if os.Remove(p) == nil {
				n++
			}
		}
		return nil
	})
	if n > 0 {
		log.Printf("demo: removed %d uploaded file(s)", n)
	}
}

// Minutes formats a duration in minutes (for logs).
func Minutes(d time.Duration) string { return strconv.Itoa(int(d/time.Minute)) + " min" }

func contextBG() context.Context { return context.Background() }
