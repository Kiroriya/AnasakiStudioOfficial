# Writing plugins for Anasaki Studio

Anasaki Studio has three kinds of plugins:

| | **Package plugins** | **Service plugins** | **Built-in (Go) plugins** |
|---|---|---|---|
| What | a `.zip` with a `manifest.json`, templates and static assets | a package whose manifest has a `service` section, **plus a server program** (usually a Docker image) | Go code compiled into the server |
| Installed | upload in the admin or one click from the Plugin Store | one click when the code is WebAssembly (runs inside the CMS); otherwise the program runs next to the CMS and is connected in the plugin's settings | passed to `plugin.NewManager` (none shipped since phase 7) |
| Can do | `<head>` assets, an HTML snippet before `</body>`, designer blocks, a settings form, webhooks | everything a package can, plus public pages, admin screens, server-rendered blocks, collections, events and calls back to the CMS | anything |
| Server-side code | none of its own — add a `service` section (WebAssembly in the zip, or a separate program) and it becomes a service plugin | yes: sandboxed WebAssembly inside the CMS, or its own process/container | yes, inside the CMS |

Use a package plugin for front-end features and a service plugin when you need server code (a shop, a
members area, bookings…). A service plugin's code can ship **inside the package as WebAssembly** — then it
installs with one click and runs sandboxed inside the CMS, like a WordPress plugin but without access to
everything — or run as a separate program. Built-in Go plugins are a mechanism for the core team only; none are
shipped since phase 7 — the former ones (seo, forms, …) are official JS plugins, see §5.

**API reference:** [plugin-api.md](plugin-api.md) lists every public joint (manifest, scopes, `site.*`, `db.*`,
`http.*`, `mail`, `jobs`, `plugins.call`, page hooks, admin pages, the `window.AK` UI kit and the iframe messages)
with the CMS version it appeared in, how to feature-detect it and how to set `minCore`. The core never
special-cases a plugin: official plugins use exactly those joints.

---

## 1. Package plugins

### 1.1 Layout

```
hello-banner/
├── manifest.json          (required, at the zip root)
├── README.md              (shown in the store)
├── icon.png               (optional; or use an emoji in the manifest)
├── assets/
│   ├── banner.css         → <link rel="stylesheet"> on public pages
│   └── banner.js          → <script defer> on public pages
└── templates/
    ├── body.html          → appended before </body>
    ├── card.html          → designer block template
    └── card.css           → block CSS (scoped automatically)
```

The zip may contain the files at its root **or** inside one top-level folder (what "Compress folder"
produces). Both `/` and `\` separators are accepted.

**Limits:** 30 MB zipped, 500 files, 10 MB per file (40 MB for a `.wasm` module), 80 MB unpacked.
Allowed extensions: `.json .html .css .js .md .txt .png .jpg .jpeg .gif .webp .svg .woff .woff2 .wasm`
(`.wasm` is the module of a WebAssembly service, §2).
Absolute paths, `..` segments, drive letters, symlinks and any other extension are rejected, as are
ids already used by a built-in plugin.

### 1.2 `manifest.json` reference

```jsonc
{
  "id": "hello-banner",           // ^[a-z0-9][a-z0-9-]{2,39}$, unique, cannot be a built-in id
  "name": "Hello Banner",         // ≤ 100 chars
  "version": "1.0.0",             // semver; upgrades compare it with the store
  "description": "Top banner with a message",
  "author": "Your name",
  "icon": "📣",                   // an emoji, or a package path such as "icon.png"
  "category": "marketing",        // marketing seo forms ecommerce content social analytics design utilities security
  "tags": ["banner"],             // ≤ 20
  "license": "MIT",
  "homepage": "https://example.com",
  "minCore": "1.0.0",             // refused on older Anasaki Studio versions
  "permissions": ["head", "bodyEnd", "blocks", "settings", "webhooks"],
  "settings": [ /* §1.4 */ ],
  "head": ["assets/banner.css", "assets/banner.js"],
  "bodyEnd": "templates/body.html",
  "blocks": [ /* §1.5 */ ],
  "webhooks": [ /* §1.6 */ ]
}
```

Every feature needs its permission: `head` → `"head"`, `bodyEnd` → `"bodyEnd"`, `blocks` → `"blocks"`,
`settings` → `"settings"`, `webhooks` → `"webhooks"`. The admin shows the list before installing.
Every file named in the manifest must exist in the zip.

### 1.3 Templates

Templates are plain HTML with placeholders. **Every value is HTML-escaped**; there are no loops,
conditions or raw output. Unknown placeholders render as nothing.

| Placeholder | Value |
|---|---|
| `{{settings.key}}` | the plugin's saved setting |
| `{{props.key}}` | the block's property (blocks only) |
| `{{site.title}}` `{{site.tagline}}` `{{site.url}}` | site options |
| `{{post.title}}` `{{post.url}}` `{{post.id}}` … | the current page/post. In `bodyEnd`: `id title url type description image` |
| `{{item.key}}` | the current item when the block sits inside a collection (e.g. a product loop) |

Escaping makes values safe inside text and quoted attributes (`title="{{props.x}}"`). Settings and props of
type `url`/`image` only accept `http(s)://…`, `/path`, `#anchor`, `mailto:`, `tel:`, so they are safe
in `href`/`src`, and `color` only accepts CSS colours, so `style="--accent: {{props.color}}"` is safe.
Do **not** put placeholders inside `<script>` or `<style>`. Pass data to your JS with `data-*` attributes:

