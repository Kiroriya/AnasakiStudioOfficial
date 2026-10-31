# Anasaki Studio — CORE API reference (v1.0.0)

Base: `/api/v1`. JSON in/out. Auth: `Authorization: Bearer <token>` (JWT, 7 days).
Errors: `{"error": "<short Thai message>"}` with 400 (validation) · 401 (login again) · 403 (no capability /
suspended) · 404 · 409 (conflict). "cap" = required capability. Plugin and platform endpoints are in
`PLATFORM_SPEC.md` §4–5.

## Setup & public info (no auth)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/v1/setup` | → `{needsSetup, product, version}` (`needsSetup` = no user yet) |
| POST | `/api/v1/setup` | `{siteTitle, tagline, language:"th"\|"en", adminName, adminEmail, adminPassword (≥8), locale:"th"\|"en", demo:bool}` → 201 `{token, user}`; 409 once installed. `demo:true` installs the demo site, `false` a blank published home page |
| GET | `/api/public/site` | → `{siteTitle, tagline, siteIcon, siteLogo, adminPath, product, version}` |
| GET | `/api/health` | → `{status:"ok"}` |

First run: the admin is created automatically only when **both** `ADMIN_EMAIL` and `ADMIN_PASSWORD` are set
(docker automation, installs the demo); otherwise the admin SPA shows the wizard.

## Auth

| Method | Path | cap | Body → Response |
|---|---|---|---|
| POST | `/auth/login` | – | `{email, password}` → `{token, user}`; 401 wrong credentials; 403 `บัญชีนี้ถูกระงับ` (suspended). Records `lastLoginAt` |
| GET | `/auth/me` | any | → `User` |
| PUT | `/auth/me` | any | any of `{name, email, password, locale, firstName, lastName, bio, website, avatar}` → `User` |

`User` = `{id, email, name, firstName, lastName, bio, website, avatar, role, status:"active"|"suspended", locale,
caps:[string], lastLoginAt:string|null, createdAt, postCount}` (`name` = display name, `postCount` = posts of type post).
Suspended users get 403 on every authenticated endpoint.

## Users (cap `manage_users`)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/users?search=&role=&status=&page=1&perPage=20` | → `{items:[User], total, counts:{all, suspended, <roleSlug>: n…}}` (counts are global) |
| POST | `/users` | `{name, email, password(≥8), role (default subscriber), status, locale, firstName, lastName, bio, website, avatar}` → 201 `User` |
| PUT | `/users/{id}` | same fields, all optional (password only when changing) → `User` |
| DELETE | `/users/{id}?reassign=<userId>` | → 204; content goes to `reassign` (default: the caller) |
| POST | `/users/bulk` | `{ids:[id], action:"role"\|"suspend"\|"activate"\|"delete", role?}` → `{updated}` (delete reassigns to the caller) |

Guards (400): you cannot suspend, delete or change the role of yourself (single or bulk). `website`/`avatar`
must be `http(s)://…` or a site path.

## Roles (cap `manage_users`)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/roles` | → `{roles:[{slug, name, builtin, caps, userCount}], caps:[{key, group}]}` |
| POST | `/roles` | `{slug (^[a-z][a-z0-9_-]{1,31}$), name, caps}` → 201 Role |
| PUT | `/roles/{slug}` | `{name?, caps?}` → Role. `administrator` is immutable (400); other built-ins are editable |
| DELETE | `/roles/{slug}` | → 204, custom roles only; their users become `subscriber` |

Groups: `content, design, media, users, settings, shop, stats, other`. The catalog includes plugin caps.
Built-ins: `administrator` (every cap, always), `editor`, `author`, `contributor` (`read, edit_own_posts` — saving
with `status:"publish"` is stored as `draft`), `subscriber` (`read`).
Core caps: `read, edit_own_posts, edit_others_posts, publish_posts, manage_categories, unfiltered_html, edit_pages,
upload_files, manage_users, manage_options, view_stats`. Effective caps = DB role caps ∪ plugin grants.

## Settings

