package legacy

import (
	"database/sql"
	"fmt"
)

// OldSchema is the DDL the removed built-ins ran in their Migrate (copied
// verbatim). Core never creates these tables any more; the DDL documents the
// shapes ReadRows understands and lets tests build sites "from before the
// upgrade" (CreateOldTables).
var OldSchema = map[string][]string{
	"analytics": {
		`CREATE TABLE IF NOT EXISTS analytics_daily (
  day   DATE         NOT NULL,
  path  VARCHAR(191) NOT NULL,
  title VARCHAR(255) NOT NULL DEFAULT '',
  views INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (day, path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
		`CREATE TABLE IF NOT EXISTS analytics_visitors (
  day  DATE     NOT NULL,
  hash CHAR(16) NOT NULL,
  PRIMARY KEY (day, hash)
) ENGINE=InnoDB DEFAULT CHARSET=ascii`,
		`CREATE TABLE IF NOT EXISTS analytics_page_visitors (
  day  DATE         NOT NULL,
  path VARCHAR(191) NOT NULL,
  hash CHAR(16)     NOT NULL,
  PRIMARY KEY (day, path, hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
		`CREATE TABLE IF NOT EXISTS analytics_referrers (
  day   DATE         NOT NULL,
  host  VARCHAR(191) NOT NULL,
  views INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (day, host)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
	},
	"seo": {
		`CREATE TABLE IF NOT EXISTS seo_meta (
  post_id     BIGINT       NOT NULL PRIMARY KEY,
  title       VARCHAR(255) NOT NULL DEFAULT '',
  description TEXT         NULL,
  image       VARCHAR(512) NOT NULL DEFAULT '',
  noindex     BOOLEAN      NOT NULL DEFAULT FALSE,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	},
	"forms": {
		`CREATE TABLE IF NOT EXISTS forms_forms (
  id              BIGINT AUTO_INCREMENT PRIMARY KEY,
  name            VARCHAR(191)  NOT NULL,
  fields          JSON          NOT NULL,
  submit_label    VARCHAR(100)  NOT NULL DEFAULT '',
  success_message VARCHAR(1000) NOT NULL DEFAULT '',
  redirect_url    VARCHAR(512)  NOT NULL DEFAULT '',
  webhook_url     VARCHAR(512)  NOT NULL DEFAULT '',
  created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS forms_submissions (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  form_id    BIGINT   NOT NULL,
  data       JSON     NOT NULL,
  is_read    BOOLEAN  NOT NULL DEFAULT FALSE,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_forms_submissions_form (form_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	},
	"redirects": {
		`CREATE TABLE IF NOT EXISTS redirects_rules (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  from_path  VARCHAR(191) NOT NULL,
  to_url     VARCHAR(1000) NOT NULL,
  code       SMALLINT     NOT NULL DEFAULT 301,
  hits       INT          NOT NULL DEFAULT 0,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_redirects_from (from_path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
		`CREATE TABLE IF NOT EXISTS redirects_404 (
  path      VARCHAR(191) NOT NULL PRIMARY KEY,
  hits      INT          NOT NULL DEFAULT 0,
  last_seen DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
	},
}

// CreateOldTables creates built-in id's old tables (tests: a site from before
// the upgrade).
func CreateOldTables(db *sql.DB, id string) error {
	for _, s := range OldSchema[id] {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("legacy %s: %w", id, err)
		}
	}
	return nil
}