```html
<div class="hb" data-text="{{settings.text}}" style="--hb-bg: {{settings.background}}">
  {{settings.text}} <a href="{{settings.linkUrl}}">{{settings.linkText}}</a>
</div>
```

### 1.4 Settings schema

The admin builds the settings form from this schema (the same format is used for block props and
built-in plugin settings):

```jsonc
{ "key": "text", "type": "text", "label": { "th": "ข้อความ", "en": "Text" }, "default": "Hello",
  "help": { "th": "…", "en": "…" }, "placeholder": "…", "maxLength": 200 }
```

| type | value | validation |
|---|---|---|
| `text` | string | ≤ `maxLength` (default 2000 bytes) |
| `textarea` | string | ≤ `maxLength` (default 20000 bytes) |
| `number` | number | optional `min` / `max` |
| `color` | string | `#rgb[a]`, `rgb()/hsl()`, colour names |
| `boolean` | true/false | |
| `select` | string | must be one of `options: [{ "value": "a", "label": { "th": "", "en": "" } }]` |
| `url` / `image` | string | `http(s)://`, `/path`, `#`, `mailto:`, `tel:` (never `javascript:`) |

`label` can also be a plain string. `key` matches `^[a-zA-Z][a-zA-Z0-9_]{0,39}$`. Missing values
get their `default`; unknown keys are dropped; invalid values are refused with
`400 {"error": "...", "fields": {"key": "reason"}}`. Settings are kept when the plugin is upgraded.

### 1.5 Designer blocks

```jsonc
"blocks": [{
  "type": "card",                               // ^[a-z0-9-]{2,40}$
  "name": { "th": "การ์ด", "en": "Card" },
  "icon": "🧩",
  "template": "templates/card.html",
  "css": "templates/card.css",                  // optional
  "props": [ { "key": "title", "type": "text", "label": "Title", "default": "Hi" } ]
}]
```

In the designer the block is a node
`{"type": "pluginBlock", "props": {"plugin": "hello-banner", "block": "card", "values": {"title": "…"}}}`
and renders as `<div class="n-… pb-hello-banner-card">…template…</div>`. Size, padding and fill
come from the node like any frame.

Your CSS is **scoped** under `.pb-<plugin>-<type>`: `h3{…}` becomes `.pb-hello-banner-card h3{…}`.
Use `:scope{…}` or `&{…}` for the wrapper itself. `@media`, `@supports`, `@container` and `@layer`
are scoped recursively, `@keyframes`/`@font-face` are kept, `@import` is removed. The CSS is
emitted once per page however many times the block is used.

When the plugin is deactivated or deleted, its blocks render as empty boxes. Pages are not
broken, and they come back when the plugin is reactivated.

### 1.6 Webhooks

```jsonc
"webhooks": [ { "event": "form.submitted", "url": "{{settings.webhookUrl}}" } ]
```

When the event happens the server POSTs (asynchronously, 5 s timeout) JSON to the URL. The URL
may use `{{settings.*}}` and must resolve to `http://` or `https://`, or nothing is sent:

```json
{ "event": "form.submitted", "plugin": "hello-banner", "data": { … }, "sentAt": "2026-10-13T08:00:00Z" }
```

Headers: `Content-Type: application/json`, `User-Agent: AnasakiStudio-Webhook/1.0`, `X-Anasaki-Event: <event>`.

| Event | `data` |
|---|---|
| `post.published` | `{id, type, title, slug, url}` |
| `user.created` | `{id, email, name, role}` |
| `media.uploaded` | `{id, filename, url, mime, size}` |
| `<plugin>.<event>` or a declared alias | whatever that plugin emits — e.g. `form.submitted` from the official forms plugin (`{formId, formName, submissionId, data, submittedAt}`) |

### 1.7 Assets

Files are served at `/plugin-assets/<id>/<path>` while the plugin is active, with the right
`Content-Type`, `X-Content-Type-Options: nosniff`, and a one-year immutable cache when requested with
`?v=<version>` (the `head` tags do this). Only `.css .js .txt` and images/fonts are served.
Templates, Markdown and JSON stay private. SVGs are served with a sandboxing CSP. A manifest icon
file is also served while the plugin is inactive, so the admin can show it.

Fonts may be packaged as `.woff .woff2 .ttf .otf` (phase 6). A JS plugin reads any of its package files
(except `code/`, `migrations/`, `manifest.json`; ≤ 5 MB) with `site.asset(path)` →
`{path, size, type, url, bytes(), text()}` — e.g. an OFL font embedded in generated PDFs (include the
licence file in the package).

Head assets load on every public page: `.css` as `<link rel="stylesheet">`, `.js` as `<script defer>`.
If your script needs cookie consent, wait for it:

```js
function start() { /* … */ }
if (window.anasakiConsent === 'accepted') start();
document.addEventListener('anasaki:consent', e => e.detail === 'accepted' && start());
```

### 1.8 Packaging and testing

1. Zip the folder (root files or one top-level folder).
2. Admin → Plugins → Upload (`POST /api/v1/package-plugins/upload`, multipart `file`).
3. Activate it, place its block in the designer, and fill in its settings.
4. Upload a new version with a higher `version` to upgrade. Settings are kept, and so is activation —
   unless the new version asks for a permission, a `service.api` scope or a `service.hosts` entry the
   old one didn't have: then the plugin is switched off and listed with what it newly asks for
   (`reconsent` in the API) until an administrator activates it again, which approves it.