| Method | Path | cap | Body → Response |
|---|---|---|---|
| GET | `/settings` | any | → every key below, defaults filled in (all values are strings) |
| PUT | `/settings` | `manage_options` | partial `{key: value}`; unknown keys ignored; `""` resets to default → full settings |

| Key | Default | Rule |
|---|---|---|
| `site_title`, `tagline` | `""` | |
| `language` | `th` | `th`\|`en` |
| `home_page_id`, `post_template_id`, `archive_template_id` | `""` | numeric id |
| `site_icon`, `site_logo` | `""` | URL (`http(s)://` or `/uploads/…`) |
| `admin_email` | `""` | email |
| `timezone` | `Asia/Bangkok` | IANA zone |
| `date_format` | `2 Jan 2006` | Go reference-time layout, ≤64 chars |
| `posts_per_page` | `10` | 1–100 |
| `blog_base`, `category_base`, `admin_path` | `blog`, `category`, `admin` | `^[a-z0-9-]{3,32}$`, not reserved (`api uploads download plugin-assets shop product cart checkout thank-you forms sitemap.xml robots.txt favicon.ico assets static feed search`), all three distinct |
| `search_visibility` | `1` | `1`\|`0` |
| `max_upload_mb` | `32` | 1–512 |
| `allowed_upload_types` | `jpg,jpeg,png,gif,webp,svg,mp4,webm,mp3,wav,ogg,pdf,doc,docx,xls,xlsx,ppt,pptx,odt,ods,odp,txt,csv,rtf,zip,rar,7z` | comma list; never-allowed extensions are rejected |

**Admin path**: the SPA is served at `/<admin_path>/` (`<base href>` rewritten); after a change the old `/admin/`
answers 404. The value is cached ≤5 s (immediately in the process that saved it).

## Media & downloads

| Method | Path | cap | Body → Response |
|---|---|---|---|
| GET | `/media?kind=&search=` | `upload_files` | → `[Media]` (newest first; `kind` = image\|video\|audio\|document\|archive\|other; `search` on filename/alt) |
| POST | `/media` | `upload_files` | multipart `file`, optional `alt` → 201 `Media` |
| POST | `/media/import` | `upload_files` | `{url, alt?}` → 201 `Media`: the server downloads a public http(s) file (same type/size rules; loopback, private and link-local addresses are refused, 5 redirects max) |
| PUT | `/media/{id}` | `upload_files` (own, or `edit_others_posts`) | `{alt}` → `Media` |
| DELETE | `/media/{id}` | same | → 204 |
| GET | `/download/{id}` | public | file as `attachment` (`filename*=UTF-8''…`, nosniff), counts a download; Range supported |

`Media` = `{id, filename, url, mime, size, width, height, alt, kind, ext, downloads, authorId, createdAt}`.
Upload rules: extension must be in `allowed_upload_types`; always denied: `html htm xhtml shtml js mjs cjs php phtml phar
exe bat cmd sh msi dll jar com scr vbs ps1 hta cgi asp aspx jsp pl py`; SVG needs `unfiltered_html` (403); images
must really be images and HTML content is refused for non-text files; size ≤ `max_upload_mb`.
`/uploads/*` is served with `X-Content-Type-Options: nosniff`.

## Menus (read: any user · write: `edit_pages`)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/menus` | → `[{id, name, items:[{label, url}], updatedAt}]` |
| POST | `/menus` | `{name, items}` → Menu |
| PUT | `/menus/{id}` | `{name, items}` → Menu; 404 when the menu was deleted |
| DELETE | `/menus/{id}` | → 204 (also when designs reference it: they then render an empty menu) |

## Posts

`GET/POST /posts`, `GET/PUT/DELETE /posts/{id}`, `POST /posts/{id}/duplicate`. Without `publish_posts`,
`status:"publish"` is saved as `draft`.

- `Post.pinned` (bool, set with `edit_others_posts`): pinned posts come first in every post list on the site.
- `GET /posts?order=manual` and post-list blocks with `query.order:"manual"` use the hand-made order saved with
  `PUT /posts/order {type, ids:[…]}` (first id = 1; `edit_others_posts` for posts, `edit_pages` otherwise) → 204.
