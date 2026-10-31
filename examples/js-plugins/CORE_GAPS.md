# Core gaps found while building the sample JS plugins

The samples in this folder use public core joints only. Where a sample needed something core does not
offer, it either works around it **through public APIs** (documented in its README) or leaves the feature
out. Each entry: what is missing, why a plugin needs it, and a proposed generic joint.

## 1. Plugins cannot register binding formatters

✅ **resolved in phase 6:** `site.formatter` works in plugins and is used as `| <id>:<name>` (`{{ price | thai:baht }}`;
the Go and editor evaluators share the `ns:name` rule and vectors). Administrators enable unprefixed aliases per formatter
(`GET /api/v1/code/formatters`, `PUT /api/v1/code/formatters/aliases`, option `site_code_formatter_aliases`); Site Code's own
formatter wins, then the first enabled alias. No scope needed.

- **What:** `site.formatter` is Site Code only (a plugin calling it fails to activate; export-as-plugin warns).
- **Why:** `thai` should provide `| baht`, `| bahtText`, `| thaiDate`, `| thaiNumber` by itself. Today the library
  exports `registerFormatters()` and Site Code must call it — one line, but every site has to know about it.
- **Proposed joint:** allow `site.formatter` in plugins, namespaced like other registrations (`| thai:bahtText`,
  plus an optional unprefixed alias the administrator can enable per plugin on the Access/Settings screen to
  avoid collisions). No new scope needed (formatters are pure; they already run with a 50 ms limit).

## 2. Route responses cannot carry binary bodies

✅ **resolved in phase 6:** `body` may be a `Uint8Array` / `ArrayBuffer` (written verbatim, default
`application/octet-stream`) or the result may use `bodyBase64`; ≤ 20 MB. Same for webhook replies and admin page downloads;
the test-run API returns `bodyBase64`.

- **What:** a route's `body` is a JS string written as UTF-8 (`program.go` `response()`), and a `Uint8Array` is
  serialised as JSON. Bytes ≥ 0x80 cannot be sent as is.
- **Why:** `files.toXLSX` produces a zip; `files` would also like to send images, PDFs with binary streams
  (compressed / embedded fonts), ZIP archives. The sample falls back to `xlsxPayload()` → JSON base64 decoded in
  the browser, and keeps its PDFs pure ASCII.
- **Proposed joint:** accept `body: Uint8Array | ArrayBuffer` (written verbatim), and/or
  `{ status, headers, bodyBase64: string }` in `RouteResponse`; same for `site.adminPage` download results (see 3).

## 3. No authenticated download from the admin

✅ **resolved in phase 6:** `site.adminPage(id, { render, download(ctx) })`, served at
`GET /ext-ui/code/<ticket>/<id>/download?…` with the page's ticket and access, `Content-Disposition: attachment` enforced;
`anasaki.download(query)` / `anasaki.downloadUrl(query)` inside the page (the iframe sandbox gained `allow-downloads`).

- **What:** admin-only routes see `ctx.user` only with an `Authorization: Bearer` header, which a browser link does
  not send; admin pages are served in a sandboxed iframe without `allow-downloads` and can only return HTML.
- **Why:** an "Export CSV/XLSX/PDF" button on an admin page (admin-crud, members, shop orders) is the most common
  export UI. Today exports must be public routes behind a visitor group or a home-made token.
- **Proposed joint:** `site.adminPage(id, { render, download?(ctx) => RouteResult })` served at
  `/ext-ui/code/<ticket>/<id>/download?…` with the same ticket auth and role check (top-level navigation, so no
  sandbox issue), or add `allow-downloads` to the sandbox together with ticket-authenticated
  `/ext-ui/code/<ticket>/_r/<route>` calls of the unit's own routes.

## 4. Plugins cannot read their own package assets at runtime

✅ **resolved in phase 6:** `site.asset(path)` → `{path, size, type, url, bytes(), text()}` for any file of the
calling plugin's package except `code/`, `migrations/`, `manifest.json` (clean relative paths, ≤ 5 MB, cached per version);
packages may contain `.ttf` / `.otf`. Site Code: throws (use the media library).

- **What:** code can import only `.ts .js .mjs .json .md` under `code/`; other package files (fonts, images,
  templates) are not readable from code.
- **Why:** Thai PDF output needs an embedded TrueType font (Sarabun/Noto Sans Thai, ~100 KB+, with a ToUnicode
  map); `files.toPDF` therefore supports Latin-1 only. Mail templates / images would benefit too.
- **Proposed joint:** `site.asset(path) → { bytes: Uint8Array, text(): string, url: string }` for files of the
  calling plugin's package (read-only, size-capped), with `url` pointing at the existing `/plugin-assets/<id>/…`.

## 5. No dynamic call of a JS plugin's exports

✅ **resolved in phase 6:** `plugins.call('<jsPluginId>', '<export>', ...args)` calls names in the target's
`code.exports` with import-proxy semantics, without a `requires` entry; a missing / inactive target throws `PluginUnavailable`
(`code: 'unavailable'`). `plugins.has(id)`, `plugins.version(id)`; manifest `optionalRequires` ("works with", version warning
only, never blocks).

- **What:** another plugin's `code.exports` are reachable only through a static `import` (which needs a
  `requires` entry and fails activation when the target is missing). `plugins.call(id, fn)` reaches WASM /
  service plugins only.
- **Why:** `admin-crud` was meant to also manage data owned by other plugins through their exports
  (`list/get/save/delete`) chosen in a JSON config; that needs a runtime lookup. It also rules out optional
  integrations (e.g. `shop` using `thai` when installed).
- **Proposed joint:** let `plugins.call('<jsPluginId>', '<exportName>', ...args)` call a JS plugin's declared
  export (same JSON copy, time accounting and error mapping as import proxies), plus `plugins.has(id)` so code can
  degrade gracefully; keep the `requires` check optional for this call (or a manifest `optionalRequires`).

## 6. Minor

✅ **resolved in phase 6:** Access-screen overrides for admin pages (`kind: "adminPage"`, admin roles only) and
`ctx.locale` (`'th' | 'en'`, the signed-in user's admin language) in admin page render / download / actions.

- **Bootstrap-time settings:** `site.adminPage` / `site.action` access lists are fixed at registration; a plugin
  cannot derive them from its own settings (e.g. "members admin page for editors"). The Access screen covers
  actions; admin pages have no override. Proposed: Access-screen overrides for admin pages (`kind: adminPage`).
- **Admin page locale:** the render ctx has no UI locale, so samples use a `lang` setting / `?lang=en`. Proposed:
  `ctx.locale` (`th` | `en`) from the signed-in user's admin language, like `X-Anasaki-User.locale` for service
  plugins.
