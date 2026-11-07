// Package db opens the MySQL connection and applies the schema.
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed schema.sql
var schema string

// Open connects to MySQL, retrying while the server starts up (docker compose).
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	var lastErr error
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			return db, nil
		}
		log.Printf("waiting for database: %v", lastErr)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("database not reachable: %w", lastErr)
}

// Migrate applies schema.sql (idempotent: CREATE TABLE IF NOT EXISTS).
func Migrate(db *sql.DB) error {
	for _, stmt := range SplitStatements(schema) {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w\n%s", err, stmt)
		}
	}
	// Additive column changes (MySQL 8 has no ADD COLUMN IF NOT EXISTS).
	cols := [][3]string{
		{"users", "locale", "VARCHAR(8) NOT NULL DEFAULT 'th'"},
		{"users", "first_name", "VARCHAR(191) NOT NULL DEFAULT ''"},
		{"users", "last_name", "VARCHAR(191) NOT NULL DEFAULT ''"},
		{"users", "bio", "TEXT NULL"},
		{"users", "website", "VARCHAR(512) NOT NULL DEFAULT ''"},
		{"users", "avatar", "VARCHAR(512) NOT NULL DEFAULT ''"},
		{"users", "status", "VARCHAR(16) NOT NULL DEFAULT 'active'"},
		{"users", "last_login_at", "DATETIME NULL"},
		// bumped by "sign out everywhere": ends every session and ticket (audit M-4)
		{"users", "session_epoch", "INT NOT NULL DEFAULT 0"},
		{"media", "downloads", "BIGINT NOT NULL DEFAULT 0"},
		{"posts", "pinned", "BOOLEAN NOT NULL DEFAULT FALSE"},
		{"posts", "sort_order", "INT NOT NULL DEFAULT 0"},
		// 2.1.0: hierarchical taxonomies (site.taxonomy)
		{"terms", "parent", "BIGINT NOT NULL DEFAULT 0"},
		{"terms", "description", "VARCHAR(500) NOT NULL DEFAULT ''"},
		// Site Code phase 4b: logs of JS plugins carry their unit id
		{"script_logs", "unit", "VARCHAR(64) NOT NULL DEFAULT 'site'"},
		// 2.3.0: unpublished designer changes of a published page (model.PostDraft)
		{"posts", "draft", "LONGTEXT NULL"},
		// 2.5.0: multilingual content — the post's language ("" = the site's) and the original it translates
		{"posts", "lang", "VARCHAR(10) NOT NULL DEFAULT ''"},
		{"posts", "translation_of", "BIGINT NOT NULL DEFAULT 0"},
	}
	for _, c := range cols {
		if err := EnsureColumn(db, c[0], c[1], c[2]); err != nil {
			return err
		}
	}
	// Site Code manifests grew in phase 3 (collections, actions, pages): TEXT → MEDIUMTEXT.
	var typ string
	if err := db.QueryRow(`SELECT DATA_TYPE FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'script_versions' AND COLUMN_NAME = 'manifest'`).Scan(&typ); err == nil && typ == "text" {
		if _, err := db.Exec("ALTER TABLE script_versions MODIFY manifest MEDIUMTEXT NULL"); err != nil {
			return fmt.Errorf("migrate: widen script_versions.manifest: %w", err)
		}
	}
	return nil
}

// EnsureColumn adds table.column with the given definition when it is
// missing, so schema upgrades stay idempotent and never touch existing rows.
func EnsureColumn(db *sql.DB, table, column, ddl string) error {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&n)
	if err != nil || n > 0 {
		return err
	}
	if _, err := db.Exec("ALTER TABLE `" + table + "` ADD COLUMN `" + column + "` " + ddl); err != nil {
		return fmt.Errorf("migrate: add %s.%s: %w", table, column, err)
	}
	return nil
}

// SplitStatements splits an SQL script into statements at the semicolons
// that end them: a ";" inside a quoted string ('…', "…", `…`) or a comment
// (-- …, # …, /* … */) does not split. Comments are dropped and empty
// statements skipped.
func SplitStatements(script string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for i := 0; i < len(script); i++ {
		c := script[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			// a quoted string or identifier; backslash escapes and doubled quotes stay inside
			j := i + 1
			for j < len(script) {
				if script[j] == '\\' && c != '`' {
					j += 2
					continue
				}
				if script[j] == c {
					if j+1 < len(script) && script[j+1] == c {
						j += 2
						continue
					}
					break
				}
				j++
			}
			end := min(j+1, len(script))
			cur.WriteString(script[i:end])
			i = end - 1
		case c == '#' || (c == '-' && strings.HasPrefix(script[i:], "--") &&
			(i+2 == len(script) || script[i+2] == ' ' || script[i+2] == '\t' || script[i+2] == '\n' || script[i+2] == '\r')):
			j := strings.IndexByte(script[i:], '\n') // comment to the end of the line
			if j < 0 {
				i = len(script)
			} else {
				i += j - 1 // the newline itself is kept
			}
		case c == '/' && strings.HasPrefix(script[i:], "/*"):
			j := strings.Index(script[i+2:], "*/")
			if j < 0 {
				i = len(script)
			} else {
				i += 2 + j + 1
			}
			cur.WriteByte(' ')
		case c == ';':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}