- Revisions (kept on every save that changes title/content/design, 25 per post): `GET /posts/{id}/revisions`,
  `GET /posts/{id}/revisions/{rid}` (with `content` and `design`), `POST /posts/{id}/revisions/{rid}/restore`.
- Component versions (`edit_pages`, 25 per component): `GET /components/{id}/revisions`,
  `GET /components/{id}/revisions/{rid}` (the old node is in `design`).

## Theme (design tokens)

Setting `theme` (JSON, `manage_options`): `{colors:{primary, secondary, accent, text, muted, surface}, fontHeading, fontBody}`.
Colors are `#hex` or `rgb()/rgba()`, fonts `[A-Za-z0-9 ]` (Google Fonts). Every page gets
`:root{--c-primary:…}` and the fonts; designs refer to colors as `var(--c-<key>)`.

## Design nodes added in this version

- `layout.columns` (1–12) on a horizontal auto layout renders an even CSS grid (override per breakpoint).
- `text.props.rich`: inline HTML subset (`b strong i em u s mark sup sub br a[href,target] span[style=color|background-color|font-weight]`),
  sanitized on render; `props.text` keeps the plain text.
- `breadcrumb` node: Home › Category › Title from the page context; props `homeLabel`, `separator`,
  `currentColor`, `hideCategory`. Bindings gained `{{post.categoryUrl}}` and `{{post.pinned}}`.

## Events emitted to plugins (`Manager.Emit`)

`post.published {id, type, title, slug, url}` (create/update when the status becomes publish) ·
`user.created {id, email, name, role}` (POST /users and setup) · `media.uploaded {id, filename, url, mime, size}`.

## Plugin Store reference server (`cmd/store`)

Implements `PLATFORM_SPEC.md` §3. Env: `STORE_ADDR` (`:8095`), `STORE_DB_DSN` (default: the CMS database; tables
`store_*`), `STORE_DATA_DIR` (`./store-data`), `STORE_EXAMPLES_DIR` (`./examples/plugins`, published on start when
the version is new, `verified:true`), `STORE_PUBLISH_TOKENS` (comma list; empty disables `POST /publish`),
`STORE_PUBLIC_URL` (base for `iconUrl`/screenshots). Extra route: `GET /files/{id}/{version}/{path}` (package
images only). Reviews: one per IP per plugin per day (429 otherwise; IPs are stored hashed).
Rankings: `topDownloads` (all-time, >0), `topRated` (≥1 rating), `trending` (downloads within `period`, default week).
`sort=popular` = downloads×3 + views + ratings×5; `trending` = downloads in the last 7 days.

## Email (built-in SMTP)

Settings keys (via `GET/PUT /settings`): `smtp_host`, `smtp_port` (empty = 587/465/25 by encryption), `smtp_user`, `smtp_password`
(GET returns `••••••••` when set; sending that value back keeps the stored password; never exported by Backup),
`smtp_encryption` (`starttls` | `ssl` | `none`), `mail_from_email` (empty = `admin_email`), `mail_from_name` (empty = site title).

| Method & path | Auth | Notes |
|---|---|---|
| `GET /mail/status` | any user | `{configured}` |
| `POST /mail/test {to?}` | `manage_options` | sends a test email (default: `admin_email`) → `{sentTo}`; SMTP errors → 502 with a readable message |
| `POST /auth/forgot {email}` | public | always 200 (does not reveal accounts); 503 when email is not configured; 5 requests / 15 min per IP |
| `POST /auth/reset {token, password}` | public | one-time token (2 h; 72 h for welcome links) → `{token, user}` (signed in) |
| `POST /users {…, sendWelcome: true}` | `manage_users` | also emails the new user a "choose your password" link |

The forms plugin emails `admin_email` on every submission (Reply-To = the visitor's email field).
Plugins can send mail with `Host.SendMail(to, subject, html, replyTo)`.
