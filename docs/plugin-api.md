# Plugin API reference

This page lists the public API of Anasaki Studio that plugins can rely on: the manifest, permissions and
scopes, the server-side joints (`site.*`, `db.*`, `http.*`, `mail`, `jobs`, `plugins.*`), page hooks, admin
pages, dashboard widgets and notices, the admin UI kit (`window.AK`) and the iframe messages.

**The core never knows or special-cases a plugin.** Official plugins (seo, forms, …) use exactly the joints
listed here and nothing else; anything they can do, your plugin can do too. If you need a joint that is
missing, ask for it to be added here rather than relying on internals.

- Tutorials and the package format in depth: [plugins.md](plugins.md)
- The JS API in depth (the same API Site Code uses): [site-code.md](site-code.md)
- Exact TypeScript types: `GET /api/v1/code/types` (`anasaki.d.ts`), shipped with every CMS

## Versions

Every entry says the CMS version it appeared in. The CMS version is in the `VERSION` file, shown in the
admin, and returned by `GET /api/public/site` (`version`).

| tag | meaning |
|---|---|
| **2.0.0** | first release of the plugin platform (package, service, WASM and JS plugins; phases 1–7) |
| **2.0.3** | admin UI kit `AK.line`, `AK.open`, `AK.height`, `data-ak-line`, `data-ak-open`; the `ak:height` and `ak:navigate` iframe messages; auto-height admin pages and widgets |
| **2.0.4** | `AK.pickMedia` and the `ak:pick-media` message; reserved paths from the installed plugins' routes (`GET /api/v1/settings/reserved-paths`); setup-wizard recommendations from the Store |
| **2.0.5** | JS plugins register post types: `site.postType` with the `post-types` scope (§3.5) |
| **2.0.6** | plugin capabilities: manifest `capabilities`, ticked per role in Users → Roles; `cap:<capability>` access entries, `site.can`, post type `caps` (§2.1) |
| **2.0.8** | `site.fields('*', fields)` — fields on every content type (also other plugins' types); a page result names the post it shows: `{page: {title, html, postId}}`, so page hooks (`page.meta`) see that post |
| **2.1.0** | media and files from code (`media.*`, `files.*`, `request.files()`, filter `media.beforeSave`, §3.2a); custom field types `relation`, `user`, `term`, `file`, `gallery`, `repeater`, `richtext`, `color` (site-code.md); settings of type `secret` (encrypted, never shown again; [plugins.md §1.4](plugins.md)); taxonomies: `site.taxonomy`, `db.terms`, `PostInput.terms`, terms screens and term archives (§3.6); scheduled posts in core (status `scheduled`, `publishedAt`); `db.posts` writes every post field (`authorId`, `publishedAt`, `imageId`, `pinned`, `sortOrder`, `design`) and lists by `status` / `authorId` / `ids`; the store fires the core events for every write, with `actor` and `previous` (§3.4) |

Versions 2.0.1 and 2.0.2 changed no plugin API.

### Requiring a version: `minCore`

```jsonc
{ "id": "my-plugin", "version": "1.0.0", "minCore": "2.0.4" }
```

`minCore` is a semver version (`MAJOR.MINOR.PATCH`). A CMS older than `minCore` refuses to install the
package (`this plugin needs Anasaki Studio 2.0.4 or newer`) and the Plugin Store shows the requirement.
Set it to the newest version tag of any joint you **cannot work without**. For optional features, feature
detection is better: older sites can still install the plugin and just skip the feature.

### Feature detection

Server side (plugin code), check that the function exists before calling it:

```ts
if (typeof site.dashboardWidget === 'function') site.dashboardWidget('visits', { /* … */ });
if (plugins.has('thai')) site.formatter('price', (v) => plugins.call('thai', 'bahtText', v));
```

Browser side (admin pages and widgets with the UI kit):

```js
if (window.AK && AK.pickMedia) {
  pickButton.hidden = false;           // 2.0.4+: the media library dialog
} else {
  urlInput.hidden = false;             // older CMS: let the user paste a URL
}
if (window.AK && AK.line) AK.line(el, opts); else AK.bar(el, values);   // 2.0.3+
```

---

## 1. Manifest

`manifest.json` at the root of the zip. Fields (all since 2.0.0):

| field | type | notes |
|---|---|---|
| `id` | string | `^[a-z0-9][a-z0-9-]{2,39}$`, unique; not `backup` |
| `name` | string | ≤ 100 characters |
| `version` | semver | upgrades compare it |
| `description`, `author`, `license`, `homepage` | string | shown in the admin and the Store |
| `icon` | string | an emoji or a package path (`icon.png`) |
| `category` | string | `marketing seo forms ecommerce content social analytics design utilities security` (Store browsing only) |
| `tags` | string[] | ≤ 20 |
| `minCore` | semver | see above |
| `permissions` | string[] | see §2 |
| `settings` | Field[] | settings form ([plugins.md §1.4](plugins.md)); permission `settings` |
| `head` | string[] | package `.css` / `.js` files added to every public page; permission `head` |
| `bodyEnd` | string | an HTML template appended before `</body>`; permission `bodyEnd` |
| `blocks` | Block[] | designer blocks from templates (or `"render": "service"`); permission `blocks` |
| `webhooks` | `[{event, url}]` | POST on events; permission `webhooks` |
| `service` | object | a service / WASM program ([plugins.md §2](plugins.md)); permission `service` |
| `runtime` | `"js"` | a JavaScript/TypeScript plugin; needs `code` |
| `code` | object | the JS plugin's code (below); permission `code` |
| `requires` | `{id: range}` | must be installed and active ([plugins.md §4.3](plugins.md)) |
| `optionalRequires` | `{id: range}` | "works with"; never blocks |
| `legacy` | string | reserved for the official replacements of the former built-ins (one-time data migration) |
| `capabilities` | Capability[] | what the plugin lets roles do, ticked per role in Users → Roles (§2.1; **since 2.0.6**) |

`code` (JS plugins):

| key | notes |
|---|---|
| `entry` | required, `.ts` / `.js` / `.mjs` under `code/` |
| `scopes` | §2 |
| `hosts` | https hosts `http.fetch` may call (scope `http`) |
| `routes` | public path prefixes the plugin's routes and pages live under (scope `routes`); also reserved for the site (see §8) |
| `library` | a module bundled into plugins that `import` this one |
| `exports` | names callable with `plugins.call` / imports (≤ 50) |
| `hooks` | `{filters: [], events: []}` — the plugin's own hook names (`<id>.…`) |
| `eventAliases` | `{"<id>.event": "other.name"}` — also delivered under the other name (≤ 10) |

## 2. Permissions and scopes

The administrator approves these before installing; an upgrade asking for anything new switches the plugin off
until it is approved again (`reconsent`). All since 2.0.0.

**Manifest permissions:** `head`, `bodyEnd`, `blocks`, `settings`, `webhooks`, `service`, `code`.

**Code scopes** (`code.scopes`; a call outside them throws and logs a warning):

| scope | allows |
|---|---|
| `db` | `db.table / from / docs / transaction / raw`, migrations (own `p_<id>_*` tables only) |
| `content` | `db.posts.list / get`, read-only `core_*` views |
| `content-write` | `db.posts.create / update / delete`, `db.asSystem` |
| `content-fields` | `site.fields`, `site.meta`, `site.setMeta` (post meta `<id>.<key>`, a panel in the post editor) |
| `post-types` | `site.postType` — the plugin's own content types (§3.5; since 2.0.5) |
| `taxonomies` | `site.taxonomy` — the plugin's own taxonomies (§3.6; since 2.1.0) |
| `terms-write` | `db.terms.create / update / delete` — `category` and the plugin's own taxonomies (since 2.1.0) |
| `media` | `media.list / get / read` — the media library (since 2.1.0) |
| `media-write` | `media.upload / update / replace / delete` (since 2.1.0) |
| `files` | `files.*` — the plugin's own files, 100 MB (since 2.1.0) |
| `mail` | `mail.send`; `site.info().adminEmail` |
| `http` | `http.fetch` to `code.hosts` that the administrator also allows |
| `webhooks` | `http.webhook` to any public URL |
| `visitor` | `site.visitor`, `site.visitorGroups` |
| `routes` | `site.route`, `site.page` under `code.routes` |
| `jobs` | `site.job`, `site.cron`, `jobs.enqueue` |
| `admin-pages` | `site.adminPage`, `site.dashboardWidget`, `site.adminNotice` |
| `filters-core` | filters on core points (`post.content`, `page.meta`, `sitemap.urls`, …) |
| `middleware` | `site.middleware`, `site.afterResponse` |
| `head-scripts` | `site.head` / `site.bodyEnd` / `site.block` output inserted unsanitised (treat it like `unfiltered_html`) |

**Service API scopes** (`service.api`): `site`, `mail`, `content`, `users`, `events`, `storage`, `http`
([plugins.md §2.4](plugins.md)).

**Capabilities** a plugin may check with `user.can(cap)` or name in `access`: the core ones (`read`,
`edit_own_posts`, `edit_others_posts`, `publish_posts`, `edit_pages`, `upload_files`, `manage_categories`,
`manage_options`, `manage_users`, `unfiltered_html`, `view_stats`, and the `*_scripts` / `*_code_data` ones) —
plus roles (`administrator`, `editor`, …), `*` (everyone) and `group:<name>` (visitor groups) in `access` lists —
and, since 2.0.6, the plugin's **own** capabilities (§2.1).

### 2.1 Plugin capabilities — since 2.0.6

Every plugin (JS, service or WASM) can declare what it lets people do; the site owner then decides, per role,
who may use what in **Users → Roles** (a section per installed plugin, next to the core capabilities). The core
never knows a plugin's capabilities — they come from the manifest:

```jsonc
"capabilities": [
  { "id": "view", "label": { "th": "ดูสถิติ", "en": "View analytics" },
    "description": { "th": "เปิดหน้าสถิติ", "en": "Open the Analytics page" },
    "default": ["administrator", "editor"] }
]
```

| key | notes |
|---|---|
| `id` | `^[a-z][a-z0-9_]{1,30}$`, unique in the manifest; the capability is **`<plugin id>.<id>`** (`analytics.view`) |
| `label` | `{th, en}` or a string, ≤ 80 characters — shown in the roles editor and the install dialog |
| `description` | optional `{th, en}`, ≤ 300 characters |
| `default` | role slugs (`administrator`, `editor`, `author`, `contributor`, `subscriber` or a custom role), ≤ 20 |

At most 30 capabilities. Lifecycle:

- **Install / upgrade:** a capability the site has never seen is granted to its `default` roles (that exist).
  A capability seen before keeps what the administrator set — an upgrade never re-grants what was taken away.
- **Upgrade dropping a capability, or uninstall:** it leaves every role (their other capabilities stay).
  **Reinstall:** only capabilities never seen before get their defaults.
- **The administrator always has every capability.** A capability counts only while its plugin is installed.
- The install / consent dialog lists the capabilities a plugin adds (`GET /package-plugins` → `capabilities`).

Using them (a plugin may only name **its own** capabilities `<own id>.*` and core capabilities; naming another
plugin's capability throws at registration — or, for `can`, when called):

| where | how |
|---|---|
| JS `access` lists — `site.route`, `site.page`, `site.collection`, `site.action`, `site.adminPage`, `site.dashboardWidget`, `site.adminNotice`, a post type's `access.edit` | `'cap:<capability>'` next to role slugs, e.g. `access: ['cap:analytics.view']` (every role holding it; the Access screen can still override) |
| JS code | `ctx.user.can('analytics.view')`, `site.can(user, 'analytics.view')` (null user → false); `ctx.user.caps` lists the core capabilities and the plugin's own |
| JS post types | `site.postType(id, { caps: { edit: 'docs.edit', publish: 'docs.publish' } })` — §3.5 |
| service / WASM admin pages | `service.admin[].cap` may be `"<own id>.<id>"` |
| service / WASM user context | the `X-Anasaki-User` header of admin screens and `plugins.call` bodies carry `user.caps`: core + the plugin's own |

Feature detection (works on older CMS versions, which ignore `capabilities` and would refuse `cap:` entries):

```ts
const CAPS = typeof (site as any).can === 'function';          // 2.0.6+
site.adminPage('stats', { label: 'Stats', access: CAPS ? ['cap:analytics.view'] : ['editor'], render });
```

## 3. Server joints (JS plugins)

Inside a plugin everything is namespaced: tables `p_<id>_*`, settings `<id>.<key>`, cookies `pl_<id>_*`,
actions `/_a/<id>:<name>`, collections `code:<id>:<name>`, admin pages `<id>:<page>`, jobs `<id>:<job>`.

### 3.1 `site`

| call | scope | since |
|---|---|---|
| `site.route(spec, opts?, handler)` — `"GET /x/{id}"`, `access`, `rawBody`, `maxBody` | `routes` | 2.0.0 |
| `site.page(path, {access, load})` — data for designed pages | `routes` | 2.0.0 |
| `site.collection(name, spec)` — designer data source | — | 2.0.0 |
| `site.action(name, spec)` — `POST /_a/<id>:<name>` (buttons, forms, `anasaki.call`) | — | 2.0.0 |
| `site.on(event, fn)` — core events and other plugins' events | — | 2.0.0 |
| `site.emit(name, data)` — own events `<id>.…` | — | 2.0.0 |
| `site.filter(name, fn, {priority})` · `site.applyFilter(name, value, ctx)` | `filters-core` for core points | 2.0.0 |
| `site.export(name, fn)` — public functions (`code.exports`) | — | 2.0.0 |
| `site.formatter(name, fn)` — used as `{{ v \| <id>:<name> }}` | — | 2.0.0 |
| `site.visitor(fn)` · `site.visitorGroups(groups)` — site members | `visitor` | 2.0.0 |
| `site.fields(type, fields)` · `site.meta(postId)` · `site.setMeta(postId, values)` — type `'*'` = every type since 2.0.8 | `content-fields` | 2.0.0 |
| `site.postType(id, spec)` — a content type, §3.5 | `post-types` | 2.0.5 |
| `site.taxonomy(id, spec)` — a taxonomy (categories, tags, brands …), §3.6 | `taxonomies` | 2.1.0 |
| `site.job(name, opts?, fn)` · `site.cron(expr, target)` | `jobs` | 2.0.0 |
| `site.adminPage(id, spec)` — §5 | `admin-pages` | 2.0.0 |
| `site.dashboardWidget(id, spec)` — §5 | `admin-pages` | 2.0.0 |
| `site.adminNotice(fn, {access})` — §5 | `admin-pages` | 2.0.0 |
| `site.can(user, capability)` — §2.1 | — | 2.0.6 |
| `site.block(type, spec)` — server-rendered designer block | — | 2.0.0 |
| `site.middleware(fn, {priority})` · `site.afterResponse(fn)` — §4 | `middleware` | 2.0.0 |
| `site.head(fn)` · `site.bodyEnd(fn)` — §4 | — (`head-scripts` for raw output) | 2.0.0 |
| `site.settings(key?)` · `site.saveSettings(values)` | — | 2.0.0 |
| `site.info()` → `{title, tagline, url, language, timezone, adminEmail?}` | — | 2.0.0 |
| `site.asset(path)` → `{path, size, type, url, bytes(), text()}` — own package files | — | 2.0.0 |

Before 2.0.5 `site.postType` was Site Code only.

Route/page/action results: a value (JSON), `{status, headers?, body?}`, bytes (`Uint8Array`, ≤ 20 MB),
`{status, headers?, bodyBase64}`, or `{page: {title, html}, status?, headers?}` / `response.page(...)` to show
HTML inside the site's header and footer. Request context: `request {method, path, params, query, headers,
cookies, body, json(), rawBody?, ip, host}`, `response.setCookie(name, value, opts)`, `user`, `visitor`.

### 3.2 `db`

| call | scope | since |
|---|---|---|
| `db.table(name)` / `db.from(name)` — query builder (`where…`, `join`, `orderBy`, `paginate`, `insert`, `update`, `delete`, `increment`, …) | `db` | 2.0.0 |
| aggregate selects `selectCount / selectCountDistinct / selectSum / selectAvg / selectMin / selectMax` | `db` | 2.0.0 |
| `db.transaction(fn)` · `db.raw(sql, params)` (no SELECT/FROM/JOIN fragments) | `db` | 2.0.0 |
| `db.docs(name)` — JSON document collections | `db` | 2.0.0 |
| `db.posts.list / get` | `content` | 2.0.0 |
| `db.posts.list({status, authorId, ids})` — `status` `'draft' / 'publish' / 'scheduled' / 'any'` (only `publish` without the right to see drafts) | `content` | 2.1.0 |
| `db.posts.create / update / delete` · `db.asSystem(fn)` | `content-write` | 2.0.0 |
| `db.terms.list({taxonomy, parent?, search?, postId?, ids?, slugs?, limit?})` · `db.terms.get(idOrSlug, {taxonomy})` | `content` | 2.1.0 |
| `db.terms.create({taxonomy, name, slug?, parent?, description?})` · `update(id, patch)` · `delete(id)` (children move up) | `terms-write` | 2.1.0 |
| `db.posts.create / update` with `terms: {<taxonomy>: (id \| slug)[]}` (replaces that taxonomy's terms); results have `terms: {<taxonomy>: [{id, slug, name, parent}]}` | `content-write` | 2.1.0 |
| `db.posts.create / update` with `authorId`, `publishedAt` (future = scheduled, past = back-dated), `imageId`, `pinned`, `sortOrder` (these three and `authorId` need the right to edit others' posts), `design` (needs `edit_pages`) — post results add `authorId`, `imageId`, `pinned`, `sortOrder`, `createdAt` | `content-write` | 2.1.0 |
| migrations `migrations/NNN_name.ts` with `migration({up, down})` — `schema.create / table / drop / rename / hasTable / hasColumn`, columns incl. `.binary()` / `.collation()` | `db` | 2.0.0 |

Plugins never get `db.query` or `schema.raw`.

### 3.2a Media and files — since 2.1.0

| call | scope | since |
|---|---|---|
| `media.list({kind?, search?, ids?, limit?})` · `media.get(id)` · `media.read(id)` → `Uint8Array` (≤ 20 MB) | `media` | 2.1.0 |
| `media.upload({bytes \| base64 \| url, filename, alt?})` — the site's file types and size limit, content checked against the extension, `media.beforeSave` runs; `url` must be a public http(s) address | `media-write` | 2.1.0 |
| `media.update(id, {alt})` · `media.replace(id, bytes)` (same URL and type; `media.updated` with `replaced: true`) · `media.delete(id)` | `media-write` | 2.1.0 |
| `media.resize(bytes, {maxWidth?, maxHeight?, quality?})` → `{bytes, format, width, height, size}` — JPEG / PNG, down only | — | 2.1.0 |
| `files.put(path, data, {public?})` · `files.get / text / list / delete / url` — private files in the server's data folder, public ones at `/uploads/plugins/<id>/<path>` (images, pdf, audio/video, json, txt, csv, css, woff2); 100 MB per plugin | `files` | 2.1.0 |
| `request.files()` → `[{field, filename, type, size, bytes}]` · `request.form()` — multipart bodies (route `maxBody`) | `routes` | 2.1.0 |

Writing media needs the caller's `upload_files` (their own files, or `edit_others_posts`), or
`db.asSystem`. An image optimiser: `site.on('media.uploaded', (m) => db.asSystem(() => { const r =
media.resize(media.read(m.id), {maxWidth: 1920}); if (r.size < m.size) media.replace(m.id, r.bytes) }))`.

### 3.3 `http`, `mail`, `jobs`, `plugins`, utilities

| call | scope | since |
|---|---|---|
| `http.fetch(url, {method, headers, body, timeout})` → `{status, ok, headers, body, url, json(), text()}` | `http` | 2.0.0 |
| `http.webhook(url, event, data, {secret?})` — queued POST, `X-Anasaki-Signature: sha256=…` | `webhooks` | 2.0.0 |
| `mail.send({to, subject, html?, text?, replyTo?})` → `{ok, cancelled?}` (site SMTP; `mail.beforeSend` filter) | `mail` | 2.0.0 |
| `jobs.enqueue(name, data, {delay, key})` → job id | `jobs` | 2.0.0 |
| `plugins.call(id, fn, ...args)` — a JS plugin's export, or a service/WASM `service.functions` entry (needs `requires`/`optionalRequires`) | — | 2.0.0 |
| `plugins.has(id)` · `plugins.version(id)` | — | 2.0.0 |
| `PluginUnavailable` error (`e.code === 'unavailable'`) | — | 2.0.0 |
| `cache.get / set / delete` · `crypto.random / uuid / hash / hmac / timingSafeEqual / password.*` · `log.*` / `console.*` | — | 2.0.0 |
| `import { x } from '<id>'` (or `'anasaki:<id>'`) — a required plugin's library / exports | — | 2.0.0 |

### 3.4 Events and filters

Core events (`site.on`, service `events`, package `webhooks`) — 2.0.0: `post.published`, `post.saved`,
`post.deleted`, `user.created`, `media.uploaded`. Every other event belongs to the plugin that emits it
(`<id>.…`, or the name it declares in `code.eventAliases`).

Since 2.1.0 the store fires the core events for **every** write — the admin, the REST API, Site Code and
JS plugins (`db.posts`), the scheduler — after the change is committed, so a handler that reads the post
sees it saved. Every payload has `actor: {id, source}` (`id` 0 = no user; `source` `admin`, `api`, `code`,
`plugin:<id>`, `scheduler`, `setup`, `password-reset` or `system`). New in 2.1.0:

| event | when | payload besides the object's fields |
|---|---|---|
| `post.created` | a post / page / custom type entry was created | — |
| `post.updated` | it was saved again | `previous: {title, slug, status, authorId, publishedAt}` |
| `post.statusChanged` | the status changed (WordPress `transition_post_status`) | `from` (`''` for a new post), `to` |
| `post.scheduled` | it was scheduled, or its time moved | — |
| `post.unpublished` | a published post went back to draft or scheduled | — |
| `user.updated` | profile, role or status changed (also bulk actions) | `previous: {email, name, role, status}`, `passwordChanged` |
| `user.roleChanged` | the role changed | `from`, `to` |
| `user.deleted` | a user was deleted | `reassignedTo` |
| `media.updated` · `media.deleted` | alt text changed · a file was deleted | `previous: {alt}` (updated) |
| `term.created` · `term.updated` · `term.deleted` | categories / tags | `previous: {name, slug}` (updated) |
| `settings.updated` | site settings saved in the admin | `keys` (never the values) |
| `plugin.activated` · `plugin.deactivated` | a plugin was switched on / off | `id`, `version` |

Post payloads: `id, type, title, slug, status, url, authorId, publishedAt` (`post.saved` adds `isNew`).
User payloads: `id, email, name, role, status`. Media: `id, filename, url, mime, size, kind, width, height,
alt, authorId`. Terms: `id, taxonomy, name, slug`. Sample content of the setup wizard fires no events.

**Scheduled posts (2.1.0).** A post saved with status `scheduled`, or `publish` with a future `publishedAt`,
waits; the core checks every 30 seconds and publishes it (`post.published`, actor source `scheduler`).
It is not public until then (`db.posts.get` / `list` show it only to users who may edit the type).

Core filters (scope `filters-core`) — 2.0.0: `post.content`, `post.beforeSave`, `page.head`, `page.meta`,
`collection.items`, `action.input`, `action.result`, `mail.beforeSend`, `sitemap.urls`, `robots.txt`;
2.1.0: `media.beforeSave` (`{filename, alt}`, ctx `{mime, size, kind, source}`; return `{error}` to refuse a file).
Once any plugin filters `sitemap.urls` or `robots.txt`, the core serves `/sitemap.xml` and `/robots.txt`
(cached sitemap). Filters run by ascending `priority`, then plugin load order, Site Code last.

### 3.5 Post types — since 2.0.5

```ts
// manifest: "minCore": "2.0.5", "code": { "scopes": ["post-types", "content", "content-write", "routes"], "routes": ["/docs"] }
site.postType('docs_doc', {
  label: { th: 'เอกสาร', en: 'Docs' }, icon: '📘',
  slug: 'docs', public: false,               // the plugin serves /docs/... itself
  supports: ['title', 'content', 'excerpt'],
  fields: { section: 'text', order: { type: 'number', min: 0 } },
  access: { edit: ['editor'] },
});
```

The spec is the Site Code one ([site-code.md](site-code.md) "Post types and fields"): `label`, `singular`, `icon`,
`slug`, `public`, `supports`, `fields`, `access` and, since 2.0.6, `caps`. What differs in a plugin:

- **Who edits and publishes (since 2.0.6)**: `caps: { edit: 'docs.edit', publish: 'docs.publish' }` names the
  plugin's own capabilities (§2.1). `caps.edit` replaces the default role list (`access.edit` becomes
  `['cap:docs.edit']`, or the entry is added to an explicit `access.edit`); `caps.publish` limits publishing —
  without it, whoever edits may publish. `access.edit` also accepts `'cap:<capability>'` entries. The REST
  `/api/v1/posts` endpoints (list, get, create, update, delete, revisions; a published post can only be changed
  by someone who may publish) and `db.posts` writes enforce them; `GET /api/v1/post-types` reports `caps`,
  `canEdit` and `canPublish` for the caller.

- **Type id**: `<plugin id with "-" as "_">_<name>`, within the usual `^[a-z][a-z0-9_]{1,19}$` (≤ 20 characters):
  plugin `docs` → `docs_doc`, plugin `my-shop` → `my_shop_item`. Anything else (also the bare plugin id) fails
  registration — the plugin does not activate. A plugin whose id starts with a digit cannot register types.
- **Slug** (URL prefix): default = the type id with `_` → `-` (`docs-doc`). It may be one of the plugin's own
  `code.routes` prefixes; it is refused when the core reserves it (§8, the admin path, blog / category bases), when
  another installed plugin declares it, or when another type (Site Code's or another plugin's) has that slug.
  The same type id in two places is refused too: the plugin does not activate (or does not load, with the error
  on the Plugins page), and Site Code cannot publish a type id / slug an active plugin already has.
- **`public: false`**: the core does not route `/<slug>` (archive) or `/<slug>/<post-slug>`; the plugin serves the
  pages (`site.route` under its `code.routes`, e.g. `{page: {title, html}}`). The core's post URL
  (`post.url`, admin links) stays `/<slug>/<post-slug>`, so a plugin with `slug: 'docs'` should answer
  `/docs/<post-slug>` (or redirect it). Default `true`: the core routes it like a Site Code type.
- **Fields**: `fields` of the spec are stored with plain keys (`meta.order`, filterable with
  `db.posts.list({where: {'meta.order': …}})`); `site.fields(type, …)` keeps the `<id>.<key>` namespace.
- **Content**: the owning plugin writes its types with `db.posts.create / update / delete` (`content-write`) and
  reads them with `db.posts.list / get` (`content`); Site Code and other plugins may read a plugin's types (not
  write them).
- **Admin**: the type gets a sidebar entry and the list / editor screens (`/content/<type>`), with the fields;
  `GET /api/v1/post-types` and `GET /api/v1/code/bindings` list it with `plugin` (id) and `pluginName`; the REST
  `/api/v1/posts?type=<type>` endpoints, the designer's post list (type picker) and export / import work as for
  Site Code types.
- **Deactivate / uninstall**: the posts stay in the database (also with "delete data"), hidden from the admin
  (the API answers 400 for the type) until the plugin is active again.

### 3.6 Taxonomies — since 2.1.0

```ts
site.postType('shop_product', { label: 'Products', slug: 'products' })
site.taxonomy('shop_brand', {
  label: { th: 'แบรนด์', en: 'Brands' },
  singular: { th: 'แบรนด์', en: 'Brand' },
  postTypes: ['shop_product'],      // 'post', 'page' or content types
  hierarchical: true,               // parent terms (like categories); false = flat (like tags)
  slug: 'brands',                   // archive /brands/<term-slug>; default: the id with '-'
  public: true,                     // false = no archive pages
  access: { manage: ['editor'] },   // who manages the terms; default editors + administrators; 'cap:<capability>' works
})
```

- A plugin's taxonomy ids follow the post type rule: `<plugin id with _>_<name>`, 2–20 characters.
- The admin gets a **terms screen** (`/terms/<id>`, in the sidebar for users who may manage it) with parent and
  description, and a **panel in the post editor** of each listed type. `GET /api/v1/taxonomies` lists
  `category` and the registered ones with `canManage`; REST posts take `terms: {<taxonomy>: [ids]}`.
- Public taxonomies have archive pages `/<slug>/<term-slug>` with the posts having the term or one of its
  children; the site's archive template is used when there is one (its "this archive's posts" list shows them),
  and the sitemap lists the terms in use.
- `db.terms` writes are limited to `category` and the code's **own** taxonomies; the caller needs the right to
  manage the terms (`manage_categories` for `category`) or `db.asSystem`. Terms fire `term.created / updated /
  deleted`.
- `PostInput.terms` accepts ids or slugs of existing terms; a taxonomy must list the post's type.

## 4. Page hooks (public site) — since 2.0.0

| hook | runs | limit |
|---|---|---|
| `site.middleware(fn, {priority})` | before routing, for every public request; return nothing to continue, a result to answer, or `{headers}` to add allowed headers and continue | 20 ms |
| `site.afterResponse(fn)` | after the response, in a queue; `{method, path, query, status, contentType, referrer, userAgent, ip, durationMs, title, isAdminUser, at, host, headers}` | 500 ms |
| `site.head(fn(page))` / `site.bodyEnd(fn(page))` | HTML in `<head>` / before `</body>`; sanitised without `head-scripts` | 50 ms |
| filter `page.meta` | `{title, fullTitle, description, image, canonical, robots, noindex, og}` | 50 ms |
| package `head` / `bodyEnd` | static assets / an escaped template from the manifest | — |

`page` = `{type, id, slug, title, description, url, path, image, lang, isHome, archiveType, status, siteTitle,
siteTagline}`. Inside these hooks `site.meta(page.id)` is answered from memory.

## 5. Admin pages, dashboard widgets and notices — since 2.0.0

```ts
site.adminPage('orders', { label: { th: 'ออเดอร์', en: 'Orders' }, icon: '🧾', access: ['editor'],
  render: (ctx) => ({ title: 'Orders', html: '<header class="ak-page-head"><h1>Orders</h1></header>…' }),
  download: (ctx) => ({ status: 200, headers: { 'Content-Type': 'text/csv' }, body: csv }) });
site.dashboardWidget('visits', { label: 'Visits', icon: '📈', access: ['editor'], height: 220,
  render: (ctx) => '<div data-ak-spark="[1,3,2,5]"></div>' });
site.adminNotice((ctx) => ({ level: 'warning', text: { th: '…', en: '…' }, link: { label: 'Fix', href: '/admin/x/code/my-plugin:settings' } }));
```

- **Admin pages** appear in the admin sidebar at `/admin/x/code/<id>:<page>` and run in a sandboxed iframe
  (`/ext-ui/code/<ticket>/<page>`, 8-hour ticket bound to the user). `ctx` = `{user, query, request, page,
  locale}`. Inside the page `window.anasaki.call(action, input)` runs your `site.action` as that user,
  `anasaki.download(query)` / `anasaki.downloadUrl(query)` serve `download(ctx)`. `site.saveSettings` works
  here (users with `manage_options`). A plugin's page has no admin header: draw `<header class="ak-page-head">`.
- **Dashboard widgets** (≤ 10 per plugin) are rendered the same way at `/ext-ui/code/<ticket>/widget/<id>`;
  `height` is the initial frame height (80–2000 px; since 2.0.3 the kit resizes it to the content).
- **Notices** (≤ 5 per plugin, 300 ms): `level` `info|warning|danger|success`, `text` ≤ 500 characters, optional
  `link {label, href}` (a site path or http(s) URL). Shown at the top of the admin.
- Access: roles in `access` (default administrator only); the administrator can override them on
  Site code → Access (kind `adminPage`).

Service plugins declare admin screens in `service.admin` instead ([plugins.md §2](plugins.md)).

## 6. Admin UI kit (`window.AK`)

Every code admin page and dashboard widget links `/ext-ui/kit.css` and `/ext-ui/kit.js` and has
`<body class="ak">` and `<html lang="th|en">`. CSS classes `ak-*` (cards, buttons, badges, tables, forms, tabs,
toasts, dialogs, stat tiles, charts) follow light/dark. A live demo of every component is
`backend/internal/sitecode/kit/kit-demo.html`.

| function | since | |
|---|---|---|
| `AK.lang()` | 2.0.0 | `'th'` or `'en'` (from `<html lang>`) |
| `AK.t(key)` · `AK.messages` | 2.0.0 | the kit's own strings |
| `AK.toast(msg, kind?, ms?)` | 2.0.0 | `kind` `success|error|warning|info` |
| `AK.confirm(text, {ok, cancel, danger})` | 2.0.0 | → `Promise<boolean>` |
| `AK.tabs(el)` | 2.0.0 | `[data-tab=x]` buttons show `[data-tab-panel=x]` → `{show}` |
| `AK.table(el, {columns: [{key, label, sort, num, render}], rows, empty})` | 2.0.0 | → `{set}` |
| `AK.sortable(table)` | 2.0.0 | `th[data-sort]` sorts rows |
| `AK.pager(el, {page, pages, onChange})` | 2.0.0 | → `{set}` |
| `AK.validate(form)` | 2.0.0 | → `boolean`, messages in `.ak-field .ak-error` |
| `AK.bar(el, data, {height})` | 2.0.0 | numbers or `[{label, value}]` |
| `AK.sparkline(el, data, {width, height})` | 2.0.0 | |
| `AK.init()` | 2.0.0 | runs on load: initialises the `data-ak-*` attributes below |
| `AK.line(el, {labels, series: [{label, color, values}], height, locale, tableLabel, chartLabel, dateLabel})` | 2.0.3 | line chart with ticks, tooltip, legend, chart ⇄ table → `{set, el}` |
| `AK.open(path)` | 2.0.3 | opens an admin path (`/admin/x/code/<id>:<page>?…`) in the admin window |
| `AK.height()` | 2.0.3 | reports the document height now (called automatically on load and resize) → px |
| `AK.pickMedia({accept: 'image' \| 'file', timeout})` | 2.0.4 | the admin's media library dialog (with upload) → `Promise<{url, id, alt, width, height} \| null>`; `null` when closed, not allowed (`upload_files`), timed out (default 30 min) or outside the admin frame |

Declarative attributes: `data-ak-tabs`, `table[data-ak-sortable]`, `form[data-ak-validate]`, `data-ak-bar`,
`data-ak-spark` (2.0.0); `data-ak-line` (JSON options), `<a href="/admin/…" data-ak-open>` (2.0.3).

```html
<input type="hidden" name="image" id="image">
<button type="button" id="pick" hidden>Choose image</button>
<script>
  if (window.AK && AK.pickMedia) {
    pick.hidden = false;
    pick.onclick = async () => { const m = await AK.pickMedia({ accept: 'image' }); if (m) image.value = m.url; };
  }
</script>
```

## 7. Iframe messages

Admin pages and widgets are sandboxed iframes with an opaque origin. The kit talks to the admin with
`postMessage`; the admin matches each message to its own iframe by window identity and ignores anything else.
You normally use the `AK` functions; the messages are documented for pages that do not load the kit.

| message (frame → admin) | since | effect |
|---|---|---|
| `{type: 'ak:height', height}` | 2.0.3 | sizes the frame: widgets 80–2000 px, admin pages grow to their content |
| `{type: 'ak:navigate', path}` | 2.0.3 | opens `path` in the admin if it is an admin path (`/admin/…` or the custom admin path); anything else is ignored |
| `{type: 'ak:pick-media', reqId, accept: 'image' \| 'file'}` | 2.0.4 | opens the media library; `reqId` `^[\w.:-]{1,64}$` |
| `{type: 'ak:pick-media:result', reqId, media}` (admin → frame) | 2.0.4 | the answer: `media` = `{url, id, alt, width, height}` or `null` |

## 8. Public URLs and reserved paths

A plugin's public URLs are what it **declares**: `code.routes` (JS) or `service.public` (service/WASM). The
core keeps a list of its own paths (`api uploads download plugin-assets ext-ui setup _a _c sitemap.xml robots.txt
favicon.ico favicon.svg assets static`, plus the admin path and the blog / category bases) that no plugin may
declare. Since 2.0.4 the paths every installed plugin declares are reserved for the site too: the admin path and
the URL bases cannot be set to them, and
`GET /api/v1/settings/reserved-paths` (`manage_options`) → `{items: [{path, plugin?}]}` lists them (`plugin`
absent = the core). Before 2.0.4 a fixed list was used instead.

## 9. Store-related behaviour

- **Recommended plugins** (since 2.0.4): the setup wizard offers what the configured Plugin Store marks as
  recommended (`GET {STORE}/api/v1/plugins?recommended=1` — store admins set `recommended`,
  `recommendedChecked`, `recommendedOrder`), official packages only. The core names no plugin; an unreachable
  Store, or one that recommends nothing, just ends the wizard.
- **Official** means the Store's `official` flag plus `"author": "Anasaki Studio"` in the package; it grants no
  extra API.
- **Capabilities** (since 2.0.6): the Store (1.2.2+) validates `capabilities` with the same rules (§2.1) and shows
  them on the plugin page and in the reviewer report.

---

Missing something? Every joint above was added because a plugin needed it, and the same path is open to you:
describe the use case and the smallest generic joint that would cover it.