Installed files live in `PLUGINS_DIR/<id>/<version>/` (env `PLUGINS_DIR`, default `./plugins-data`).
Old versions are removed on upgrade. Uninstalling also deletes the plugin's stored data (`storage` scope).

### 1.9 Publishing to the Plugin Store

```
curl -X POST https://store.example.com/api/v1/publish \
     -H "X-Publish-Token: <token>" -F "file=@hello-banner-1.0.0.zip"
```

The store validates the same format, keeps every version, and shows `README.md` as the description page.
Every site talks to the official Anasaki Plugin Store (compiled in; developers can point a local build at
another store with env `STORE_URL`). Sites install with `POST /api/v1/package-plugins/install {id, version?}`
and see `updateAvailable` when the store has a newer version.

**Publisher binding (since 2.0.7):** the Store sends a stable publisher id with every plugin (`publisher` in the
catalogue, `X-Anasaki-Publisher` on downloads: `anasaki` for official plugins, `store` for the operator's,
`dev-<n>` for a developer account). The first install from the Store binds the plugin to it (`publisher` in the
installed list); an update from another publisher is refused (409) and the Store screens show
`publisherChanged` instead of an update, so a plugin id that changed hands can't push code to the site. Installs
made before 2.0.7 are bound by their next update. A manual zip upload keeps the binding.

---

## 2. Service plugins

A service plugin is a package plugin (same zip, same install, same store) whose `manifest.json` has the
`"service"` permission and a `"service"` section. The package describes **what** the plugin handles; a
separate program — the *service* — does the work. The CMS stays in charge: it forwards requests to the
service and signs every one of them, so neither side has to trust the network.

```
browser ──► CMS ──(signed HTTP)──► plugin service (its own container, any language)
                ◄──(signed HTTP)──   /api/service/... (site info, mail, posts, users, events)
```

### 2.0 Two ways to run: inside the CMS (WASM) or as a separate service

| | `"runtime": "wasm"` (recommended) | separate service |
|---|---|---|
| Install | one click, like any package — the `.wasm` file is in the zip | run the program yourself (e.g. a container) and connect it in the plugin's settings |
| Runs | inside the CMS, sandboxed (WebAssembly + WASI, via wazero): no files, no network, no database access | its own process, any language |
| Data | the CMS storage API (`"storage"` scope, a key/value store per plugin) | anything (its own database) |
| Outside world | `"http"` scope: https calls to hosts listed in `service.hosts` | anything |
| Limits | 256 MB memory; blocks/collections 3 s, pages 30 s, admin 60 s; 8 concurrent runs per plugin | none |

The protocol below is the same for both, and so is the SDK: the same plugin code builds as either.

```json
"service": {
  "runtime": "wasm",
  "module": "plugin.wasm",
  "api": ["site", "content", "storage"],
  "public": ["/hello"]
}
```

```sh
GOOS=wasip1 GOARCH=wasm go build -trimpath -ldflags="-s -w" -o package/plugin.wasm .   # inside the CMS
go build -o hello-service .                                                           # separate service
```

A WASM module runs once per request (the CMS compiles it once, at activation, and caches the result on
disk), so keep state in the storage API, not in global variables. Any language that targets
`wasm32-wasip1` works; the module reads the request JSON `{method, uri, header, body}` from stdin, writes
`{status, header, body}` (body base64) to stdout, and calls the CMS through the host functions
`anasaki.call(ptr, len) → len` / `anasaki.result(ptr)` (request `{method, uri, body}`, answer
`{status, body}`). The Go SDK hides all of this.

### 2.1 Manifest

```json
{
  "id": "hello-service", "name": "Hello Service", "version": "1.0.0", "icon": "🛰",
  "permissions": ["service", "blocks"],
  "blocks": [{ "type": "greeting", "name": { "th": "คำทักทาย", "en": "Greeting" }, "render": "service",
               "props": [{ "key": "name", "type": "text", "label": { "th": "ชื่อ", "en": "Name" } }] }],
  "service": {
    "public": ["/hello"],
    "events": ["post.published", "user.created"],
    "collections": [{ "source": "quotes", "label": "Quotes" }],
    "admin": [{ "path": "dashboard", "label": "Hello", "labelEn": "Hello", "icon": "🛰", "cap": "manage_options" }],
    "api": ["site", "content", "events"]
  }
}
```

| key | meaning |
|---|---|
| `public` | public path prefixes the service answers (`/shop` also covers `/shop/…`); `api`, `admin`, `uploads`, `download`, `plugin-assets`, `ext-ui`, `sitemap.xml`, `robots.txt` are reserved, and so are the site's current admin path, blog base and category base. Since 2.0.4 the declared prefixes of installed plugins are reserved in turn: the admin path and the bases cannot be set to them (`GET /api/v1/settings/reserved-paths`, [plugin-api.md §8](plugin-api.md)) |
| `events` | events POSTed to the service: core events (`post.published`, `post.saved`, `post.deleted`, `user.created`, `media.uploaded`), other plugins' events (e.g. `form.submitted`), or `*` |
| `collections` | data sources for the designer's collection block (`{{item.*}}` bindings) |
| `admin` | admin screens, shown in the sidebar while the plugin is active (`cap` defaults to `manage_options`) |
| `api` | what the service may call on the CMS: `site`, `mail`, `content`, `users`, `events`, `storage`, `http` |
| `runtime`, `module` | `"wasm"` and the `.wasm` file in the package: runs inside the CMS (§2.0) |
| `hosts` | https hosts the `http` scope may call (`api.example.com`, `*.example.com`) |
| `functions` | functions JS code may call with `plugins.call('<id>', '<fn>', input)` (§4.5) |
| blocks with `"render": "service"` | rendered by the service instead of a template |

