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
plus roles (`administrator`, `editor`, …), `*` (everyone) and `group:<name>` (visitor groups) in `access` lists.

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
| `site.fields(type, fields)` · `site.meta(postId)` · `site.setMeta(postId, values)` | `content-fields` | 2.0.0 |
| `site.job(name, opts?, fn)` · `site.cron(expr, target)` | `jobs` | 2.0.0 |
| `site.adminPage(id, spec)` — §5 | `admin-pages` | 2.0.0 |
| `site.dashboardWidget(id, spec)` — §5 | `admin-pages` | 2.0.0 |
| `site.adminNotice(fn, {access})` — §5 | `admin-pages` | 2.0.0 |
| `site.block(type, spec)` — server-rendered designer block | — | 2.0.0 |
| `site.middleware(fn, {priority})` · `site.afterResponse(fn)` — §4 | `middleware` | 2.0.0 |
| `site.head(fn)` · `site.bodyEnd(fn)` — §4 | — (`head-scripts` for raw output) | 2.0.0 |
| `site.settings(key?)` · `site.saveSettings(values)` | — | 2.0.0 |
| `site.info()` → `{title, tagline, url, language, timezone, adminEmail?}` | — | 2.0.0 |
| `site.asset(path)` → `{path, size, type, url, bytes(), text()}` — own package files | — | 2.0.0 |

`site.postType` is Site Code only.

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
| `db.posts.create / update / delete` · `db.asSystem(fn)` | `content-write` | 2.0.0 |
| migrations `migrations/NNN_name.ts` with `migration({up, down})` — `schema.create / table / drop / rename / hasTable / hasColumn`, columns incl. `.binary()` / `.collation()` | `db` | 2.0.0 |

Plugins never get `db.query` or `schema.raw`.

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

Core filters (scope `filters-core`) — 2.0.0: `post.content`, `post.beforeSave`, `page.head`, `page.meta`,
`collection.items`, `action.input`, `action.result`, `mail.beforeSend`, `sitemap.urls`, `robots.txt`.
Once any plugin filters `sitemap.urls` or `robots.txt`, the core serves `/sitemap.xml` and `/robots.txt`
(cached sitemap). Filters run by ascending `priority`, then plugin load order, Site Code last.

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

---

Missing something? Every joint above was added because a plugin needed it, and the same path is open to you:
describe the use case and the smallest generic joint that would cover it.
