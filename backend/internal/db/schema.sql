-- Anasaki Studio schema. Modelled on WordPress' core tables (wp_users, wp_posts,
-- wp_terms/term_taxonomy/term_relationships, wp_options) but normalised and
-- extended with a `design` JSON column holding the Figma-like node tree.

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  email         VARCHAR(191) NOT NULL,
  name          VARCHAR(191) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role          VARCHAR(32)  NOT NULL DEFAULT 'author',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One table for every content type, like wp_posts: page | post | template
CREATE TABLE IF NOT EXISTS posts (
  id                BIGINT AUTO_INCREMENT PRIMARY KEY,
  type              VARCHAR(20)  NOT NULL,
  title             VARCHAR(255) NOT NULL,
  slug              VARCHAR(191) NOT NULL,
  status            VARCHAR(20)  NOT NULL DEFAULT 'draft',
  excerpt           TEXT         NULL,
  content           MEDIUMTEXT   NULL,
  design            JSON         NULL,
  featured_media_id BIGINT       NULL,
  author_id         BIGINT       NOT NULL,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  published_at      DATETIME     NULL,
  UNIQUE KEY uq_posts_type_slug (type, slug),
  KEY idx_posts_type_status (type, status, published_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS revisions (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  post_id    BIGINT       NOT NULL,
  title      VARCHAR(255) NOT NULL,
  content    MEDIUMTEXT   NULL,
  design     JSON         NULL,
  author_id  BIGINT       NOT NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_revisions_post (post_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- wp_terms + wp_term_taxonomy collapsed into one table
CREATE TABLE IF NOT EXISTS terms (
  id       BIGINT AUTO_INCREMENT PRIMARY KEY,
  taxonomy VARCHAR(32)  NOT NULL,
  name     VARCHAR(191) NOT NULL,
  slug     VARCHAR(191) NOT NULL,
  UNIQUE KEY uq_terms_tax_slug (taxonomy, slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS post_terms (
  post_id BIGINT NOT NULL,
  term_id BIGINT NOT NULL,
  PRIMARY KEY (post_id, term_id),
  KEY idx_post_terms_term (term_id)
) ENGINE=InnoDB;

-- Attachments live in their own table instead of wp_posts(type=attachment)
CREATE TABLE IF NOT EXISTS media (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  filename   VARCHAR(255) NOT NULL,
  url        VARCHAR(512) NOT NULL,
  mime       VARCHAR(100) NOT NULL,
  size       BIGINT       NOT NULL DEFAULT 0,
  width      INT          NOT NULL DEFAULT 0,
  height     INT          NOT NULL DEFAULT 0,
  alt        VARCHAR(255) NOT NULL DEFAULT '',
  author_id  BIGINT       NOT NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Navigation menus (WordPress stores these as nav_menu_item posts)
CREATE TABLE IF NOT EXISTS menus (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(191) NOT NULL,
  items      JSON         NOT NULL,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Figma "main components": reusable node trees placed via instance nodes
CREATE TABLE IF NOT EXISTS components (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(191) NOT NULL,
  node       JSON         NOT NULL,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Earlier versions of a component, kept on every save (like post revisions)
CREATE TABLE IF NOT EXISTS component_revisions (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  component_id BIGINT       NOT NULL,
  name         VARCHAR(191) NOT NULL,
  node         JSON         NOT NULL,
  author_id    BIGINT       NOT NULL DEFAULT 0,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_component_revisions (component_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Site-wide key/value settings, like wp_options
CREATE TABLE IF NOT EXISTS options (
  name  VARCHAR(191) NOT NULL PRIMARY KEY,
  value TEXT         NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Roles and their capabilities (WordPress keeps these in wp_options/wp_user_roles)
CREATE TABLE IF NOT EXISTS roles (
  slug    VARCHAR(32)  NOT NULL PRIMARY KEY,
  name    VARCHAR(191) NOT NULL,
  caps    JSON         NOT NULL,
  builtin BOOLEAN      NOT NULL DEFAULT FALSE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Designer comments (Figma comment mode): pinned to a layer (node_id) of a
-- page/template/post or a component. x/y are design px from the layer's
-- (or the artboard's) top-left. Replies set parent_id to the thread's root.
CREATE TABLE IF NOT EXISTS design_comments (
  id          BIGINT AUTO_INCREMENT PRIMARY KEY,
  target_kind VARCHAR(16)  NOT NULL,
  target_id   BIGINT       NOT NULL,
  node_id     VARCHAR(64)  NULL,
  x           DOUBLE       NOT NULL DEFAULT 0,
  y           DOUBLE       NOT NULL DEFAULT 0,
  body        TEXT         NOT NULL,
  author_id   BIGINT       NOT NULL,
  parent_id   BIGINT       NULL,
  resolved    BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_design_comments_target (target_kind, target_id, id),
  KEY idx_design_comments_parent (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One-time links for "forgot password" and the welcome e-mail's "set your
-- password" (only the SHA-256 of the token is stored).
CREATE TABLE IF NOT EXISTS password_resets (
  token_hash CHAR(64) NOT NULL PRIMARY KEY,
  user_id    BIGINT   NOT NULL,
  expires_at DATETIME NOT NULL,
  INDEX (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Session tokens revoked by a logout, kept until they would have expired
-- (audit M-4).
CREATE TABLE IF NOT EXISTS revoked_sessions (
  jti        CHAR(32) NOT NULL PRIMARY KEY,
  expires_at DATETIME NOT NULL,
  INDEX (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Demo mode (DEMO_MODE=true): when the baseline was saved, etc. The baseline
-- itself lives in demo_snap_<table> copies made at runtime.
CREATE TABLE IF NOT EXISTS demo_meta (
  k VARCHAR(64) NOT NULL PRIMARY KEY,
  v TEXT        NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Site Code (the site's own server-side JS/TS, see internal/sitecode).
-- script_files is the current draft; paths compare case-sensitively (the
-- application refuses names differing only by case).
CREATE TABLE IF NOT EXISTS script_files (
  path       VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
  content    MEDIUMTEXT   NOT NULL,
  updated_at DATETIME(3)  NOT NULL,
  updated_by BIGINT       NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One row per publish (the newest 100 are kept): the project snapshot
-- (files: JSON {path: content}), the bundle goja runs and the registered
-- routes/events (manifest JSON). Option site_code_version names the live one.
CREATE TABLE IF NOT EXISTS script_versions (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  message    VARCHAR(500) NOT NULL DEFAULT '',
  files      LONGTEXT     NOT NULL,
  bundle     LONGTEXT     NOT NULL,
  source_map LONGTEXT     NULL,
  manifest   MEDIUMTEXT   NULL,
  created_by BIGINT       NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Runtime log of Site Code (log.*, errors, timeouts); 7 days / 10,000 rows.
CREATE TABLE IF NOT EXISTS script_logs (
  id      BIGINT AUTO_INCREMENT PRIMARY KEY,
  at      DATETIME(3)  NOT NULL,
  level   VARCHAR(8)   NOT NULL,
  source  VARCHAR(191) NOT NULL DEFAULT '',
  message TEXT         NOT NULL,
  trace   TEXT         NULL,
  version BIGINT       NULL,
  unit    VARCHAR(64)  NOT NULL DEFAULT 'site',
  KEY idx_script_logs_at (at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Site Code access overrides (admin, "Access" screen): who may use a route,
-- collection, action, page or developer table, replacing the code's default.
-- roles is a JSON array of role slugs ('*' = anyone, also anonymous).
CREATE TABLE IF NOT EXISTS script_access (
  kind       VARCHAR(16)  NOT NULL,
  name       VARCHAR(191) NOT NULL,
  roles      TEXT         NULL,
  updated_by BIGINT       NULL,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (kind, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Who changed Site Code: save/delete/move/upload/publish/restore/safe_mode.
CREATE TABLE IF NOT EXISTS script_audit (
  id      BIGINT AUTO_INCREMENT PRIMARY KEY,
  at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  user_id BIGINT      NULL,
  action  VARCHAR(40) NOT NULL,
  detail  TEXT        NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Custom fields of posts/pages/custom post types (site.postType / site.fields,
-- Site Code phase 4a): one JSON value per key, deleted with the post.
CREATE TABLE IF NOT EXISTS post_meta (
  post_id  BIGINT       NOT NULL,
  meta_key VARCHAR(64)  CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  value    JSON         NULL,
  PRIMARY KEY (post_id, meta_key),
  KEY idx_post_meta_key (meta_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Site Code settings and secrets (site.settings(key)): secret values are
-- AES-256-GCM encrypted ("enc:v1:…", key SITE_CODE_SECRET_KEY or
-- site-code.key in PLUGINS_DIR) and never returned by the API.
CREATE TABLE IF NOT EXISTS script_settings (
  k          VARCHAR(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
  v          TEXT         NOT NULL,
  secret     TINYINT(1)   NOT NULL DEFAULT 0,
  updated_by BIGINT       NULL,
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Background jobs of Site Code (site.job / jobs.enqueue / site.cron),
-- claimed by the workers with SELECT … FOR UPDATE SKIP LOCKED.
CREATE TABLE IF NOT EXISTS script_jobs (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  name         VARCHAR(100) NOT NULL,
  data         JSON         NULL,
  status       ENUM('pending','running','done','failed','cancelled') NOT NULL DEFAULT 'pending',
  attempts     INT          NOT NULL DEFAULT 0,
  max_attempts INT          NOT NULL DEFAULT 4,
  run_at       DATETIME(3)  NOT NULL,
  started_at   DATETIME(3)  NULL,
  finished_at  DATETIME(3)  NULL,
  progress     VARCHAR(200) NULL,
  result       TEXT         NULL,
  error        TEXT         NULL,
  created_at   DATETIME(3)  NOT NULL,
  dedupe_key   VARCHAR(191) NULL,
  KEY idx_script_jobs_claim (status, run_at),
  KEY idx_script_jobs_name (name, status),
  KEY idx_script_jobs_dedupe (dedupe_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Cron schedules (site.cron): next/last run; an instance claims a run with a
-- conditional UPDATE, so several servers start it once per minute.
CREATE TABLE IF NOT EXISTS script_cron (
  name        VARCHAR(100) NOT NULL PRIMARY KEY,
  expr        VARCHAR(100) NOT NULL DEFAULT '',
  next_run    DATETIME     NULL,
  last_run    DATETIME     NULL,
  last_status VARCHAR(20)  NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Personal access tokens (Site Code phase 5): "ast_" + 40 base62 characters,
-- shown once; only the SHA-256 is stored. prefix = the first 8 characters
-- (shown in the admin). scopes: JSON array (["code"]). A password change
-- deletes the user's tokens, like it ends their sessions.
CREATE TABLE IF NOT EXISTS api_tokens (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  name         VARCHAR(100) NOT NULL,
  token_hash   CHAR(64)     NOT NULL,
  prefix       VARCHAR(16)  NOT NULL,
  scopes       TEXT         NOT NULL,
  last_used_at DATETIME     NULL,
  expires_at   DATETIME     NULL,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_api_tokens_hash (token_hash),
  KEY idx_api_tokens_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