### 2.2 Connecting

On install the CMS creates a random **secret** for the plugin. In *Plugins → the plugin → Settings*:
enter the service address (e.g. `http://hello-service:9300` inside the same docker compose), give the
secret to the service, press **Test connection**, then activate the plugin.

```yaml
# docker-compose.yml of the CMS, under services:
  hello-service:
    image: example/hello-service:1.0.0
    environment:
      ANASAKI_PLUGIN_ID: hello-service
      ANASAKI_PLUGIN_SECRET: ${HELLO_SERVICE_SECRET}
      ANASAKI_URL: http://studio:8090
    restart: unless-stopped
```

The service needs no published port: only the CMS talks to it.

### 2.3 Protocol (CMS → service)

| request | when | answer |
|---|---|---|
| `GET /health` | connection test | `{"name","version"}` |
| `ANY /public/<path>` | a visitor opens a `public` path (query, the plugin's own cookies, `Content-Type`, `Accept*`, `User-Agent`, `X-Forwarded-*` are forwarded) | any HTTP response; with `X-Anasaki-Layout: page` (and `X-Anasaki-Title`, URL-encoded) the HTML body is wrapped in the site's header and footer |
| `ANY /admin/<path>` | an admin screen (in an iframe) | any HTTP response; `X-Anasaki-User` = base64 JSON `{id,name,email,role,caps,locale}`, `X-Anasaki-Admin-Base` = the screen's URL prefix |
| `POST /blocks/<type>` | a page with the block is rendered (cached 10 s) | `{"html","css"}` — CSS is scoped under `.pb-<plugin>-<type>` |
| `POST /collections/<source>` `{"query"}` | a collection block is rendered | `{"items":[{"title":…, "url":…}]}` |
| `POST /events` `{"event","data","sentAt"}` | a subscribed event happens (async) | any 2xx |

Every request carries `X-Anasaki-Plugin`, `X-Anasaki-Timestamp` (unix seconds) and two signatures:

- **v1** `X-Anasaki-Signature` = hex(HMAC-SHA256(secret, `<ts>\n<METHOD>\n<request URI>\n<hex sha256(body)>`))
- **v2** `X-Anasaki-Signature-V2` = hex(HMAC-SHA256(secret,
  `v2\n<ts>\n<nonce>\n<METHOD>\n<request URI>\n<hex sha256(body)>\n<X-Anasaki-Plugin>\n<X-Anasaki-User>\n<X-Anasaki-Admin-Base>`)),
  with `X-Anasaki-Nonce` = random hex (16–64 chars); absent headers count as empty strings.

Refuse anything unsigned or older than 5 minutes. Verify v2 when it is present: it also covers the admin user
header, and remembering each nonce for 10 minutes refuses replays. Once v2 has been seen, refuse v1-only
requests (a downgrade). v1 is still sent so services built before v2 keep working. The Go SDK does all of
this (`anasaki.Config.RequireV2` refuses v1-only requests from the start). Timeouts: blocks and collections
3 s, public 30 s, admin 60 s.

**Admin screens** are served under `/ext-ui/<plugin>/<ticket>/<path>`. The ticket (8 hours) is in the URL,
so use **relative links** or `X-Anasaki-Admin-Base`. The page is sandboxed (`Content-Security-Policy:
sandbox`): it can run scripts and submit forms but can never read the admin's session, and cookies set by
the service are dropped. `fetch()` calls to the same `/ext-ui/...` prefix work (CORS is allowed).

**Cookies on public pages** are namespaced: only cookies named `pl_<plugin-id>_…` are forwarded to the
service, and `Set-Cookie` headers for any other name are dropped. A plugin never sees the cookies of the CMS
or of other plugins.

### 2.4 Service → CMS

Sign the same way (your plugin id in `X-Anasaki-Plugin`; send both v1 and v2 with a fresh nonce) and call.
The CMS checks v2 and its nonce when present, and once your service has sent v2 it refuses v1-only requests
claiming to be from it (until its URL or secret is changed, or the CMS restarts):

| call | scope | |
|---|---|---|
| `GET /api/service/site` | `site` | title, tagline, url, logo, icon, language, timezone, theme |
| `POST /api/service/mail` `{to[], subject, html, replyTo}` | `mail` | sends with the site's SMTP settings |
| `POST /api/service/events` `{event, data}` | `events` | emitted as `<plugin>.<event>` to other plugins |
| `GET /api/service/posts?type=post or page&category=&search=&order=&limit=` | `content` | published items (same fields as design bindings) |
| `GET /api/service/users/{id}` | `users` | `{id, name, email, role, avatar, status}` |
| `GET /api/service/kv/get?key=` · `POST kv/set {key, value}` · `POST kv/delete {key}` · `GET kv/list?prefix=&limit=` | `storage` | per-plugin JSON key/value store (value ≤ 1 MB) |
| `POST /api/service/fetch {method, url, headers, body}` | `http` | https request to a host in `service.hosts` → `{status, headers, body}` |

A service keeps its data in the storage API or its own database; it never gets the CMS database.

### 2.5 Go SDK

`sdk/go/anasaki` implements all of the above:

```go
p := anasaki.New(anasaki.Config{Name: "Hello Service", Version: "1.0.0"}) // reads ANASAKI_* env
p.Public("GET /hello", func(w http.ResponseWriter, r *http.Request) {
    anasaki.Page(w, http.StatusOK, "Hello", "<h1>Hello!</h1>")
})
p.Admin("GET /dashboard", func(w http.ResponseWriter, r *http.Request) { u := anasaki.UserFrom(r); _ = u })
p.Block("greeting", func(in anasaki.BlockInput) anasaki.BlockOutput { return anasaki.BlockOutput{HTML: "…"} })
p.Collection("quotes", func(q map[string]any) []map[string]any { return nil })
p.OnEvent(func(e anasaki.Event) {})
site, _ := p.CMS.Site(ctx)                    // scope site
_ = p.CMS.Set(ctx, "visits", 42)              // scope storage (Get, Delete, List too)
res, _ := p.CMS.Fetch(ctx, "GET", "https://api.example.com/x", nil, "") // scope http
log.Fatal(anasaki.Run(p, ":9300"))            // a server, or one WASM run — chosen at build time
```

A complete example (public page, block, collection, admin screen, events, Dockerfile) is in
`examples/service-plugins/hello-service`.

## 4. JavaScript plugins (`"runtime": "js"`)

A JS plugin is Site Code packed as a plugin: the same TypeScript/JavaScript API (`docs/site-code.md`),
running inside the CMS as its own **unit** with its own tables, settings, cookies, hooks and routes,
limited to the code scopes the administrator approved. Examples: `examples/js-plugins/money` (a library)
and `examples/js-plugins/shop` (requires money); `examples/service-plugins/functions-wasm` is a WASM
plugin with `service.functions`.

**Sample plugins** (phase 5, `examples/js-plugins/<id>`, each with a Thai/English README; they use only
public core joints — anything missing is listed in `examples/js-plugins/CORE_GAPS.md`):

| plugin | shows |
|---|---|
| [`thai`](../examples/js-plugins/thai) | library + formatters: `bahtText`, `thaiDate` (พ.ศ.), phone / citizen id checks |
| [`promptpay`](../examples/js-plugins/promptpay) | PromptPay EMVCo payload + pure-JS QR SVG, a route behind a setting |
| [`files`](../examples/js-plugins/files) | CSV / XLSX / PDF writers and download helpers |
| [`admin-crud`](../examples/js-plugins/admin-crud) | a generic admin page (list / search / edit) via `site.adminPage` + actions |
| [`members`](../examples/js-plugins/members) | site members: register / login / verify / reset, `site.visitor`, groups, mail |
| [`money`](../examples/js-plugins/money) · [`shop`](../examples/js-plugins/shop) | library + exports, requires, migrations, cron |

A JS plugin can ship **`code/index.d.ts`**: it becomes the `declare module '<id>'` block of `anasaki.d.ts`
(`GET /api/v1/code/types`), so importers get precise types; without it, library exports and `code.exports`
are typed `any`. Developers working locally use the `anasaki-code` CLI ([docs/cli.md](cli.md)).

### 4.1 Manifest

```jsonc
{
  "id": "shop", "name": "Shop", "version": "1.2.0", "minCore": "1.0.0", "runtime": "js",
  "permissions": ["code"],
  "code": {
    "entry": "code/main.ts",                    // required: .ts/.js/.mjs under code/
    "scopes": ["db", "content", "mail", "routes", "jobs"],
    "hosts": ["api.example.com"],               // needs the "http" scope
    "routes": ["/shop", "/cart"],               // needs the "routes" scope
    "library": "code/lib/index.ts",             // optional: bundled into importers
    "exports": ["cartOf", "createOrder"],       // optional: site.export names, run in this plugin
    "hooks": { "filters": ["shop.price"], "events": ["shop.order.paid"] },
    "eventAliases": { "shop.order.paid": "order.paid" } // phase 7: also delivered under this name
  },
  "requires": { "money": "^1.0.0" },
  "optionalRequires": { "thai": "^1.0.0" }     // phase 6: "works with" — never blocks an install
}
```

- Files: `code/**` and `migrations/**` (`.ts .js .mjs .json .md`, segments `A-Z a-z 0-9 . _ -`, no
  case-only duplicates, ≤ 200 files, ≤ 512 KB each, ≤ 2 MB in total) plus the usual package files.
  `.ts`/`.mjs` are allowed only there and only in JS plugins; a non-JS package may not have a `code`
  section or the `code` permission; a JS plugin may not have a `service` section.
- Migrations `migrations/NNN_name.ts|js` need the `db` scope. They run at activation and on upgrades of an
  active plugin; destructive ones need the administrator's confirmation (`confirmDestructive`; tables are
  backed up first). They are recorded as `<id>/<file>`.
