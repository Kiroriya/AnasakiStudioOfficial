# Site Code (phases 1-6)

Site Code is the site's own **server-side JavaScript / TypeScript**, the Anasaki Studio take on
WordPress' `functions.php`. Files live in the CMS database, are bundled with
[esbuild](https://esbuild.github.io/) and run in the [goja](https://github.com/dop251/goja) sandbox
inside the CMS. Phase 1 covers custom routes on the public site, core events, read access to posts,
logging, an in-memory cache and crypto helpers. Phase 2 adds the site's own database: migrations, a
query builder, `db.docs` and the **Database** screen (see "The site's own database" below). Phase 3
connects code to the designer: collections, actions, page data, formatters, expressions and conditions in
bindings, pagination and inputs, and the **Access** screen. Phase 4a adds hooks, visitors, post types,
mail / http / secrets, jobs and admin pages; phase 4b JavaScript plugins. Phase 5 adds developer tools:
personal access tokens, the `anasaki-code` CLI ([docs/cli.md](cli.md)), the **Frontend code** designer
block and `window.anasaki` on public pages. Phase 6 adds plugin formatters (`| thai:baht`) and aliases,
binary bodies, admin page downloads, package assets, `plugins.call` to JS exports, and the page joints
`site.middleware`, `site.afterResponse`, `site.head`, `site.bodyEnd`, `page.meta`, `sitemap.urls` plus the
admin UI kit (see "Plugin joints and page hooks").

Admin screens: **Site code** (editor), **Versions**, **Logs**, **Database**, **Access**, **Settings**,
**Jobs**, **Hooks**. The REST API is under `/api/v1/code` (see the table at the end).

**Contents:** [Getting started with VS Code + CLI](#getting-started-with-vs-code--cli) ·
[Project layout](#project-layout) · [Script API](#script-api) ·
[Database](#the-sites-own-database-phase-2) · [Designer](#connecting-code-to-the-designer-phase-3) ·
[Hooks, services, jobs](#hooks-visitors-post-types-services-jobs-and-admin-pages-phase-4a) ·
[JS plugins](#javascript-plugins-units-and-interop-phase-4b) ·
[Frontend code + window.anasaki](#frontend-code-block-and-windowanasaki-phase-5) ·
[Tokens and sync API](#personal-access-tokens-and-the-sync-api-phase-5) · [Limits](#limits-at-a-glance) ·
[Capabilities](#capabilities) · [REST API](#rest-api-apiv1code-bearer-auth) ·
[Troubleshooting](#troubleshooting)

## Getting started with VS Code + CLI

1. **Admin → Profile → API tokens / โทเค็นสำหรับเครื่องมือ** → *Create*: give it a name (e.g. "laptop"),
   pick an expiry; scope `code`. The token (`ast_…`) is shown **once** — copy it.
2. With Node.js ≥ 20:
   ```sh
   npx anasaki-code login https://example.com      # paste the token
   mkdir my-site && cd my-site
   npx anasaki-code init https://example.com       # anasaki.json, tsconfig.json, .anasakiignore, then pull
   code .
   ```
3. VS Code reads `anasaki.d.ts` through `tsconfig.json`: every global (`site`, `db`, `log`, `cache`,
   `crypto`, `mail`, `http`, `jobs`, `plugins`…) and the modules of active JS plugins
   (`import { bahtText } from 'thai'`) are completed and type-checked. Refresh it with
   `anasaki-code types` after migrations (row types) or plugin changes.
4. `npx anasaki-code watch` pushes every save to the server **draft** (300 ms debounce) and prints build
   problems as `file:line:col: error: message` (VS Code problem matcher in [docs/cli.md](cli.md)).
   `npx anasaki-code publish -m "message"` makes the draft live (pending migrations are listed and
   destructive ones need a confirmation).
5. The web editor and the CLI edit the same draft. The CLI never overwrites a file someone changed on the
   server since your last pull — it reports a conflict instead ([docs/cli.md](cli.md#conflicts)).

Your role still decides what a token may do: `view_scripts` to pull, `edit_scripts` to push,
`publish_scripts` to publish.

## Project layout

```
main.ts          ← entry point: main.ts, else main.js, else index.ts, else index.js
events.ts        ← other files: import { x } from './events'
lib/format.ts
data.json        ← JSON imports work (import data from './data.json')
README.md        ← .md files are stored (and importable as text)
anasaki.d.ts     ← type definitions (Download all / GET /api/v1/code/types)
```

- Allowed extensions: `.ts .js .mjs .json .md`. At most 200 files, 512 KB per file, 2 MB in total.
- Paths: relative, `/`-separated, segments of `A-Z a-z 0-9 . _ -`, no `..`, at most 255 characters.
  Names that differ only by case are refused.
- Imports: relative (`./x`, `../lib/y`; the extension and `/index.ts` may be omitted) and active JS
  plugins (`import { money } from 'money'`, phase 4b). There is no `node_modules`: copy an ES-module
  library into `lib/` instead.

## Draft → publish

- **Save** writes the draft (`PUT /files/{path}`, with `baseUpdatedAt` for conflict detection; a stale
  base answers 409 with the current file). The editor then calls `POST /build`, which bundles the draft
  and runs the *registration* and reports problems with `file:line:column`.
- **Publish** (`publish_scripts`) bundles the draft again, runs the registration, validates what was
  registered and stores a version snapshot (files, bundle, source map, manifest). Any problem → the
  publish is refused and the live version keeps running. `baseVersion` must be the live version id
  (`null` when none): someone else published meanwhile → 409.
- **Versions**: the newest 100 are kept. *Restore* makes an old snapshot live as a new version
  ("Restore #id") and can also replace the draft with it.
- **Upload** a project `.zip` (a single top folder is stripped; `anasaki.d.ts` at the root and OS junk
  such as `__MACOSX/` are ignored) or a single file; `?dryRun=1` returns the diff first,
  `mode=replace` removes draft files missing from the zip. **Download** gives the draft as a zip
  (plus `anasaki.d.ts`), a folder as a zip or a single file — upload/download round-trips exactly.

## Script API

```ts
// Routes on the public site
site.route('GET /hello/{name}', { access: ['*'] }, ({ request, user }) => ({ hi: request.params.name }));
site.route('GET /feed.txt', { access: ['*'] }, () => ({ status: 200, headers: { 'Content-Type': 'text/plain' }, body: 'ok' }));
site.route('GET /files/*', { access: ['*'] }, ({ request }) => request.params['*']);   // trailing wildcard
site.route('POST /api-hook', ({ request }) => request.json());                         // default: administrator only

// Core events (asynchronous, after the fact)
site.on('post.published', (post) => log.info('published', post.title, post.url));

// Inside handlers
db.posts.list({ type: 'post', category: 'news', search: 'x', order: 'newest', page: 1, perPage: 10 }); // { items, total }
db.posts.get('hello-world');                // by slug (or id) → Post | null
cache.set('key', { any: 'json' }, 60); cache.get('key'); cache.delete('key');
crypto.hash('sha256', 'data'); crypto.hmac('sha256', 'key', 'data'); crypto.random(16); crypto.uuid();
crypto.timingSafeEqual(a, b); crypto.password.hash(pw); crypto.password.verify(pw, hash);
log.debug/info/warn/error(...args); console.log(...args);   // → the Logs screen
```

`anasaki.d.ts` documents every function and type (English with Thai one-liners).

### Routes

- Spec: `"METHOD /path"`, methods `GET POST PUT PATCH DELETE` (GET also answers HEAD). Path parameters
  `{name}`, a trailing `{name...}` or `/*` (→ `params['*']`). `/` is allowed (it replaces the home page);
  a trailing `/` is not.
- Reserved (refused at publish, and never handed to Site Code at request time): `/api`, `/admin` and
  the configured admin path, `/uploads`, `/download`, `/plugin-assets`, `/ext-ui`, `/sitemap.xml`,
  `/robots.txt`, `/favicon.ico`, the blog and category bases, and paths of active service plugins.
  Duplicate routes (same method and shape) and conflicting patterns are refused too.
- `access`: `['*']` = anyone; otherwise role slugs (the administrator always passes). **Default:
  administrator only.** No/invalid session → 401, wrong role → 403.
- `ctx.user` is the CMS user when the request carries a valid admin session as
  `Authorization: Bearer <token>` (the same token the admin app uses); otherwise `null`. The
  `Authorization` header itself is never shown to scripts.
- Return value: object/array → JSON 200; string → `text/html` 200; `{ status, headers?, body? }`
  (no other keys) → as given; `undefined`/`null` → 204. Async handlers are awaited.
- `ctx.request`: `method, path, params, query, headers` (lower-case), `cookies`, `body` (string, 1 MB
  max) and `json()`.
- A route that throws or times out answers a generic 500 / 504 (JSON when the client sends
  `Accept: application/json`, else a small HTML page) — never the stack. The Logs screen shows the
  message and the source-mapped stack (`lib/x.ts:12:5`).

### Events

`post.published`, `post.saved`, `post.deleted`, `user.created`, `media.uploaded`, `form.submitted`
(forms plugin) and any event a service plugin emits (`<plugin>.<event>`). Handlers run after the fact,
each in its own VM, 10 s limit; failures are logged.

Since 2.1.0 the core fires its events for every write, whoever makes it (admin, REST API, Site Code, JS
plugins, the scheduler), each with `actor: {id, source}`; updates carry `previous`. Added: `post.created`,
`post.updated`, `post.statusChanged`, `post.scheduled`, `post.unpublished`, `user.updated`,
`user.roleChanged`, `user.deleted`, `media.updated`, `media.deleted`, `term.created`, `term.updated`,
`term.deleted`, `settings.updated` (changed keys only), `plugin.activated`, `plugin.deactivated` — see
`docs/plugin-api.md` §3.4 for the payloads.

```ts
site.on('post.updated', (p) => {
  if (p.previous.slug !== p.slug) log.info('moved', p.previous.slug, '→', p.slug, 'by', p.actor.source);
});
```

### db.posts

Read-only in phase 1. Anonymous callers (and event handlers) see **published** posts/pages only; drafts
are visible to `ctx.user` when the role may edit that type (`edit_others_posts` for posts,
`edit_pages` for pages). `content` is sanitised HTML. Post fields: `id, type, title, slug, status, url,
excerpt, content, image, category, categories[{slug,name}], author, publishedAt, updatedAt`.

### What is not available (phase 1)

No `require`, no npm packages, no `fs`/`net`/`process`, no `fetch`, no `setTimeout`/`setInterval`
(calling them throws). Because there are no timers, a promise that waits for nothing settles at once;
a promise that never settles is reported as an error. `db`, `cache` and `crypto` work only inside
handlers — calling them at the top level (registration) is an error.

## JavaScript support (goja)

The bundle is built with esbuild for **ES2022**. goja runs natively: `let/const`, arrow functions,
classes (private `#fields`, static blocks, `#x in obj`), `async`/`await`, generators, destructuring,
spread, optional chaining `?.`, nullish `??` and `??=`, BigInt, exponent `**`, `Array.prototype.at`,
`findLast`, `Object.hasOwn`, `Object.fromEntries`, `String.prototype.replaceAll`, `Promise.allSettled`,
`Proxy`/`Reflect`, `Symbol.asyncIterator`, `for await`, `Error` `cause`, RegExp named groups and
lookbehind.

esbuild lowers what goja lacks: **async generators** (`async function*`). Not available: the RegExp
`d` flag (match indices), `Intl`, `structuredClone`, `TextEncoder/Decoder`, `WeakRef`,
`queueMicrotask`. `toLocaleString` / `toLocaleDateString` / `toLocaleTimeString` format plainly without
`Intl` (2.1.1): numbers get `,` grouping, `minimumFractionDigits` / `maximumFractionDigits`, `style: 'percent'`
and `'currency'`; dates follow the locale's order — `th` gives day/month/Buddhist year (`3/11/2569`), with Thai
and English names for `{ month: 'long' | 'short', weekday }` — and `timeZone: 'UTC'`; other `Intl` options are
ignored (no other time zones). Before 2.1.1 `(1234).toLocaleString('th-TH')` threw a RangeError.

## Runtime and safety

- **Isolation**: every invocation gets a fresh VM loaded from the compiled bundle (a few are pre-warmed);
  nothing leaks between requests except what you put in `cache` (in-memory, per server process,
  JSON values, 10 MB, oldest evicted first).
- **Limits**: routes 2 s, events 10 s (stopped with `vm.Interrupt`), call stack depth 2000, at most 8
  invocations at once — extra requests wait up to 2 s, then 503. Memory is not metered in phase 1.
- **Circuit breaker**: a route/event handler failing 20 times within a minute is switched off for
  5 minutes (logged as an error, shown as `lastError` in `GET /status`).
- **Safe mode** (Site code screen, `publish_scripts`) or env **`SITE_CODE_DISABLED=true`** (when the
  admin itself is unreachable) or **`DEMO_MODE`**: no Site Code runs at all — routes fall through to the
  normal site, events are ignored. In demo mode every `/api/v1/code` endpoint except `GET /status`
  answers 403.
- **Performance**: a public request costs nothing extra when nothing is published or no route matches
  (the check runs after plugin public routes and before page routing, using the options the site has
  already loaded).
- **Logs** (`script_logs`): 7 days / newest 10,000 rows; filter by level, source (prefix, e.g.
  `route GET /hello`, `event post.published`, `publish`) and text; download as `.txt`.
- **Audit** (`script_audit`): save, delete, move, upload, publish, restore, safe_mode, clear_logs.

## The site's own database (phase 2)

Scripts get their own MySQL tables, designed with migrations and queried with a synchronous query
builder. Admin screen: **Site code → Database**.

### Separate database and user (recommended)

```
MySQL
├── anasaki        CMS tables. The code user has NO privileges here.
└── anasaki_code   developer tables + read-only core_* views. Code user: ALL on this database only.
```

- `CODE_DB_DSN` — the code user on the code database, e.g.
  `anasaki_code:<password>@tcp(mysql:3306)/anasaki_code?parseTime=true&charset=utf8mb4&loc=UTC`.
  Migrations, `db.*` calls and the SQL console run as this user, so MySQL itself stops them from
  touching CMS tables (no SQL text filtering involved).
- The CMS user (`DB_DSN`) also needs `ALL` on the code database: it creates the `core_*` views, the
  bookkeeping tables (`script_migrations`, `script_table_backups`, `script_relations`), backup copies
  and serves the admin screens.
- `core_*` views (created by the CMS user at startup and after every publish/rollback,
  `SQL SECURITY DEFINER`, `ALGORITHM=TEMPTABLE` so they are never updatable):
  `core_posts(id, type, title, slug, status, excerpt, author_id, featured_media_id, published_at,
  created_at, updated_at)` — published only; `core_terms(id, taxonomy, name, slug)`;
  `core_post_terms(post_id, term_id)` — of published posts; `core_users(id, name, email, role,
  status, created_at)` — never password hashes or tokens. The code user may drop a view (it is
  recreated) but cannot redefine one over CMS tables (MySQL requires SELECT on them).
- **docker compose** creates both on the first start of the MySQL volume
  (`docker/mysql-init/10-site-code.sh`, needs `CODE_DB_PASSWORD`) and passes `CODE_DB_DSN` to the CMS.

#### Existing installs

Init scripts only run on an empty data volume. On a site installed before, run as MySQL root
(`docker compose exec mysql mysql -uroot -p`), with your own password:

```sql
CREATE DATABASE IF NOT EXISTS `anasaki_code` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'anasaki_code'@'%' IDENTIFIED BY '<password>';
GRANT ALL PRIVILEGES ON `anasaki_code`.* TO 'anasaki_code'@'%';
GRANT ALL PRIVILEGES ON `anasaki_code`.* TO 'anasaki'@'%';
```

then set `CODE_DB_PASSWORD=<password>` in `.env` (the compose file builds `CODE_DB_DSN` from it) and
`docker compose up -d`. The Database screen shows these statements (with your database and user
names) while the site runs in fallback mode.

### Fallback mode

Without `CODE_DB_DSN` (or when it does not work — the reason is logged and shown as `error` in
`GET /db/status`) developer tables live in the CMS database with the prefix `sc_` (`orders` →
`sc_orders`, `kv_docs` → `sc_kv_docs`, backups `sc_bak_…`); the builder adds the prefix and keeps the
logical name usable as a qualifier (`orders.id`). Migrations, the query builder, `db.docs`, the admin
data screens, backups and rollback work. **Disabled**: `db.query`, `db.raw`, `schema.raw` and the SQL
console (they would run as the CMS user), and the `core_*` views (use `db.posts`). The mode is chosen
at startup; moving from fallback to separate does not move existing `sc_` tables (dump → import).

### Migrations

```ts
// migrations/001_create_orders.ts
export default migration({
  up(schema) {
    schema.create('orders', (t) => {
      t.id();                                                   // BIGINT UNSIGNED AUTO_INCREMENT PK
      t.string('code', 20).unique();
      t.foreignId('customer_id').references('customers').onDelete('restrict');
      t.foreignId('user_id').nullable().references('core_users'); // metadata only (views cannot be FK targets)
      t.enum('status', ['pending', 'paid']).default('pending');
      t.decimal('total', 12, 2);
      t.json('shipping').nullable();
      t.timestamps();                                           // created_at, updated_at DATETIME(3)
      t.index(['status', 'created_at']);
    });
  },
  down(schema) { schema.drop('orders'); },
});
```

- Files `migrations/*.ts|js` (directly in the folder), run in file-name order. The name without the
  extension (`001_create_orders`) is the key in `script_migrations`.
- `schema.create / table / drop / dropIfExists / rename / hasTable / hasColumn / raw` (raw: separate
  mode only). Columns: `id, string(len=255), text, integer, bigInteger, decimal(p=12, s=2), boolean,
  date, datetime (DATETIME(3)), json, enum(values), uuid (CHAR(36)), foreignId, timestamps()`.
  Modifiers: `nullable, default, unique, index, unsigned, comment, after, change, useCurrent,
  useCurrentOnUpdate, references(table, col='id').onDelete('cascade'|'restrict'|'set null')`.
  Table level: `t.index(cols, name?), t.unique(cols, name?), t.dropColumn, t.renameColumn,
  t.change(column)` (same as `.change()`), `t.dropIndex, t.dropUnique, t.dropForeign(column)`.
  Generated index names: `<cols>_index` / `<cols>_unique`; FKs `fk_<table>_<col>`.
- Names: `^[a-z][a-z0-9_]{0,62}$`, not starting with `core_`, `script_`, `sc_`, `bak_` and not
  `kv_docs`.
- **Publish** runs pending migrations (not `ran` in `script_migrations`) **before** the new code goes
  live, as one batch, as the code user. A failure stops the publish (`400 {error, migration,
  errors:[{file,line,column,message}]}`), is recorded as `failed` with the MySQL error and the
  migration file:line, and the live code keeps running. MySQL DDL is not transactional: a failed
  migration may be partially applied (earlier statements, earlier migrations of the batch stay) —
  fix it with a new migration (or edit the failed one: failed migrations run again on the next publish).
- **Dry run**: `POST /build` returns `migrations.pending[{name, sql[], destructive[]}]`, computed against
  an in-memory snapshot of the database (nothing runs). Builder mistakes (table exists, unknown column,
  missing FK target) are build errors with file:line.
- **Destructive changes** (drop table, drop column, rename table/column, a column change to a smaller
  or different type, `schema.raw` starting with DROP/TRUNCATE/ALTER/RENAME/DELETE/UPDATE/REPLACE): publish
  answers `400 {error, destructive:[{migration, op, table, column}]}` unless the body has
  `"confirmDestructive": true`. Before such a statement runs, the table is copied to
  `bak_<table>_<yyyymmddhhmmss>` (recorded in `script_table_backups`, dropped after 7 days by an hourly
  cleanup). `schema.raw` statements are not backed up.
- **Rollback**: `POST /db/migrations/{name}/down` runs `down()` of a migration of the newest batch only
  (from the live version, else the draft), backs up like a publish, and marks it `rolled_back`
  (pending again).
- Restoring an old version does **not** run `down()`.

### Query API (inside handlers)

```ts
const page = db.from('orders as o')
  .join('customers as c', 'c.id', 'o.customer_id')         // join / leftJoin / rightJoin
  .leftJoin('core_users as u', 'u.id', 'o.user_id')         // core_* views: separate mode
  .select('o.id', 'o.code', 'c.name as customer', db.raw('SUM(o.total) AS total'))
  .where('o.status', 'paid').where('o.total', '>', 100).whereIn('o.id', [1, 2])
  .where((q) => q.whereNull('o.note').orWhere('o.note', 'like', 'A%'))
  .groupBy('o.id').having('total', '>', 1).orderBy('o.created_at', 'desc')
  .paginate(1, 20);                                          // { items, total, page, perPage }

const id = db.table('orders').insert({ code: 'A1', total: 10, placed_at: new Date() });
db.table('orders').where('id', id).update({ total: 12 });    // update/delete need where() or .all()
db.table('products').whereIn('id', ids).decrement('stock', 1);
db.transaction((tx) => { tx.table('a').insert({}); tx.table('b').insert({}); });  // rollback on throw
db.query('SELECT p.title, COUNT(*) n FROM orders o JOIN core_posts p ON p.id = o.post_id GROUP BY p.id', []);
const notes = db.docs('notes'); notes.insert({ title: 'x' }); notes.find({ where: { title: 'x' }, orderBy: '-createdAt' });
```

- Terminal methods: `get, first, pluck, paginate (perPage ≤ 100), count, exists, sum, avg, min, max,
  insert (one or ≤ 1000 rows → first id), update, delete, increment, decrement, toSQL`.
  Where forms: `where(col, v)`, `where(col, op, v)` (`= != <> < <= > >= like 'not like' in 'not in'`),
  `where({col: v})`, `where(q => …)`, `where(db.raw(…))`, `whereIn/NotIn, whereNull/NotNull,
  whereBetween/NotBetween, whereLike` and `or…` variants.
- Every value is a bound parameter. Identifiers must be `col`, `table.col`, `alias.*`, `*`, `col as x`;
  anything else goes through `db.raw(sql, params)` (separate mode only).
- Results: DECIMAL → string, DATETIME/TIMESTAMP → ISO string (`2026-10-24T10:30:00.123Z`), DATE →
  `2026-10-24`, JSON → parsed, BIGINT → number (string beyond 2^53), signed TINYINT → boolean.
  Parameters: `Date` objects and `toISOString()`-shaped strings are sent as DATETIME values
  (MySQL refuses the trailing `Z`); objects/arrays are sent as JSON text.
- Limits: each statement gets min(5 s, the handler's remaining time) (context deadline +
  `MAX_EXECUTION_TIME` hint on SELECTs); at most 10,000 rows / 10 MB per result (use `paginate`).
  `db.transaction` callbacks must be synchronous; nested transactions are refused; use `tx` inside.
- Available in routes, events and `POST /run` (test runs use the **same** database — they write real
  data). Not available at the top level (registration) or inside migrations.
- `GET /types` adds the query/schema types and, generated from the current tables, `interface
  OrdersRow {…}` merged into `TableRows`, so `db.table('orders')` is typed.

### Database screen API (`/api/v1/code/db`)

| Method + path | Cap | Notes |
|---|---|---|
| `GET /status` | view_code_data | `{mode, database, configured, sizeBytes, tables, setupSql?, setupDsn?, error?}` (allowed in demo mode) |
| `GET /tables` | view_code_data | `[{name, rows, sizeBytes, comment, view, system}]` — views `view:true`, kv_docs `system:true`, no backups/bookkeeping |
| `GET /tables/{t}` | view_code_data | `{name, view, system, primaryKey, columns[], indexes[], foreignKeys[], relations[]}` |
| `GET /tables/{t}/rows?page=&perPage=&q=&sort=&dir=&filter[col]=` | view_code_data | `{items, total, primaryKey}`; filters: contains for text, equals otherwise |
| `POST /tables/{t}/rows` · `PUT /tables/{t}/rows/{pk}` · `DELETE /tables/{t}/rows/{pk}` | edit_code_data | row (201/200) / 204; views and kv_docs read-only (400) |
| `POST /tables/{t}/import?dryRun=1` (multipart `file`, CSV, header = columns) | edit_code_data | `{inserted, errors:[{row, message}], dryRun}` — `row` = line number; empty cell = NULL for nullable columns, `\N` = NULL |
| `GET /tables/{t}/export?format=csv\|json&q=` | view_code_data | attachment (CSV writes NULL as `\N`) |
| `GET /dump` | view_code_data | `.sql` of every developer table + kv_docs (structure + data, generated in Go) |
| `POST /sql` `{sql, params, write}` | view_code_data (read) / edit_code_data (write) | `{columns, rows (arrays), affectedRows, insertId, durationMs, truncated}`; separate mode only; read mode accepts SELECT/WITH/SHOW/DESCRIBE/EXPLAIN only and runs in a READ ONLY transaction; 10 s; 1000 rows shown |
| `GET /migrations` | view_code_data | `[{name, batch, ranAt, ranBy, durationMs, status: ran\|failed\|rolled_back\|pending, error, pending, inDraft, inLive}]` |
| `POST /migrations/{name}/down` | edit_code_data + publish_scripts | `{ok:true}`; 409 when not in the newest batch |
| `GET /backups` · `GET /backups/{id}/download` · `DELETE /backups/{id}` | view / view / edit_code_data | `[{id, tableName, backupTable, migration, createdAt, expiresAt, sizeBytes}]` |

All mutations are audited (`db_insert, db_update, db_delete, db_import, db_sql, db_rollback,
db_backup_delete`). Demo mode: every `/db` endpoint except `GET /status` answers 403.

## Connecting code to the designer (phase 3)

### Script API

```ts
site.collection('events', {
  label: { th: 'อีเวนต์', en: 'Events' },
  fields: { title: 'text', date: 'date', image: 'image', body: 'html' },   // 'html' is sanitised
  params: { category: 'text', q: 'text' },          // others are dropped; values arrive typed
  perPage: { default: 9, max: 30 },                 // default 10 / 50, hard cap 100
  cache: 60,                                        // seconds, per (params, page, perPage, viewer role)
  access: ['*'],                                    // default ['administrator']
  load: ({ page, perPage, params, request, user }) =>
    db.table('events').where('category', params.category).paginate(page, perPage),  // { items, total }
});
site.action('register', {
  input: { eventId: 'number', note: 'text?', email: 'email' },   // validated first ('?' = optional)
  access: ['*'],
  run: ({ input, request, response, user }) => {
    response.setCookie('ticket', '42', { maxAge: 3600 });        // → pl_site_ticket (Secure on https)
    return { message: 'ลงทะเบียนแล้ว', refresh: ['events'], update: ['cartBadge'], page: { count: 1 } };
  },
});
site.page('/checkout', { load: ({ request, params, user }) => ({ total: 1234.5 }) }); // {{page.total}}; default access ['*']
site.formatter('baht', (v) => '฿' + Number(v).toFixed(2));                           // {{ item.price | baht }}
```

- Names `^[a-z][a-zA-Z0-9_-]{0,62}$`, unique per kind (publish fails otherwise). `/status` and `/build`
  `registered` list `collections`, `actions`, `pages`, `formatters`.
- Limits: collection / page / action 2 s, formatter 50 ms per call (failure → the raw value + a log line).
  A failing `load` renders the collection empty; a failing `run` answers 500 `{"error":"ทำรายการไม่สำเร็จ"}`.
- `load` may return `{ items, total }` or a plain array (a plain array longer than a page is treated as
  the whole collection and paged by the system).
- `request.cookies` holds only the site's own cookies (`pl_site_*`, prefix removed) — routes too.
  `response.setCookie(name, value, { maxAge, httpOnly = true, sameSite = 'Lax', path = '/' })` works in
  route, page and action handlers.

### Access (layer 2)

Every route (`"GET /hello"`), collection, action, page (`"/checkout"`) and developer table has an
effective access list: the admin override in `script_access`, else the code's `access`, else the default
(`['administrator']`; pages `['*']`; tables `[]` = nobody). `'*'` = anyone including anonymous visitors;
the administrator always passes. Visitors are anonymous unless the request carries an admin bearer token.
`GET /api/v1/code/access` → `[{kind, name, label, codeDefault, override, effective}]`;
`PUT /api/v1/code/access` `{kind, name, roles: [...] | null}` (null removes the override; audited `access`).

### Bindings: expressions, formatters, conditions

`{{ expr }}` works wherever bindings do. Literals, dotted paths (`item.price`, `page.total`, `post.title`,
`site.title`, `query.q`, `$index`, `item.tags.length`), `- !`, `* / % + - < <= > >= == != && || ??`, `?:`,
parentheses and pipes (`| number:2`); no calls, indexing or assignment. Numeric strings (DECIMAL) count as
numbers; `+` concatenates when a side is a non-numeric string; division by zero / maths with null → empty;
`&&`/`||` give booleans; `==` is numeric when one side is a number, otherwise same-type strict. Unknown
roots and `__proto__`/`constructor`/`prototype` are null. Core formatters: `number[:decimals]`,
`date:'D MMM YYYY'` (tokens `D DD M MM MMM MMMM YY YYYY BBBB H HH m mm`, `[literal]`; Thai month names on
Thai sites; site timezone, default Asia/Bangkok; `YYYY-MM-DD` values are never shifted; numbers are Unix
seconds), `upper`, `lower`, `truncate:n`, `default:v`, `json`, plus `site.formatter`s. Values are
HTML-escaped. An expression that does not parse stays as literal text. Shared test vectors:
`backend/internal/design/testdata/expr_vectors.json` (the editor's TypeScript evaluator passes the same).

`props.showWhen` (every layer): falsy (`false, null, '', 0, '0', []`) → the layer and its children are not
sent at all. A condition that does not parse shows the layer (logged). Inside a collection, children whose
condition reads `$empty` render once when there are no items (empty state).

### Designer nodes and public markup

- **Collection** `props.source = 'code:<name>' | 'table:<name>'` (plugin sources unchanged),
  `perPage`, `params: {param: expr}`, `key` (`[a-z0-9]{1,12}`, default: the first 12 letters/digits of
  the node id), `emptyText`, `noReload`, and for tables `table: {columns, where: [{column, op, value}],
  orderBy: [{column, dir}]}` (`op`: `= != < <= > >= like in null notnull`; a where with an empty value is
  skipped; params named like a column filter by equality). Root:
  `<div class="n-<id>" data-rea-collection="<id>" data-rea-key="<key>" [data-rea-noreload]>`. Page from
  `?<key>_page=N` (or `?page=` when it is the only paginated collection); filter values from
  `?<key>_<param>=` (or `?<x>` when the param maps to `query.<x>`).
- **Pagination** `props.collection, style (numbers|prevnext|loadmore), window (2), labels, itemStyle,
  activeStyle, disabledStyle`: `<nav class="n-<id> rea-pagination" data-rea-pagination="<collection>"
  data-rea-style="…">` with `a.rea-pg-item` links (`rel=prev/next`, current page `.rea-pg-active
  [aria-current=page]`, disabled `a.is-disabled[aria-disabled]` without href, gaps `span.rea-pg-gap`;
  load more `a.rea-pg-more[data-rea-more]`), other query parameters kept; `hidden` when everything fits
  on one page. The first paginated collection adds `<link rel="prev|next">` to `<head>`.
- **Input** `props.kind (text|search|number|email|select|checkbox|textarea|hidden), name, placeholder,
  value (expr), options | optionsFrom {collection, value, label}, required, filter {collection, param}`:
  `<div class="n-<id> rea-field"><input class="rea-field-el" …><span class="rea-error"
  data-rea-error-for="<name>"></span></div>` (checkbox: `<label class="… rea-field rea-check">`). A filter
  input carries `data-rea-filter="<collection>" data-rea-reset="<key>_page"`; outside a form it is wrapped
  in a `display:contents` GET form so it works without JavaScript.
- **Form** a frame with `props.form`: `<form data-rea-form>` (GET when it holds filters, otherwise only the
  runtime submits it).
- **Action** `props.action = {type:'code', name, input:{k: expr}, confirm, update:[nodeIds],
  success:{message, redirect}}` on any layer → `data-rea-action="<name>" data-rea-input="<JSON, evaluated on
  the server, escaped>"` (+ `data-rea-confirm`, `data-rea-update`, `data-rea-success`, `data-rea-redirect`).

### Partial re-render, actions and the client runtime

- `GET /_c/{postId}/{nodeId}?<page query>` (header `X-Rea-Path` = page path) →
  `{"nodes":{"<id>":"<outerHTML>"},"head":{}}` — the node plus the pagination bound to it.
- `POST /_a/{name}` `{input, ctx:{postId, path, query}, update:[ids]}` with `X-CSRF-Token` equal to the
  `anasaki_csrf` cookie (set, SameSite=Lax, readable, on pages that include the runtime) and a same-site
  `Origin` (or `Referer`); 30 calls / minute / IP per action (429). Answers
  `{message?, redirect?, errors?, data?, nodes}` — nodes re-rendered for `update` ids, layers named in
  `result.update` and collections in `result.refresh`, with `result.page` merged into `{{page.*}}`.
  Input errors → 400 `{error, errors}`; no access → 401/403.
- The runtime (< 7 KB minified since phase 5, no framework) is added only to pages that use actions,
  filters, forms, load more, no-reload pagination or a Frontend code block with JS
  (`window.__reaP=<postId>`). It exposes `window.anasaki` — see
  [Frontend code block and window.anasaki](#frontend-code-block-and-windowanasaki-phase-5).

### Editor APIs

- `GET /api/v1/code/bindings` (edit_pages; `?draft=1` also needs edit_scripts) →
  `{collections:[{name,label,fields,params,perPage}], actions:[{name,input}], pages:[{path}],
  formatters:[script formatter names], tables:[{name, columns:[{name,type}]}]}`.
- `POST /api/v1/code/preview` (edit_pages; `draft` also needs edit_scripts)
  `{kind:'collection', source, page, perPage, params, table?, viewAs:'anonymous'|'me', draft?}` →
  `{items, total, perPage, page, denied?}` or `{error}`; `{kind:'page', path, viewAs, draft?}` → `{data}`.

## Hooks, visitors, post types, services, jobs and admin pages (phase 4a)

### Filters and custom hooks

```ts
site.filter('post.content', (html, { post }) => html + '<p class="sig">…</p>', { priority: 20 });
site.filter('post.beforeSave', (p, { user, isNew }) => p.title === 'x' ? { error: 'ห้ามใช้ชื่อนี้' } : { ...p, excerpt: p.excerpt || p.title });
site.filter('site.price', (v, { currency }) => v * 1.07);        // your own filter point ('site.*')
const total = site.applyFilter('site.price', 100, { currency: 'THB' });   // inside a handler
site.on('site.orderPaid', (data) => log.info('paid', data.id));
site.emit('site.orderPaid', { id: 7 });                           // inside a handler, delivered asynchronously
```

| filter | value | ctx | where | limit |
|---|---|---|---|---|
| `post.content` | sanitized HTML | `{post}` (`post.meta` included) | postContent node / default post template | 50 ms |
| `post.beforeSave` | `{type,title,slug,status,excerpt,content,meta}` | `{user, isNew}` | admin `POST/PUT /posts` and `db.posts` writes; `{error:'…'}` → 400 with that message | 2 s |
| `page.head` | `{title, description, image, noindex}` (`title` = the full `<title>`) | `{post, path}` | designed pages, after the SEO plugin | 50 ms |
| `collection.items` | items array | `{name, params, page}` (`name` = collection name, `table:<t>` for tables) | after every `code:`/`table:` load (not cached) | 50 ms |
| `action.input` / `action.result` | input / result | `{name, user, visitor}` | around `site.action` | 2 s |
| `mail.beforeSend` | `{to, subject, html, text, replyTo}` | `{source}` (`"route GET /x"`, `"job send"`…) | before `mail.send`; `null` cancels (`{ok:false, cancelled:true}`) | 2 s |

Order: ascending `priority` (default 10), then registration order. A filter that throws or runs out of
time is skipped (the previous value is kept) and logged as a warning with its `file:line`. Returning
`undefined` keeps the (possibly mutated) argument. Every filter ctx also has `user` and a lazy `visitor`.
Filters run in the request's VM (never at registration). Filters run from code (`applyFilter`, `db.posts`,
`mail.send`) count against the handler's own time limit.

### Visitors (`site.visitor`)

Core has no member system — the visitor hook says who a request is:

```ts
site.visitor(({ request }) => {
  const s = request.cookies.sid && db.table('member_sessions').where('id', crypto.hash('sha256', request.cookies.sid)).first();
  return s ? { id: s.member_id, name: s.name, groups: ['member', s.tier] } : null;
});
site.visitorGroups(['member', 'gold']);   // columns on the Access screen
```

- One resolver per site (a second one fails the publish). It runs lazily — the first time something needs
  the visitor — at most once per request, 200 ms; an error, a timeout or a bad result (no `id`) → `null`
  (logged). It sees only `pl_site_*` cookies, never the admin's credentials.
- `ctx.visitor` in route, collection, page, action, filter and admin-page handlers (jobs: `null`).
- Access lists accept `group:<name>` (`access: ['group:member', 'editor']`, also as admin overrides):
  anonymous visitors are matched by group, CMS users by role (the administrator always passes).
- Since 2.0.6 access lists also accept `cap:<capability>` — every role holding it (Users → Roles), e.g.
  `access: ['cap:edit_pages']` or a plugin capability `['cap:analytics.view']`; `site.can(user, cap)` checks one in
  code. Site Code may name any capability; a JS plugin only its own and the core ones
  ([plugin-api.md §2.1](plugin-api.md)).
- Bindings and `showWhen`: `visitor`, `visitor.name`, `visitor.groups`, and `visitor.is.<group>` →
  `true`/`false` (false without a visitor). `is` is reserved: `visitor.is` alone and
  `visitor.is.<g>.<x>` are null. Shared vectors: `backend/internal/design/testdata/expr_vectors_visitor.json`.
- Editor preview: `POST /api/v1/code/preview` accepts `viewAs: {groups: [...]}` (a visitor
  `{id:'preview', name:'Preview', groups}`).

### Post types and fields

```ts
site.postType('event', {
  label: { th: 'อีเวนต์', en: 'Events' }, icon: '📅', slug: 'events',
  fields: { date: { type: 'datetime', required: true, label: { th: 'วันที่', en: 'Date' } }, place: 'text', seats: { type: 'number', min: 0 } },
  access: { edit: ['editor', 'author'] },
});
site.fields('post', { readingTime: 'number' });
```

- Type ids `^[a-z][a-z0-9_]{1,19}$`, not a core type (`post, page, template, revision, …`); slug `a-z 0-9 -`,
  not reserved (same list as routes, incl. blog/category bases) and unique.
- Field types `text, textarea, number, bool, date, datetime (YYYY-MM-DDTHH:mm[:ss], site timezone), select
  (options), url (https://… or /path), image (URL or media id), email, json`; `required`, `min`/`max` (number
  value / text length), `options` (strings or `{value, label}`). Invalid values → 400
  `{error, fields: {<key>: message}}` (Thai). Unknown keys are dropped; `""`/`null` removes a value.
- Since 2.1.0 also: `relation` (post ids; `postType: 'event'` or a list, default `post`), `user` (user
  ids), `term` (term ids; `taxonomy`, default `category`), `file` (a media id), `gallery` (media ids),
  `repeater` (rows of `fields: {name: 'text', price: 'number'}`, no nesting; `max` rows), `richtext`
  (HTML, sanitised like post content) and `color`. `multiple: true` makes relation / user / term / file a
  list (at most 100, duplicates dropped; `max` limits it). The editor shows search pickers, the media
  picker and repeatable rows; the values are ids — read the items with `db.posts.list({ids})`,
  `db.terms.list({ids})`.
- Stored in `post_meta(post_id, meta_key, value JSON)`, deleted with the post, included in backups
  (`posts[].meta` in `data.json`).
- Admin API: the existing `/api/v1/posts` endpoints — `GET /posts?type=event`, `Post.meta`, `POST/PUT /posts`
  with `meta`. Permissions of a custom type: the roles in `access.edit` (default editor + administrator) list,
  edit and publish it; since 2.0.6 `access.edit` accepts `cap:<capability>` entries and
  `caps: { edit, publish }` names the capabilities that edit / publish the type (publishing limited to
  `caps.publish` when set). `GET /api/v1/post-types` (any signed-in user) →
  `{postTypes:[{type,label,singular,icon,slug,public,supports,fields,access,canEdit}], postFields:{post:{…}, page:{…}, <type>:{…}}}`
  where `fields`/`postFields` are objects `{"<key>": {type, label:{th,en}, required?, min?, max?, options?}}`
  in declaration order (`postTypes[].fields` = the type's own fields, `postFields` = `site.fields` additions).
  `GET /api/v1/code/bindings` returns the same `postTypes` / `postFields` (+ `visitorGroups`, `adminPages`);
  users without `edit_pages` get only those keys.
- Public site: `/<slug>/<post-slug>` with the template in option `post_template_<type>` (Settings → Reading;
  saved with `PUT /api/v1/settings`), else the post's own design, else the default post template;
  `/<slug>` lists them (archive template, else a simple list) unless a page has that slug or a `site.route`
  claims the path. `post.meta.<field>` works in bindings.
- `db.posts.list({ type: 'event', where: { 'meta.date': { gte: '2026-01-01' } }, orderBy: '-meta.date' })`:
  `= != < <= > >= in` (also `eq ne lt lte gt gte`) on top-level meta keys (JSON comparison); results include `meta`.
- A type removed from the code: its posts stay in the database, hidden from the admin (the API answers 400
  for that type) until the type is registered again.
- **JS plugins** register post types too (since 2.0.5, scope `post-types`, [plugin-api.md §3.5](plugin-api.md)):
  type ids `<plugin id, - as _>_<name>` (e.g. `docs_doc`), `public: false` = the plugin serves the pages. A
  plugin's type id or slug cannot be used here while that plugin is active (publish is refused), and Site Code may
  read (`db.posts.list / get`) but not write a plugin's types. `GET /api/v1/post-types` marks them with
  `plugin` / `pluginName`.

### Writing content

`db.posts.create(post)`, `db.posts.update(id, patch)`, `db.posts.delete(id)` — checked against the caller's
capabilities like the admin (a visitor or anonymous caller is refused; contributors' posts stay drafts),
run `post.beforeSave`, validate `meta`; the store fires `post.created` / `post.updated` / `post.saved` /
`post.published` / `post.deleted` (actor source `code`). Since 2.1.0 a post may also set `authorId`,
`publishedAt` (a future date schedules it: status `scheduled`), `imageId`, `pinned`, `sortOrder` and `design`,
and `db.posts.list` filters by `status` (`'any'` for every status), `authorId` and `ids`.
`db.asSystem(() => …)` runs the callback with system rights (author = first administrator) and is audited as
`as_system` with the caller's `file:line`.

### Services

- **`mail.send({ to, subject, html?, text?, replyTo? })`** → `{ ok: true }` through the SMTP settings; ≤ 20
  recipients; 100 per hour (setting); logged without the body; throws in demo mode or without SMTP. For
  development set **`SITE_CODE_MAIL_SINK=log`**: messages are written to the Logs screen (`mail (dev sink)`)
  instead of being sent.
- **`http.fetch(url, { method, headers, body, timeout })`** → `{ status, ok, headers, body, url, json(), text() }`
  — synchronous; https only; the host must be on the allowlist (exact or `*.example.com` = any subdomain; IPs
  refused); internal/private addresses refused after DNS resolution; redirects only on the same host (≤ 5);
  5 MB response cap; timeout ≤ 10 s (default 5 s) and never past the handler's limit.
- **Settings and secrets**: `site.settings(key)` → string | null. Stored in `script_settings`; secret values are
  AES-256-GCM encrypted (`enc:v1:…`) with **`SITE_CODE_SECRET_KEY`** (any string) or, when unset, a random key
  created once as `site-code.key` (mode 0600) in `PLUGINS_DIR` (default `./plugins-data`; not under uploads,
  never in backups — keep it with your volume backups: losing it makes stored secrets unreadable).

### Jobs and cron

```ts
site.job('sendNewsletter', { timeout: 300, retries: 3, concurrency: 1 }, ({ data, attempt, progress }) => { progress('50%'); });
jobs.enqueue('sendNewsletter', { subject: 'x' }, { delay: '10m', key: 'oct' });   // → job id
site.cron('0 8 * * *', 'sendNewsletter');                                         // site timezone
site.cron('*/15 * * * *', () => db.table('carts').where('expired', true).delete());
site.cron('30 3 * * *', () => cleanupLinks(), { name: 'linkCleanup' });             // listed as @linkCleanup (2.1.1)
```

- Queue `script_jobs` (CMS database); workers started by the server claim due jobs with
  `SELECT … FOR UPDATE SKIP LOCKED` (several instances share the queue). Handlers run in a fresh VM with
  `user = null`, `visitor = null` (use `db.asSystem` for content writes), `timeout` ≤ 300 s.
- Failures retry after 30 s × 2^(attempt−1) until `retries + 1` attempts, then `failed`. `key` dedupes
  against a *pending* job of the same name. Unknown (unpublished) job names fail with "job not found".
- Cron: 5 fields (`* , - /`, `JAN-DEC`, `SUN-SAT`, `@hourly/@daily/@weekly/@monthly/@yearly`), Vixie
  day-of-month/day-of-week rule, evaluated every minute; `script_cron(name, expr, next_run, last_run,
  last_status)` with a conditional UPDATE → one run per schedule per minute across instances (no catch-up
  after downtime). Schedule names: the job name (`<job>@2` for a second schedule of the same job), `@cron-N`
  for function schedules, or `@<name>` for a function schedule given `{ name }` (2.1.1; names are unique,
  and renaming a schedule starts its history afresh).
- Safe mode, `SITE_CODE_DISABLED` and demo mode pause the workers and cron. The server stops them cleanly on
  SIGINT/SIGTERM (running handlers are interrupted and their jobs go back to the queue).

### Webhooks

`site.route('POST /hooks/pay', { access: ['*'], rawBody: true, maxBody: 65536 }, ({ request }) => …)` →
`request.rawBody` (UTF-8) and `request.rawBodyBase64`. `maxBody` defaults to 1 MB (≤ 5 MB) for every route
(larger → 413). Public routes (effective access includes `'*'`) have a per-IP limit, 120 calls / minute by
default (setting `routePerMinute`; 429).

### Admin pages

```ts
site.adminPage('orders', { label: { th: 'ออเดอร์', en: 'Orders' }, icon: '🧾', access: ['editor'],
  render: ({ user, query }) => ({ title: 'Orders', html: `<h1>${query.tab ?? 'all'}</h1><button onclick="anasaki.call('ship', {id: 1}).then(r => alert(r.message))">Ship</button>` }) });
```

- `GET /api/v1/code/admin-pages` → `[{id, label, icon}]` the caller's role may open (also `bindings.adminPages`).
- `POST /api/v1/code/admin-pages/{id}/ticket {query?: "tab=x" | {tab:"x"}}` → `{url: "/ext-ui/code/<ticket>/<id>?…"}`
  (8 h ticket, bound to the password like plugin admin pages). The page is served there with
  `Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups; frame-ancestors 'self'` and a
  `window.anasaki.call(name, input)` helper that posts to `/ext-ui/code/<ticket>/_a/<name>` (CORS `*`, the
  ticket authenticates instead of CSRF) and runs the `site.action` as the ticket's user (role access applies).
  Relative links (`?tab=x`, `orders?x=1`) stay inside the ticket URL. Render errors → 500 page (details in Logs).

### Admin API (phase 4a)

| Method + path | Cap | |
|---|---|---|
| `GET /code/settings` | manage_scripts | `{values:[{key, secret, set, value? /*non-secret only*/}], fetchHosts:[…], limits:{mailPerHour, routePerMinute}}` |
| `PUT /code/settings` | manage_scripts | `{values:[{key, secret, value}] /*secret '' = unchanged*/, fetchHosts?, limits?:{mailPerHour?, routePerMinute?}, remove?:[keys]}` → as GET; audited `settings` (keys only) |
| `GET /code/jobs?status=&name=&page=&perPage=` | view_scripts | `{items:[{id,name,data,status,attempts,maxAttempts,runAt,startedAt,finishedAt,progress,result,error,createdAt,key}], total, page, perPage}` |
| `POST /code/jobs/{id}/cancel` · `POST /code/jobs/{id}/retry` | publish_scripts | `{ok:true}`; 409 when the status does not allow it (cancel: pending/running, retry: failed/cancelled) |
| `GET /code/cron` | view_scripts | `[{name, expr, job, nextRun, lastRun, lastStatus /*queued|done|failed|retrying*/}]` |
| `POST /code/cron/{name}/run` | publish_scripts | `{jobId}` |
| `GET /code/admin-pages` · `POST /code/admin-pages/{id}/ticket` | signed in | see above |
| `GET /api/v1/post-types` | signed in | see "Post types" |

## JavaScript plugins, units and interop (phase 4b)

A **JS plugin** is Site Code packed as a plugin (`"runtime": "js"`, see `docs/plugins.md` §4). The engine
runs several **units**: `site` (this project) and one unit per installed and **active** JS plugin. Each unit
has its own bundle, registrations, VMs, logs (source prefixed `"<id>: "`, `unit` column), circuit breakers
and last error.

- **Load order**: plugins sorted by `requires` (a plugin after the JS plugins it requires; ties: install
  time, then id), Site Code last. A cycle or a missing / incompatible / inactive dependency keeps those
  plugins off (`activationError`, logged as `load`). Filters run by ascending `priority`, then this order
  (at equal priority Site Code has the last word); events reach every unit's `site.on`.
- **Namespaces of plugin `<id>`**: collections `code:<id>:<name>`, actions `/_a/<id>:<name>`, access rows
  `<id>:<name>` (routes `<id>:GET /x`, pages `<id>:/path`); routes only under the manifest's
  `code.routes` prefixes (a route that conflicts with an earlier unit's is refused and logged — Site Code
  is the later unit); tables `p_<id>_*` (dashes → `_`; fallback `sc_p_<id>_*`) added/stripped by the
  schema and query builders; no unit can name another unit's tables (`p_*` is refused everywhere, also
  for Site Code); migrations are recorded as `<id>/<file>`; `site.settings(k)` reads `<id>.<k>` (Site Code
  cannot read a plugin's keys); cookies `pl_<id>_*` (and `request.cookies` holds only those); own hooks
  `site.applyFilter` / `site.emit` must start with `<id>.` (Site Code: `site.`; undeclared names log a
  warning); anyone may `site.filter` / `site.on` any name. One `site.visitor` across all units (a second
  one fails activation / publish); `visitorGroups` are merged.
- **Scopes** (manifest `code.scopes`, enforced in the Go bindings): `db` (db.table/from/docs/transaction/raw,
  migrations), `content` (db.posts.list/get, `core_*` views), `content-write` (db.posts writes,
  db.asSystem), `mail`, `http` (host within `code.hosts` **and** the admin allowlist), `visitor`,
  `admin-pages`, `jobs` (site.job, site.cron, jobs.enqueue), `routes`, `filters-core` (filters on core
  points). Outside them a call throws `ไม่มีสิทธิ์ใช้ <scope> …` and a warning is logged; at registration
  the activation fails. Plugins never get `db.query` / `schema.raw`, and `db.raw` fragments may not
  contain SELECT/FROM/JOIN/… or SQL comments (`/* */`, `/*! */`, `--`, `#` — a MySQL versioned comment
  could hide a keyword; since 2.0.7) (raw SQL could reach other units' tables). Site Code has every scope.

  Memory: goja (the JavaScript engine) has no per-script memory limit — a handler or plugin can still use a
  lot of memory within its time limit (e.g. `'x'.repeat(n)` on visitor input). Limit the container
  (`mem_limit` / `deploy.resources.limits.memory` in docker-compose) and set `GOMEMLIMIT` a bit below it so
  the Go runtime collects garbage before the kernel kills the process; validate sizes of visitor input in
  your code.
- `site.postType` in a plugin needs the `post-types` scope (since 2.0.5; type ids `<id>_<name>`).
- A plugin's jobs and schedules are `<id>:<job>`; its admin pages `<id>:<page>`.

### Imports, exports and plugins.call

```ts
import { baht } from 'money';          // or 'anasaki:money'. Site Code: any active plugin; a plugin: ids in "requires"
import { cartTotal } from 'shop';
site.export('rate', (from, to) => 1);  // a plugin: only names listed in code.exports
const r = plugins.call('reports', 'monthly', { sales: [100, 250] });   // WASM / service plugin function
```

- A plugin's `code.library` module is **bundled into the importer** (runs with the importer's scopes; it
  may import only its own files; source paths show as `@money/code/lib/x.ts`).
- Names in the target's `code.exports` become proxies that run **in the target unit** (its scopes, tables,
  settings) within the same request session (user, visitor; its cookies go to the caller's response).
  Arguments and results are copied as JSON (functions/`undefined` dropped, `Date` → ISO string); the time
  counts against the caller's limit (a timeout ends the caller as a timeout); errors are rethrown as
  `<id>.<name>: <message> (<id>: code/x.ts:12:5)`. Cross-unit calls nest at most 16 levels.
- Importing a plugin that is not active / not required is a build or activation error naming it.
- `plugins.call(id, fn, input)` POSTs `{input, user, visitor}` (signed v1+v2) to `/functions/<fn>` of an
  active service or WASM plugin declaring `service.functions`; `{"result": x}` → `x` (else the whole JSON);
  a status ≥ 300 throws its `error`. Timeout: the handler's remaining time, at most 10 s. A plugin may
  call only plugins in its `requires`; Site Code any.
- **Event bridge**: service/WASM events (`/api/service/events` → `<plugin>.<event>`) reach `site.on` in all
  units; `site.emit` from any unit is also delivered to the service/WASM plugins subscribed to that name
  (and to package webhooks), like core events.

### Export Site Code as a plugin

`POST /api/v1/code/package?dryRun=1` (publish_scripts) with `{id?, name?, version?, description?, scopes?,
routes?, library?, exports?}` → `{manifest, warnings:[{message, file?, line?}], fileName}`; without
`dryRun` the zip (`<id>-<version>.zip`). The **live** version is restructured: files → `code/…`,
`migrations/…` kept, `manifest.json` pre-filled (scopes inferred from the APIs used, routes from the
registered route prefixes, exports from `site.export`, hooks from `applyFilter`/`emit` names, `requires`
from plugin imports); `site.` hook names in `site.filter/on/applyFilter/emit` are renamed to `<id>.`
(warned). The zip passes the package validator (`id` = `^[a-z0-9][a-z0-9-]{2,39}$`, not `site`).

### Admin API additions (phase 4b)

| Method + path | Cap | |
|---|---|---|
| `GET /code/status?unit=` | view_scripts | `unit`, `activationError?`, `units:[{id, name, version?, plugin, active, loaded, error?}]` (site first) |
| `GET /code/logs?unit=` · `/logs/download?unit=` | view_scripts | items carry `unit` |
| `GET /code/access?unit=` · `PUT /code/access {kind, name, roles, unit?}` | manage_scripts | rows carry `unit`; plugin names `<id>:<name>` |
| `GET /code/settings?unit=` · `PUT /code/settings {…, unit?}` | manage_scripts | values carry `unit`, keys without the `<id>.` prefix; fetch hosts / limits ignored for plugin units |
| `GET /code/jobs?unit=` · `GET /code/cron?unit=` | view_scripts | plugin jobs / schedules are `<id>:<name>`; rows carry `unit` |
| `GET /code/hooks` | view_scripts | `[{name, kind:'filter'\|'event', declaredBy:'<unit>'\|'core', subscribers:[{unit, priority, file, line}]}]` in effective order |
| `POST /code/package?dryRun=1` | publish_scripts | see above |
| `GET /code/bindings` | edit_pages | collections / actions / pages carry `unit`; plugin names `<id>:<name>` |

## Frontend code block and window.anasaki (phase 5)

When a page needs a little browser code next to a design — a chart, a live counter, a widget calling your
routes — use the designer block **Frontend code / โค้ดหน้าเว็บ** (Insert → Code) instead of a raw embed. It
has three fields:

| field | becomes |
|---|---|
| **HTML** | inserted as is inside the block root (raw HTML, not sanitised); `<style>` elements in it are scoped like the CSS field |
| **CSS** | scoped to the block: every selector is prefixed with the block class; `:root` / `:scope` mean the block itself |
| **JS** | an ES module run after the page is parsed, once per rendered copy of the block, with `el` (the block root) and `anasaki` |

Data shape (node type `codeblock`):
`{ "id": "cb1", "type": "codeblock", "props": { "html": "…", "css": "…", "js": "…" } }`. Server render:

```html
<div class="n-cb1 rea-cb-cb1" data-rea-code-block="cb1">HTML</div>
<style>.rea-cb-cb1 p { color: red }</style>
<script type="module">document.querySelectorAll('[data-rea-code-block="cb1"]').forEach(el => (async (el, anasaki) => {
JS
})(el, window.anasaki).catch(e => console.error('code block cb1:', e)));</script>
```

- The id in the class and attribute keeps only `A-Z a-z 0-9 _ -` of the node id; `n-<id>` carries the
  designer's size and layout styles.
- **CSS scoping** (identical in the designer canvas and on the server — shared test vectors
  `codeblock_scope_vectors.json`): comments are removed; each selector of a list becomes
  `.rea-cb-<id> selector`, except that `:root` / `:scope` are replaced by `.rea-cb-<id>` (so `:scope > p`
  styles direct children); `@media`, `@supports`, `@container`, `@layer`, `@scope`, `@document`,
  `@starting-style` are scoped recursively; `@keyframes`, `@font-face`, `@page`, `@property`… are kept as
  written (keyframe names stay global — prefix them yourself); `@import` and `@charset` are dropped and
  saving a changed block whose CSS contains `@import` is refused (400); `</style` is escaped.
- The JS runs inside an async function: `await` works at its top level and `return` ends it early. Errors go
  to the browser console prefixed with the block id. `</script` and `<!--` in the JS are escaped. JS never
  runs in the designer canvas (use *Run preview*) and does not re-run when a collection partial is swapped
  in; a block inside a collection item writes its `<style>` / `<script>` once and the script runs for every
  copy.
- **Permission:** only users with **`edit_scripts`** can add a block or change its HTML/CSS/JS
  (`unfiltered_html` is not enough — editors have it). Others see it locked in the designer. When anyone
  saves a page, template, post or component, the server compares every code block with the stored version:
  unchanged blocks (by any author) are kept byte for byte; an added or changed non-empty block from a user
  without `edit_scripts` → 403 `{error, blocks:[ids]}` and nothing is saved. Restoring a revision follows
  the same rule. Previews by such users run only the stored blocks, and a backup import by them empties the
  code of imported blocks.
- The CMS sets no Content-Security-Policy on public pages, so no nonce is involved. If you add a CSP in a
  reverse proxy, allow inline module scripts.

### window.anasaki

Available on every page that includes the runtime (actions, filters, pagination, or a code block with JS)
and as `anasaki` inside a code block:

```js
const r = await anasaki.call('subscribe', { email });   // site.action → { ok, data, errors, message, redirect, status }
if (!r.ok) el.querySelector('.err').textContent = r.message ?? '';

const stats = await anasaki.get('/api/stats', { days: 7 });   // a site.route (same origin) → parsed JSON
await anasaki.post('/api/like', { id: 5 });                    // JSON body + X-CSRF-Token; rejects on HTTP ≥ 400 (err.status, err.body)

await anasaki.refresh('events');   // re-render the collections with source code:events ('shop:orders', 'table:x' or a node id work too)
const off = anasaki.on('refresh', (detail) => console.log('re-rendered', detail.collection));
anasaki.on('action', ({ name, result }) => {});   // after every action (buttons, forms, anasaki.call)
off();
```

- `call` posts to `/_a/<name>` with the phase-3 CSRF double-submit cookie and same-origin check; nodes the
  action re-renders (`refresh`, `update`) are swapped in; a `redirect` is returned, not followed.
- `get` / `post` accept same-origin paths only (`/…`); the answer is parsed as JSON when possible, else
  returned as text.
- `on('refresh' | 'action' | 'anasaki:refresh' | 'anasaki:action', fn)` listens to the DOM events
  `anasaki:refresh` (detail `{collection}`) and `anasaki:action` (detail `{name, result}`, bubbling from the
  trigger element) and returns an unsubscribe function.
- Collections with a `code:` / `table:` source carry `data-rea-source="<source>"` for `refresh`.
- Admin pages (`site.adminPage`, sandboxed iframe) keep only `anasaki.call(action, input)` (phase 4a).
- Types: `AnasakiClient` / `AnasakiCallResult` at the end of `anasaki.d.ts`.

## Personal access tokens and the sync API (phase 5)

Tokens let tools (the CLI, CI, editor plugins) use the Site Code API without a browser session.

- Format `ast_` + 40 base62 characters, shown **once**. The database keeps only its SHA-256 and the first 8
  characters (`prefix`, shown in the admin).
- A token authenticates as its user: the user must still exist and be active, and the role's capabilities
  apply as for a session. Scope `code` (the only scope) limits it to `/api/v1/code/**` and
  `GET /api/v1/auth/me`; anything else → 403 `{"error":"token scope"}`. Unknown, revoked or expired → 401.
  Failed token checks are rate limited per client address (50 per 15 minutes, then 429).
- **A password change revokes all the user's tokens** (as it ends sessions); deleting the user deletes them.
- Send it as `Authorization: Bearer ast_…`.

| Method + path | Who | Request → response |
|---|---|---|
| `GET /api/v1/auth/tokens[?user=<id>]` | any signed-in user (own tokens); `?user=` needs `manage_users` and a role you may manage | `{items:[token]}` newest first |
| `POST /api/v1/auth/tokens` | any signed-in user (session only; not the demo visitor) | `{name, scopes:["code"], expiresInDays?: 0\|30\|90\|365, currentPassword}` (default 90; 0 = never) → 201 `{token:"ast_…", item}`; the account password is required (400 `{error, currentPassword:true}` otherwise, since 2.0.7); at most 50 per user |
| `DELETE /api/v1/auth/tokens/{id}` | the owner, or `manage_users` | 204; someone else's token without `manage_users` → 404 |

Token item: `{id, userId, userName, name, prefix, scopes, lastUsedAt, expiresAt, createdAt, expired}` (RFC 3339
or null; `lastUsedAt` is updated at most once a minute).

### Tree and sync

| Method + path | Cap | Request → response |
|---|---|---|
| `GET /api/v1/code/tree` | view_scripts | `{files:[{path, sha256, size, updatedAt}], liveVersion, draftChanged}` — `sha256` = hex SHA-256 of the stored content (UTF-8) |
| `POST /api/v1/code/sync` | edit_scripts | `{put:[{path, content, baseSha256?}], delete:[{path, baseSha256?}], dryRun?}` → `{files, liveVersion, draftChanged, ok, problems:[{file,line,column,message}], warnings, written, deleted, skipped, dryRun}` |

- One transaction (draft rows locked): everything is applied or nothing. Same rules as `PUT /files`
  (extensions, path rules, 200 files, 512 KB per file, 2 MB total, case-insensitive duplicates, each path
  once per request) → 400 `{error}`.
- `baseSha256` omitted / null = no check (overwrite); a hash = the server file must still have it; `""` =
  the file must not exist on the server. Any mismatch → 409 `{error, conflicts:[{path, serverSha256}]}`
  (`serverSha256: ""` = not on the server) and nothing is written.
- Identical content is not rewritten; deleting a missing file is a no-op. `anasaki.d.ts` at the root is
  generated by the server and never stored (reported in `skipped`).
- `problems` / `warnings` are the build check of the resulting draft (same shape as `POST /build`), so a
  watcher can print them; `dryRun` validates and builds without writing.

### Plugin modules in anasaki.d.ts

`GET /api/v1/code/types` ends with one block per **active** JS plugin:

```ts
declare module 'thai' {
  /** Library (bundled into your code). */
  export const bahtText: any;
  /** Runs in plugin thai (JSON arguments and result). */
  export function lookup(...args: any[]): any;
}
declare module 'anasaki:thai' {
  export * from 'thai';
}
```

A plugin that ships **`code/index.d.ts`** gets that file instead (its `declare` modifiers are removed so it
fits in the module block) — plugin authors: ship one for precise types.

## Plugin joints and page hooks (phase 6)

Generic joints added after building the sample plugins (`examples/js-plugins/CORE_GAPS.md`) and for moving
the built-in Go plugins out of core. Everything here works the same in Site Code and in JS plugins (scoped).

### Formatters from plugins

`site.formatter(name, fn)` works in JS plugins; expressions use it as **`<id>:<name>`**:
`{{ price | thai:baht }}`, with arguments after it: `{{ n | thai:baht:2 }}`. The evaluator (Go and the
editor's TypeScript twin, shared vectors in `testdata/expr_vectors.json`) reads `| a:b` as one name only
when written **without spaces**, `a` is not a core formatter (`number date upper lower truncate default
json`), `b` is not a context root (`item page post site query visitor $index $empty`) and `b` is not
followed by `.`; otherwise it is formatter `a` with argument `b` as before.

**Aliases.** The administrator may enable an unprefixed alias per plugin formatter (`baht` →
`thai:baht`), stored in option `site_code_formatter_aliases` as `{"baht": "thai:baht"}`. An unprefixed
name resolves: core formatter → Site Code's own formatter → enabled alias. One alias names one formatter,
so the plugin whose alias was enabled first keeps the name; the others are listed with a conflict.

| call | cap | |
|---|---|---|
| `GET /api/v1/code/formatters` | `view_scripts` | `[{name, unit, alias?, conflict?}]` — `name` is `baht` (Site Code) or `thai:baht`; `conflict` is `core`, `site` or the formatter holding the alias |
| `PUT /api/v1/code/formatters/aliases` | `manage_scripts` | `{aliases: {"baht": "thai:baht"}}` — the whole map (missing = off); the key must equal the local name and not be a core formatter; answers like GET |

`GET /code/bindings` lists plugin formatters (`thai:baht`) and the active aliases for autocomplete.

### Binary bodies

A route (webhook replies too) and an admin page download may answer with bytes:
`body: Uint8Array | ArrayBuffer` (written verbatim) or `{ status, headers?, bodyBase64 }`; returning a
`Uint8Array` / `ArrayBuffer` directly is a 200. Content type defaults to `application/octet-stream`. At
most **20 MB** (`MaxBinaryBody`). Other typed arrays are serialised as JSON. The test-run API
(`POST /code/run`) returns a binary body as `bodyBase64`.

### Admin page downloads

```ts
site.adminPage('orders', {
  label: { th: 'คำสั่งซื้อ', en: 'Orders' }, access: ['editor'],
  render: (ctx) => `<button onclick="anasaki.download({ status: 'paid' })">${ctx.locale === 'en' ? 'Export CSV' : 'ส่งออก CSV'}</button>`,
  download: (ctx) => ({ status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8',
    'Content-Disposition': 'attachment; filename="orders.csv"' }, body: toCSV(rows(ctx.query.status)) }),
})
```

`download(ctx)` is served at `GET /ext-ui/code/<ticket>/<page>/download?…` with the same ticket and access
as the page (the ticket's user is re-checked); `Content-Disposition: attachment` is enforced (added, or
`inline` replaced), `Cache-Control: no-store`, a `sandbox` CSP; no cookies are set. Inside the page,
`anasaki.download(query)` starts the download (the iframe sandbox includes `allow-downloads`) and
`anasaki.downloadUrl(query)` returns the URL for a plain `<a href>`.

`ctx.locale` (`'th' | 'en'`) is the signed-in user's admin language, in `render`, `download` and the
actions an admin page calls. The page title uses the label in that language.

**Access overrides** for admin pages: the Access screen has rows of kind `adminPage` (`<plugin>:<id>` for
plugins): `PUT /code/access {kind: "adminPage", name, roles, unit?}`. Admin roles only (`*` and `group:*`
are refused); without an override the code's `access` applies (default administrator only).

### Package assets (`site.asset`)

```ts
const font = site.asset('fonts/Sarabun-Regular.ttf')   // { path, size, type, url, bytes(), text() }
const pdf = toPDF(doc, { font: font.bytes() })
```

A JS plugin reads files of its **own package**: any file except `code/`, `migrations/` and `manifest.json`;
the path must be clean and relative (no `..`, `\`, `:`, dot-files). At most **5 MB** per file, cached per
plugin version. `bytes()` returns a new `Uint8Array`, `text()` UTF-8; `url` is the public
`/plugin-assets/<id>/<path>?v=<version>` when that file type is served (css, js, txt, images, fonts), else
`''`. Packages may now contain `.ttf` / `.otf` fonts. Site Code has no package: `site.asset` throws (use
the media library).

### plugins.call to JS exports, plugins.has, plugins.version

`plugins.call('<jsPluginId>', '<export>', ...args)` calls a name in the target's `code.exports` with the
same semantics as an import proxy (JSON copies, runs in the target unit, time counted against the caller,
errors with the target's file:line). No `requires` entry is needed — exports are the plugin's public API.
A target that is not installed or not active throws **`PluginUnavailable`** (`e.name ===
'PluginUnavailable'`, `e.code === 'unavailable'`, `e.plugin`). Service / WASM functions keep their rule
(a plugin lists the id in `requires` or `optionalRequires`). `plugins.has(id)` (installed, active and
running) and `plugins.version(id)` (`string | null`) work anywhere, also at the top level:

```ts
if (plugins.has('thai')) site.formatter('price', (v) => plugins.call('thai', 'bahtText', v))
```

Manifest **`optionalRequires`** (`{"thai": "^1.0.0"}`) documents plugins it works with ("works with" on
the cards): never blocks an install; an installed but incompatible version only logs a warning.

### Middleware and afterResponse (scope `middleware`)

```ts
site.middleware((ctx) => {
  if (maintenanceOn() && !ctx.isAdminUser && !ctx.request.cookies.bypass)
    return { status: 503, headers: { 'Retry-After': '3600' }, body: page() }
}, { priority: 5 })

site.afterResponse((info) => {  // {method, path, query, status, contentType, referrer, userAgent, ip, durationMs, title, isAdminUser, at}
  if (info.status === 404) db.table('notfound').insert({ path: info.path })
})
```

- `site.middleware(fn, {priority})` runs **before routing** for every request the public site handles
  (not `/api`, the admin, `/ext-ui`, `/uploads`, `/plugin-assets`, favicons): before plugin public routes,
  Site Code routes, `/_a`, `/_c` and pages. Return nothing to continue, or a `RouteResult` to answer now
  (default `Cache-Control: no-store`; `ctx.response.setCookie` works). Order: ascending priority (default
  10), then unit load order (plugins, Site Code last), then registration order. `ctx.request` has no
  body (it stays for the route) but has `ip`, `userAgent`, `referrer`, `url`; `ctx.visitor` (lazy),
  `ctx.user` / `ctx.isAdminUser` (an admin bearer token, e.g. editor previews — a browser visit of a
  signed-in admin carries none, so use your own bypass cookie). Each middleware has **20 ms**
  (`Engine.MiddlewareTimeout`); a failure or timeout skips it (logged) and counts against the circuit
  breaker (20 in a minute → skipped for 5 minutes).
- `site.afterResponse(fn)` runs after the response was written, in a queue (1000 items, one worker,
  **500 ms** each, a full queue drops items with a warning). It never delays a response.

### Head and body HTML, page.meta, sitemap and robots

```ts
site.head((page) => `<meta property="og:type" content="${page.type === 'post' ? 'article' : 'website'}">`)
site.bodyEnd((page) => page.isHome ? '<div class="promo">…</div>' : '')
site.filter('page.meta', (m, { page }) => ({ ...m, canonical: page.path, og: { ...m.og, 'og:title': m.title } }))
site.filter('sitemap.urls', (urls) => [...urls, { loc: '/events', changefreq: 'daily' }])
site.filter('robots.txt', (txt) => txt + 'Disallow: /cart\n')
```

- `site.head(fn(page))` / `site.bodyEnd(fn(page))` add HTML to every public page (designed pages and
  fallbacks). `page` = `{type, id, slug, title, description, url, path, image, lang, isHome, archiveType,
  status, siteTitle}`. 50 ms each. Output of a plugin **without scope `head-scripts`** is sanitised:
  head keeps `<meta name|property|itemprop|content>`, `<link>` (not stylesheet / preload / import, safe
  `href`) and `<script type="application/ld+json">`; bodyEnd is sanitised like post content (no
  scripts). Site Code and plugins with `head-scripts` are inserted as is.
- Core filter **`page.meta`** (scope `filters-core`, 50 ms): value `{title, fullTitle, description, image,
  canonical, robots, noindex, og}`, ctx `{page, path}`. `fullTitle` is the `<title>`, `canonical` (site
  path or absolute URL) becomes `<link rel="canonical">`, `robots` a robots meta (noindex wins), `og`
  keys `og:* twitter:* article:* profile:* book:* fb:* music:* video:*` become meta tags (`twitter:` with
  `name=`). It runs after `page.head`.
- Core filters **`sitemap.urls`** and **`robots.txt`**: once a unit registers one of them, core serves
  `/sitemap.xml` (value = core's list: home, published pages and posts with `lastmod`, categories with
  posts; entries are paths, absolute URLs of this site or `{loc, lastmod?, changefreq?, priority?}`; other
  hosts are dropped; ≤ 50,000) and `/robots.txt` (default `Disallow: /api/` + `Sitemap:`; `Disallow: /`
  when search engines are discouraged). The official **seo** plugin uses exactly these filters.

### Admin UI kit

Every code admin page links **`/ext-ui/kit.css`** and **`/ext-ui/kit.js`** (served by core, cached with a
version query + ETag, dependency-free) and has `<body class="ak">` and `<html lang="th|en">`. The kit
gives `ak-*` classes (cards, buttons, badges, tables, forms, tabs, toasts, dialogs, stat tiles, charts)
that follow light/dark (`prefers-color-scheme`, or `<html data-theme="dark|light">`) and `window.AK`:
`AK.toast(msg, kind)`, `AK.confirm(text, {ok, cancel, danger}) → Promise<boolean>`, `AK.tabs(el)`,
`AK.table(...)`, `AK.sortable(table)`, `AK.pager(...)`, `AK.validate(form)`, `AK.bar(el, data)`,
`AK.sparkline(el, values)`, `AK.line(el, {labels, series: [{label, color, values}], height, locale, tableLabel,
chartLabel, dateLabel})` (line chart: nice y ticks, area under the first series, hover tooltip, legend, chart ⇄ table),
`AK.open(path)` (opens an admin path such as `/admin/x/code/<plugin>:<page>` in the admin — the sandboxed frame never
navigates the top window; the admin accepts admin paths only), `AK.pickMedia({accept: 'image' | 'file', timeout})
→ Promise<{url, id, alt, width, height} | null>` (opens the admin's own media library dialog — the one the post editor
and the designer use, with upload — and resolves with the chosen item's public fields; `null` when the user closes it,
may not use the media library (`upload_files`), after `timeout` ms (default 30 min) or outside the admin frame. Under
the hood the frame posts `{type: 'ak:pick-media', reqId, accept}` to the admin, which only accepts it from its own
iframe's window and answers that window with `{type: 'ak:pick-media:result', reqId, media}`; works in admin pages and
dashboard widgets. Store `media.url`, e.g. in a hidden input), `AK.t(key)`; declarative `data-ak-tabs`, `data-ak-sortable`,
`data-ak-validate`, `data-ak-bar`, `data-ak-spark`, `data-ak-line` (JSON options) and `<a href="/admin/…" data-ak-open>`
attributes are initialised on load. The kit also reports the document height to the admin (`{type: 'ak:height'}`), which
sizes dashboard widgets (80–2000 px; the registered `height` is the initial one) and lets admin pages grow to their
content instead of scrolling inside the frame — see `backend/internal/sitecode/kit/kit-demo.html` for every component and its exact options.
A plugin's admin page (`/admin/x/code/<plugin>:<page>`) is shown without the admin's own page header (the site code's
pages keep it), so it should draw its own `<header class="ak-page-head">`.

### For the built-in plugins (phase 7)

| official plugin | joints |
|---|---|
| redirects | `site.middleware` (redirect before routing), `site.afterResponse` (404 log), admin page + kit, db |
| maintenance | `site.middleware` (503 page, bypass cookie; `{headers}` to continue with `Cache-Control: no-store`), `site.info` (site name, language), `site.adminNotice` (admin banner), `site.saveSettings` (settings form) |
| analytics | `site.afterResponse` (`host`, `headers` for prefetch / internal referrers), aggregate selects, `.binary()` paths, `site.dashboardWidget`, admin page + kit charts |
| seo | `page.meta` (`page.siteTagline`), `sitemap.urls` (cached by core), plugin `site.fields` (editor panel) + `site.meta` / `site.setMeta`, `site.saveSettings` (defaults form) |
| cookie-consent | package `bodyEnd` template with the manifest settings |
| custom-code | `site.head` / `site.bodyEnd` with `head-scripts`, manifest settings (100,000 characters) |
| forms | `site.block` (server-rendered, form select), `request.ip` (rate limit), `code.eventAliases` (`form.submitted`), `http.webhook` (per-form), `{page}` results, `site.info().adminEmail`, routes, actions, mail, admin page + CSV |

## Joints for the official plugins (phase 7)

Generic APIs added while porting the built-in plugins to JS (any JS plugin, most of them Site Code too).
Types: `anasaki.d.ts` (section "phase 7").

### Settings: `site.settings`, `site.saveSettings` (B9 included)

In a plugin `site.settings(key)` resolves, in this order:

1. the value saved on **Plugins → plugin → Settings** (the manifest `settings`, stored in
   `plugin_packages.settings`; booleans and numbers come back typed, an **empty text counts as unset**),
2. **Site code → Settings** with `unit: "<id>"` (key `<id>.<key>`, secrets decrypted) — also the only source for
   keys that are not in the manifest,
3. the manifest `default` (typed), else `null`.

`site.settings()` (no key) returns every resolved value as an object. Manifest values are limited by the field's
`maxLength` (textarea default 20,000, up to 100,000 like the built-in custom-code fields); Site code → Settings
values stay limited to 20,000 characters. In Site Code nothing changes (Site code → Settings only).

`site.saveSettings(values)` updates the calling plugin's manifest settings: partial (other keys keep their values),
validated like the Settings form (unknown keys or bad values throw, nothing is saved), audited
(`plugin_settings`), returns every value. Allowed only in admin page code — `render`, an action the page calls with
`anasaki.call`, `download` — and only for a user with `manage_options`.

### `site.info()`

`{ title, tagline, url, language, timezone, adminEmail? }` from Settings → General (`url`: `SITE_URL` / the
`site_url` option, else the request's scheme and host, `''` in jobs). `adminEmail` is included for Site Code and
for plugins with the `mail` scope only. The page object of `site.head` / `site.bodyEnd` / `page.meta` also has
`siteTagline`.

### Plugin custom fields (scope `content-fields`)

`site.fields(type, fields)` works in a plugin with the `content-fields` scope: the keys are stored as
`<id>.<key>` in `post_meta` (at most 64 characters), validated like Site Code fields, and the post editor shows
them in one panel per plugin (`section.plugin-fields[data-plugin=<id>]`, heading = plugin name) for everyone who
may edit the post. `type` is `post`, `page` or a Site Code post type (ignored while that type does not exist), or —
since 2.0.8 — `'*'`: every type, the post types of Site Code and of other plugins included, also ones added later
(feature-detect: `try { site.fields('*', F) } catch { site.fields('post', F); site.fields('page', F) }`).
`GET /api/v1/post-types` and `/code/bindings` list them in `postFields` with `plugin` / `pluginName`.
`db.posts` returns them in `post.meta['<id>.<key>']`; `db.posts.list({ where: { 'meta.seo.noindex': true } })`
filters on them. `site.meta(postId)` returns the plugin's own fields without the prefix (null for a missing post);
in `page.meta` / `site.head` / `site.bodyEnd`, `site.meta(page.id)` is answered from the page's post that core read
before the hook started, so the database read does not count against the hook's 50 ms;
`site.setMeta(postId, values)` writes them (validated; `''`/`null` removes) when the caller may edit the post, inside
`db.asSystem`, or in a system call (the legacy migration's `legacyImport`).

### Admin notices and dashboard widgets (scope `admin-pages`)

```ts
site.adminNotice((ctx) => on ? { level: 'warning', text: { th: '…', en: '…' },
  link: { label: { th: 'ดู', en: 'View' }, href: '/?bypass=KEY' } } : null, { access: ['administrator'] })
site.dashboardWidget('visitors', { label: { th: '…', en: '…' }, icon: '📈', access: ['editor'], height: 220,
  render: (ctx) => '<div data-ak-bar="…"></div>' })
```

Notices (≤ 5 per unit, 300 ms each, `level` `info|warning|danger|success`, text ≤ 500 characters, `href` a site
path or an http(s) URL) are served by `GET /api/v1/code/admin-notices` (text and label in the user's admin
language) and shown in the admin shell (`#admin-notices .admin-notice[data-unit][data-level]`). Widgets (≤ 10 per
unit) are listed by `GET /api/v1/code/dashboard-widgets` (`[{id, label, icon, url, height}]`) and rendered like admin
pages — same ticket, sandbox, kit and `anasaki.call` — at `/ext-ui/code/<ticket>/widget/<id>`; the dashboard shows
each in `section.dash-widget[data-id] iframe.dash-widget-frame`.

### Server-rendered blocks (`site.block`, JS plugins)

```ts
site.block('form', {
  label: { th: 'ฟอร์ม', en: 'Form' }, icon: '📝',
  props: [{ key: 'formId', type: 'select', label: 'Form' }],          // manifest settings field format
  options: () => ({ formId: listForms().map((f) => ({ value: String(f.id), label: f.name })) }),
  render: (ctx, props) => ({ html: renderForm(props.formId), css: '.rea-form{…}' }),
  script: 'assets/form.js',                                           // optional package asset
  cache: 0,                                                           // optional seconds
})
```

The designer lists the block with the package blocks (`GET /api/v1/blocks`, dynamic select options resolved when
the list loads) and previews it (`POST /api/v1/blocks/render`); public pages render it on the server inside
`div.pb-<plugin>-<block>` in the page's code session (200 ms). The node is the usual
`{type: "pluginBlock", props: {plugin, block, values}}`, so a code block can take over a manifest block of the same
type. The HTML is sanitised — forms (relative `action`), inputs, labels, buttons and `data-*` are kept, scripts,
styles and `on*` attributes are removed — unless the plugin has `head-scripts`; `css` is scoped under the wrapper;
`script` adds `<script defer src="/plugin-assets/<id>/<path>?v=<version>">`. A failing render leaves the block
empty (logged). No scope is needed.

### Requests: `request.ip`, `request.host`, ResponseInfo

Every request ctx (`site.route`, actions, pages, middleware) has `request.ip` (same trusted-proxy rules as the
rate limits) and `request.host`. `site.afterResponse` info adds `host` and `headers` (lower-case `purpose`,
`sec-purpose`, `x-moz`, `x-purpose`, `accept-language`, `dnt`, `sec-fetch-dest`, `sec-fetch-mode`).

### Middleware headers on a request that continues

A middleware may return `{ headers: {...} }` (no other key): the request continues and these headers are added to
its response. Allowed: `Cache-Control`, `Vary` (appended), `X-Robots-Tag`, `Content-Language`, `Link`,
`Referrer-Policy`, `X-Frame-Options`, `X-Content-Type-Options`, `Permissions-Policy`; others are ignored. The
page handler may still set its own `Cache-Control`.

### Pages in the site's layout

A route (or middleware) may return `{ page: { title, html }, status?, headers? }` — or
`return response.page({ title, html }, status)` — to show the HTML between the site's header and footer
(the same wrapper as service plugins' `X-Anasaki-Layout: page`); without a renderer it is a plain document.
Since 2.0.8 `page.postId` names the published post the page shows (e.g. an article a plugin serves at its own URL):
the page hooks then see that post (`ctx.page.id`, `type`, `slug`, excerpt as the description, featured image), so
`page.meta` filters such as the SEO plugin apply its per-post values. A draft or a missing id is ignored.

### Event aliases (`code.eventAliases`)

```jsonc
"code": { "hooks": { "events": ["forms.submitted"] }, "eventAliases": { "forms.submitted": "form.submitted" } }
```

`site.emit('forms.submitted', data)` is then also delivered as `form.submitted` (to `site.on` in every unit, to
subscribed service plugins and package webhooks). The source must be one of the plugin's own events; the alias may
not start with the plugin's own id, a CMS namespace (`post. user. media. comment. site. plugin. page. mail. action.
collection. sitemap. robots.`) or — at run time — another running plugin's id (≤ 10 aliases).

### `http.webhook(url, event, data, { secret? })` (scope `webhooks`)

Queues a POST of `data` as JSON to any public http(s) URL (internal addresses refused, 5 s, no redirects),
`X-Anasaki-Event: <event>`, and with a secret `X-Anasaki-Signature: sha256=<hex HMAC-SHA256(secret, body)>`. At most
10 per handler run; failures are logged in the unit's log. Not available on the demo site.

### Database: aggregate selects, case-sensitive columns

`selectCount(alias?, column?)`, `selectCountDistinct(column, alias?)`, `selectSum(column, alias?)`, `selectAvg`,
`selectMin`, `selectMax` work in both database modes (no `db.raw`); `SUM`/`AVG` come back as numbers:
`db.table('daily').select('path').selectSum('views', 'v').groupBy('path').orderBy('v', 'desc').limit(10).get()`.
In migrations `t.string('path', 191).binary()` (= `.collation('utf8mb4_bin')`; also `utf8mb4_unicode_ci`,
`utf8mb4_general_ci`) makes a string / text / enum / uuid column case-sensitive; `t.change(...)` converts an
existing one.

### Sitemap cache

Core keeps the rendered `/sitemap.xml` (5 minutes) while nothing it depends on changes: published posts and their
`updated_at`, category links, the `home_page_id` / bases / visibility options, and for every unit that filters
`sitemap.urls` its version, manifest settings and the data it wrote (any `db` write of that unit).

## Limits at a glance

| what | limit |
|---|---|
| project files | 200 files, 512 KB each, 2 MB in total; `.ts .js .mjs .json .md` |
| handlers | routes / collections / actions / pages 2 s; events 10 s; formatters and render-path filters 50 ms; visitor 200 ms |
| VM pool | 8 concurrent; 2 s queue wait, then 503 |
| request body | 1 MB (routes with `maxBody` up to 5 MB) |
| queries | 10,000 rows / 10 MB per result, 5 s per statement; `paginate` perPage ≤ 100 |
| cache | 10 MB per site |
| mail | 20 recipients per call, 100 per hour (setting) |
| http.fetch | https + allowlist, ≤ 10 s, 5 MB |
| jobs | timeout ≤ 300 s, retries ≤ 10 |
| rate limits | actions 30 / minute / IP per action; public routes 120 / minute / IP (setting) |
| circuit breaker | 20 failures in 1 minute → the handler is off for 5 minutes |
| retention | logs 7 days / 10,000 rows; newest 100 versions |
| tokens | 50 per user; failed checks 50 per 15 minutes per address |
| client runtime | < 7 KB minified |
| binary bodies | 20 MB (routes, webhook replies, admin page downloads) |
| site.asset | 5 MB per file (cached per plugin version) |
| site.middleware / afterResponse | 20 ms / 500 ms each; afterResponse queue 1,000 |
| site.head / bodyEnd / page.meta | 50 ms each |

## Capabilities

| capability | allows |
|---|---|
| `view_scripts` | files, downloads, versions, logs, status, types |
| `edit_scripts` | save/delete/move/upload/sync files, build, test-run routes of the draft; add or change **Frontend code** blocks in designs |
| `publish_scripts` | publish, restore, safe mode — **administrator-equivalent** (the code runs as the site) |
| `manage_scripts` | clear logs, access overrides, settings / secrets, fetch allowlist and limits |
| `view_code_data` | Database screen: tables, rows, export, dump, read-only SQL console, migrations, backups |
| `edit_code_data` | change rows, import CSV, SQL console writes, delete backups; with `publish_scripts`: roll back migrations |

Only the administrator has them by default; grant them per role in **Users → Roles**.

## REST API (`/api/v1/code`, bearer auth)

| Method + path | Cap |
|---|---|
| `GET /status` | view_scripts |
| `GET /files` · `GET /files/{path}` | view_scripts |
| `PUT /files/{path}` `{content, baseUpdatedAt}` · `DELETE /files/{path}` (`/files/{dir}/?recursive=1`) · `POST /files/move` `{from, to}` | edit_scripts |
| `POST /upload?dryRun=1&mode=merge\|replace&dir=` (multipart `file`) | edit_scripts |
| `GET /download?path=` | view_scripts |
| `POST /build` · `POST /run` `{method, path, query, headers, body}` | edit_scripts |
| `POST /publish` `{message, baseVersion, confirmDestructive}` | publish_scripts |
| `GET /versions` · `GET /versions/{id}` · `GET /versions/{id}/files/{path}` · `GET /versions/{id}/download` | view_scripts |
| `POST /versions/{id}/restore` `{alsoDraft}` · `POST /safe-mode` `{enabled}` | publish_scripts |
| `GET /logs?level=&source=&q=&page=&perPage=` · `GET /logs/download?level=&from=&to=` | view_scripts |
| `DELETE /logs` | manage_scripts |
| `GET /types` (`anasaki.d.ts` + generated row types + active plugin modules) | view_scripts |
| `GET /tree` | view_scripts |
| `POST /sync` `{put, delete, dryRun}` | edit_scripts |
| `/db/...` (the site's own database, table above) | view_code_data / edit_code_data |
| `GET /access` · `PUT /access` `{kind, name, roles}` (kind `route collection action page table adminPage`) | manage_scripts |
| `GET /formatters` | view_scripts |
| `PUT /formatters/aliases` `{aliases}` | manage_scripts |
| `GET /bindings` · `POST /preview` | edit_pages (draft: + edit_scripts); bindings without edit_pages: post types, groups, admin pages only |
| `/settings`, `/jobs`, `/cron`, `/admin-pages` (phase 4a, table above) | see above |

Timestamps are RFC 3339; draft file `updatedAt` has millisecond precision — send it back unchanged as
`baseUpdatedAt`. Every endpoint accepts a session token or a personal access token with scope `code`
(`Bearer ast_…`).

## Troubleshooting

| symptom | cause / fix |
|---|---|
| Publish refused with problems | the draft does not build or registration failed — fix the listed `file:line`; the live version keeps running |
| A route answers 404 | not published yet (the draft only runs in *Test route*), a reserved path, safe mode, or another unit claimed it (see Logs) |
| A route answers 503 | the VM pool was busy for 2 s (slow handlers), or the circuit breaker switched the handler off (Status → lastError) |
| "not available during registration" | `db`, `cache`, `crypto`, `mail`, `http`, `jobs` work only inside handlers, not at the top level |
| `db.query` / SQL console disabled | fallback mode: `CODE_DB_DSN` is not set or unreachable (the Database screen shows the setup SQL) |
| Publish refused: destructive migration | confirm "I understand data may be lost" (`confirmDestructive`); a `bak_*` copy is made first |
| `import … from 'x'` fails | only `./` paths and active JS plugins can be imported (a plugin: only ids in its `requires`) |
| CLI: 401 | token revoked or expired, or the password changed — create a new token and `anasaki-code login` |
| CLI: 403 `token scope` | the endpoint is outside `/api/v1/code/**`; another 403 message = your role lacks the capability |
| CLI: exit 2 on push | a file changed on the server since your last pull — `anasaki-code pull`, merge, push ([docs/cli.md](cli.md#conflicts)) |
| Saving a page: 403 about Frontend code | you lack `edit_scripts` and added or changed a code block — undo that change or ask an administrator |
| Code block CSS has no effect | selectors are scoped inside the block: use `:scope` for the block itself; `html` / `body` selectors cannot match |
| Code block JS did not run | check the browser console (errors start with `code block <id>:`); JS never runs in the designer canvas |
| `window.anasaki` is undefined | the page has no runtime: it is added only with actions, filters, pagination or a code block with JS |