- Scopes: `db content content-write mail http visitor admin-pages jobs routes filters-core middleware
  head-scripts content-fields webhooks post-types` (2.0.5: `post-types` = `site.postType`, type ids `<id>_<name>`, see `docs/plugin-api.md` §3.5; phase 6: `middleware` = `site.middleware` / `site.afterResponse`;
  `head-scripts` = `site.head` / `site.bodyEnd` / `site.block` output inserted unsanitised — treat it like
  `unfiltered_html`; phase 7: `content-fields` = `site.fields` / `site.meta` / `site.setMeta` (post meta
  `<id>.<key>`, a panel in the post editor), `webhooks` = `http.webhook` to any public URL; `admin-pages` also
  covers `site.adminNotice` / `site.dashboardWidget`; see `docs/site-code.md`, "JavaScript plugins" and
  "Joints for the official plugins").
- `code.eventAliases` (phase 7, ≤ 10): `{"<id>.event": "other.name"}` — the plugin's own event is also delivered
  under the other name (used by the official forms plugin for the former core event `form.submitted`). The alias
  may not use the plugin's own prefix or a CMS namespace (`post. user. media. comment. site. plugin. page. mail.
  action. collection. sitemap. robots.`).
- Manifest `settings` are what plugin code reads first with `site.settings(key)` (then Site code → Settings
  `<id>.<key>`, then the default) and what `site.saveSettings(values)` updates from the plugin's admin page
  (phase 7). Route prefixes may not start with `api admin uploads
  download plugin-assets _a _c ext-ui sitemap.xml robots.txt favicon.ico favicon.svg assets static blog
  category`, nor the site's admin path / blog / category bases. Exports `^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`
  (≤ 50); hook names `<id>.…` (≤ 50 each). The CMS validator applies the same rules as the Plugin Store's.

### 4.2 Permissions and consent

The `code` permission and every scope are listed before installing. An upgrade asking for a new
permission, scope or host switches the plugin off until an administrator activates it again
(`reconsent: {permissions, api, hosts, scopes}`).

### 4.3 Dependencies (`requires`)

Any package may declare `requires: {"<plugin id>": "<range>"}` (npm-style ranges: `^1.2.0`, `~1.2`,
`1.x`, `>=1.0.0 <2`, `*`, alternatives with `||`; no hyphen ranges, leading `v` or build metadata; ≤ 20
entries, not `backup` (the core export tool), not itself; the official plugins — `seo`, `forms` … — may be
required like any plugin).

- **Install / upload** with missing or incompatible dependencies → `400 {error, missing:[{id, range,
  installed?, active?, storeVersion?, latest?, compatible?}]}` (store installs fill `storeVersion`/`latest`
  with the newest compatible store version, so the admin can install the dependencies first).
- **Activate** needs the dependencies installed, compatible and active (`400 {error, missing}` with
  `active: false`).
- **Deactivate / uninstall** a plugin others require → `409 {error, dependents:[ids]}` unless `?force=1`
  (then the dependents are switched off too). Upgrading to a version a dependent's range no longer
  accepts is refused.

### 4.3.1 Optional dependencies (`optionalRequires`, phase 6)

`optionalRequires: {"<plugin id>": "<range>"}` lists plugins this one **works with** when they are
installed (shown as "works with / ใช้ร่วมกับ" on the Store and CMS cards). Same syntax rules as
`requires` (an id may not be in both). It never blocks an install or activation; an installed but
incompatible version only logs a warning when the plugin loads. Code checks at run time with
`plugins.has(id)` / `plugins.version(id)` and calls exports with `plugins.call(id, name, ...args)` (a
missing target throws `PluginUnavailable`, `e.code === 'unavailable'`). Static `import` still needs
`requires`.

### 4.4 Package API additions

| call | |
|---|---|
| `POST /api/v1/package-plugins/upload?confirmDestructive=1` (or form field `confirmDestructive=1`) | install/upgrade; `400 {error, destructive:[{migration, op, table, column}]}` without confirmation; `400 {error, activationError, errors:[{file,line,column,message}]}` when an active plugin's new code does not build (nothing changes) |
| `POST /api/v1/package-plugins/install {id, version?, confirmDestructive?, acceptPublisher?}` | from the store, same errors; 409 `{error, publisherChanged:true, installedPublisher, offeredPublisher}` when the update comes from another Store publisher than the installed plugin is bound to (`acceptPublisher:true` installs it anyway) |
| `POST /api/v1/package-plugins/{id}/activate?confirmDestructive=1` (or body `{confirmDestructive:true}`) | builds, migrates and registers; failure → `400 {error, activationError, errors}`, the plugin stays off and lists `activationError` |
| `POST /api/v1/package-plugins/{id}/deactivate?force=1` | `409 {error, dependents}` without force |
| `DELETE /api/v1/package-plugins/{id}?force=1&deleteData=1` | `deleteData`: back up (7 days) and drop the `p_<id>_*` tables, delete its settings, access rows, jobs, cron and migration records |
| `GET /api/v1/package-plugins` | items gain `runtime` (`js` / `wasm` / `service` / ""), `requires` (map), `requiresStatus:[{id, range, installed, active, ok}]`, `dependents`, `code:{entry, scopes, hosts, routes, exports, library, hooks}`, `activationError?`, `functions?`; phase 6: `optionalRequires` (map) and `optionalRequiresStatus` (same shape as `requiresStatus`; `ok:false` with an installed version = incompatible, a warning only) |

### 4.5 Functions of service / WASM plugins

`"service": {"functions": ["monthly"]}` makes `plugins.call('<id>', 'monthly', input)` available to JS
code (Site Code always; a JS plugin when `<id>` is in its `requires` or `optionalRequires`). A JS plugin's
`code.exports` are callable the same way by any unit without a `requires` entry (phase 6). The CMS POSTs `{input, user,
visitor}` to `/functions/monthly` (signed like every call; a WASM module gets the same envelope); answer
`{"result": …}` (2xx) or `{"error": "…"}` (4xx/5xx). Events a JS unit emits with `site.emit` are delivered
to services subscribed to that name, and `/api/service/events` events reach `site.on` in every unit.

## 5. Official plugins and the upgrade from the built-ins (phase 7)

Up to phase 6 seven features were Go plugins compiled into the server. Since phase 7 they are **official JS
plugins** distributed through the Plugin Store, and **backup is a core system tool** ("Export / import
site", §5.4). Their packages live in `plugins-official/<id>` of this repository:

| id | what it does |
|---|---|
| `seo` | title template, descriptions, Open Graph / Twitter, per-page overrides, `/sitemap.xml`, `/robots.txt` |
| `redirects` | 301/302/307/308 rules and the 404 log |
| `forms` | form builder, block, inbox, CSV, webhook, e-mail notification |
| `cookie-consent` | PDPA/GDPR consent banner |
| `analytics` | cookie-less server-side page views |
| `maintenance` | 503 notice page with a bypass key |
| `custom-code` | raw HTML in `<head>` / before `</body>` |

Which of them the setup wizard offers (and ticks) is decided in the Plugin Store, not in the CMS (§5.2).

### 5.1 The official package format

An official plugin is an ordinary JS plugin (§4) with:

- the same `id` as the former built-in, `"version": "2.0.0"`, `"author": "Anasaki Studio"`, `"runtime": "js"`,
  a Thai + English README;
- `"legacy": "<id>"` — the former built-in whose data it takes over (must equal `id`);
- manifest `settings` with **the same keys** as the built-in's settings, so the stored values map 1:1;
- an export **`legacyImport`** listed in `code.exports` when the built-in had tables:

```ts
// payload: { kind: 'rows', table: '<old table name>', rows: [...], last: boolean }
site.export('legacyImport', (p) => {
  for (const r of p.rows) { /* upsert by the old id / natural key into the plugin's own tables */ }
  return { imported: p.rows.length };
});
```

Rows come in batches of at most 500, table by table in dependency order (e.g. `forms_forms` before
`forms_submissions`); `last` is `true` on the last batch of each table (an existing but empty table is sent
once as `rows: []`, `last: true`; when every old table is empty nothing is sent). Values: numbers, strings,
`true`/`false` for booleans, parsed JSON for JSON columns, `null` for NULL, DATETIME as RFC 3339 UTC
(`"2024-05-01T10:00:00Z"`, bound as a date by `db`), DATE as `"2024-05-01"`. The import must be
**idempotent** (a retry may send a batch again). Old tables: `analytics_daily`, `analytics_visitors`,
`analytics_page_visitors`, `analytics_referrers`; `seo_meta`; `forms_forms`, `forms_submissions`;
`redirects_rules`, `redirects_404` (columns as in `backend/internal/legacy/legacy.go`).

The Plugin Store keeps these seven ids for the operator and store admins; `scripts/publish-official.sh`
zips `plugins-official/*` and publishes them with the operator token as **Official / ทางการ**:

```sh
STORE_URL=http://localhost:8095 STORE_PUBLISH_TOKEN=<one of STORE_PUBLISH_TOKENS> scripts/publish-official.sh [id ...]
```

### 5.2 Fresh installs: recommended plugins

The setup wizard creates the site first, then offers a **Recommended plugins** step: a checklist with a short
description and the permissions each plugin asks for. The CMS hardcodes no plugin (since 2.0.4): the list is
what the configured Plugin Store returns for `GET /api/v1/plugins?recommended=1&perPage=50` — items with
`recommended: true` **and** `official: true`, ticked when `recommendedChecked` is true, sorted by
`recommendedOrder` then name (store admins set these flags). Permissions come from the Store detail when the
summary has none. Ticked plugins are downloaded from the Store, checked to be official (`author` "Anasaki
Studio") and switched on; install accepts only ids the Store **currently** lists as recommended and official.
If the Store cannot be reached, or recommends nothing, the wizard still finishes (*install later from the
Plugins page*).
API: `GET /api/v1/setup/recommended` → `{storeAvailable, items: [{id, name, description, icon, iconUrl?,
checked, order, available, version, permissions, scopes, hosts, installed}]}` (`name` / `description` as the
Store sends them: a string or `{th, en}`; Store down → `storeAvailable: false`, no items); `POST
/api/v1/setup/recommended/install {ids}` → `{storeAvailable, results: [{id, ok, version?, error?}]}`
(both `manage_options`).

### 5.3 Upgraded sites: automatic migration

At startup (and every minute) a background task looks at every former built-in that **is not part of this
build** and **was active** (`active_plugins` option) **or left data** (rows in its old tables, or stored
settings). For each one it records a row in `legacy_migrations` (`id, status
pending|installing|importing|done|failed, error, attempts, updated_at, …`) and then:

1. gets the official package — already installed (any source), an uploaded zip (below), or a download
   from the Store — and checks it is official (`legacy` = id, author Anasaki Studio, runtime js) after the
   usual package validation;
2. installs it and switches it on; the permissions are approved on the administrator's behalf and
   recorded as approved by **"system migration"** with the time and the list (`approved`);
3. copies the settings (same keys; invalid values fall back to the plugin's defaults);
4. streams the old rows to `legacyImport`, saving its position after each batch (a retry resumes there);
5. marks it `done`. A built-in that was **inactive** but had data ends up installed, imported and
   **switched off**, like before.

Failures (Store unreachable, a package that is not official, an import error) mark the row `failed`
and retry automatically with backoff (1, 2, 4 … minutes, at most hourly). While anything waits the admin
shows a banner **“N plugins are waiting to be moved”** with **Retry** and **Upload the zip instead**
(sites without Store access upload the official zip; the same task picks it up). The feature stays off
until its plugin is moved — the site itself keeps working.

**Old data is never deleted automatically.** On the Plugins page each migrated plugin has **Clean up old
data** (after a confirmation) which drops its old tables and deletes its old options
(`plugin_settings.<id>`, `analytics_salt`) and its `active_plugins` entry. The plugin's own data is not
touched.

API (`manage_options`): `GET /api/v1/legacy-migrations` → `{waiting, running, items: [{id, name, nameEn,
icon, status, error, attempts, wasActive, hadData, imported, source, version, approvedBy, approvedAt,
approved{permissions,scopes,hosts}, nextAttemptAt, updatedAt, doneAt, cleanedAt, cleanable}]}`;
`POST /api/v1/legacy-migrations/retry {id?}`; `POST /api/v1/legacy-migrations/upload` (multipart
`file`); `POST /api/v1/legacy-migrations/{id}/cleanup {confirm: true}` (409 until the migration is done).

### 5.4 Export / import site (core)

**Settings → Export / import site** (`/tools/export`, `manage_options`) replaces the backup plugin with
the same zip format: `data.json` + `uploads/`. Besides the content it now lists the **installed package
plugins** (id, version, active, source, settings without secret-looking keys — never their code) and the
**Site Code draft**. Importing adds content (never overwrites or deletes), reports which plugins are
missing (one click installs them from the Store — they start switched off, as after any install — and
applies their settings), and adds the Site Code files that do not exist yet to the draft (only for an
importer with `edit_scripts`; nothing is published). API: `GET /api/v1/tools/export`, `POST
/api/v1/tools/import` → `{imported: {posts, terms, menus, components, media}, packages: [{id, name, version,
active, source, settings, status: installed|missing, installedVersion?}], siteCode: {added, skipped}}`.

## 3. Built-in (Go) plugins

The plugin manager still accepts Go plugins (`plugin.NewManager(st, uploadDir, plugins...)`), but core
passes none since phase 7. A built-in plugin is a Go package implementing `plugin.Plugin`, handed to
`NewManager`. The order there is the admin order and also the middleware order (first = outermost).

```go
type Plugin interface {
    Info() plugin.Info                          // id, name, description, version, author, emoji icon
    Migrate(db *sql.DB) error                   // CREATE TABLE IF NOT EXISTS …, runs at every start
    Register(r *plugin.Registry)                // declare everything below, once at start
    Activate(ctx plugin.ActivateContext) error  // idempotent first-time setup (sample data…)
}
```

Everything registered only takes effect **while the plugin is active**. Inactive plugins' API
routes answer 404.

| Registry method | Purpose |
|---|---|
| `Handle("GET /things/{id}", h, caps...)` | authenticated API at `/api/v1/<id>/things/{id}`; caller needs any of `caps` |
| `HandlePublic("GET /thing/{slug}", h)` | public site route, checked before the site's own routing |
| `Middleware(func(http.Handler) http.Handler)` | wrap the public site (redirects, maintenance, analytics) |
| `Head(func(PageInfo) string)` / `BodyEnd(...)` | per-page HTML in `<head>` / before `</body>` |
| `PageFilter(func(*PageInfo))` | change `Title`, `Description`, `Image`, `FullTitle` (`<title>`), `NoIndex` before the head is written |
| `HeadTag(html)` | static `<head>` HTML |
| `On("post.published", func(payload))` | core events (async, panics recovered) |
| `Settings(fields...)` + `Values()` | settings schema (§1.4). The admin edits it through `GET/PUT /api/v1/plugin-settings/<id>` (stored in option `plugin_settings.<id>`). `Values()` returns validated values with defaults (cached 2 s) |
| `Block(plugin.Block{Type, Name, Icon, Props/PropsFunc, CSS, Render})` | designer block rendered by Go. Escape your output with `plugin.Esc`; scope CSS under `.pb-<id>-<type>` |
| `Collection(plugin.Collection{...})` | data source for designer collection nodes |
| `Menu(plugin.MenuItem{Path, Label, LabelEn, Icon, Cap})` | admin sidebar entry |
| `Grant(role, caps...)` | add capabilities to a role |
| `DefaultActive()` | switch on at first start (once, option `plugins_defaults_applied`) |
| `Host()` | `Store`, `UploadDir`, `Pages` (render pages with the site's header/footer), `Active(id)`, `Emit(event, payload)` |

`PageInfo` = `{Path, Title, Description, Image, PostID, PostType (page|post|archive|404|plugin), Status,
FullTitle, NoIndex, BaseURL, Site}`. Use `p.AbsURL(path)` for absolute URLs.

Helpers: `plugin.WriteJSON`, `WriteErr`, `ServerErr`, `ReadJSON`, `ClientIP`, `StatusRecorder` (for
middlewares that need the final status), `PostJSON` (webhooks), `SafeURL`.

Rules: create only your own tables (prefix them with the plugin id) and use additive migrations;
never block in `Head`/`BodyEnd` hooks (they run on every page view); do slow work asynchronously.

### No feature plugins are compiled in any more

Since phase 7 core ships **no** built-in feature plugins: analytics, seo, forms, redirects, cookie-consent,
maintenance and custom-code are official JS plugins from the Plugin Store (§5, sources in
`plugins-official/`), and backup is the core tool "Export / import site"
(§5.4). **Export / import limits:** import never overwrites or deletes. It remaps component instances, menus,
download media, featured images and upload URLs inside designs and HTML. Options are exported
(secrets such as tokens, passwords, salts, bypass keys and custom code are left out) but not
imported. Template assignments and the home page are not changed. Categories with an existing
slug are reused. Users are not exported, so imported content belongs to the importing admin.
