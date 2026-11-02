// Anasaki Studio — Site Code type definitions (phases 1-7).
// Site Code — type definition ของโค้ดฝั่ง server ของเว็บ (ช่วงที่ 1-5)
//
// Put this file next to main.ts so VS Code completes the globals below.
// วางไฟล์นี้ไว้ข้าง main.ts เพื่อให้ VS Code เติมโค้ดได้
//
// Runtime: goja (ES2022: classes, async/await, optional chaining, BigInt…;
// esbuild lowers async generators). Imports: your own files (./x) and active
// JS plugins ('<plugin id>', typed at the end of this file). No npm packages,
// fs, net, process, fetch or setTimeout — use http.fetch and jobs instead.
// Reference: docs/site-code.md · CLI: docs/cli.md (anasaki-code types).

/** An HTTP request reaching a Site Code route. / คำขอที่เข้ามาที่ route */
interface SiteRequest {
  /** "GET", "POST", … */
  method: string;
  /** Request path, e.g. "/hello/world". / path ของคำขอ */
  path: string;
  /** Path parameters: `{name}` → params.name, a trailing `/*` → params['*']. / พารามิเตอร์ใน path */
  params: Record<string, string>;
  /** Query string values (an array when a key repeats). / ค่าใน query string */
  query: Record<string, string | string[]>;
  /** Request headers, lower-case names (Authorization is never exposed). / header (ชื่อตัวพิมพ์เล็ก) */
  headers: Record<string, string>;
  /**
   * The site's own cookies: only names starting with `pl_site_`, prefix removed
   * (set them with `response.setCookie`). / cookie ของเว็บ (ขึ้นต้น pl_site_ ตัด prefix แล้ว)
   */
  cookies: Record<string, string>;
  /** Raw body (max 1 MB). / body ดิบ */
  body: string;
  /** Parses the body as JSON (throws on invalid JSON). / แปลง body เป็น JSON */
  json<T = any>(): T;
}

/** The signed-in CMS user (admin bearer token), or null. / ผู้ใช้ CMS ที่ล็อกอิน */
interface SiteUser {
  id: number;
  name: string;
  email: string;
  /** Role slug, e.g. "administrator", "editor". / รหัส role */
  role: string;
  /** Capabilities of the role (in a JS plugin: the core ones and the plugin's own "<id>.*"). / สิทธิ์ของ role */
  caps: string[];
  /**
   * Whether the user's role has a capability: a core one ('edit_pages') or, since 2.0.6, one of the plugin's
   * own capabilities declared in the manifest ('analytics.view'). Naming another plugin's capability throws.
   * ตรวจว่ามีสิทธิ์นี้ไหม (ปลั๊กอินตรวจได้เฉพาะสิทธิ์ของตัวเอง)
   */
  can(cap: string): boolean;
}

/** The argument of a route handler. / อาร์กิวเมนต์ของ handler */
interface CookieOptions {
  /** Seconds; 0 deletes the cookie. Default: a session cookie. */
  maxAge?: number;
  /** Default true. */
  httpOnly?: boolean;
  /** Default 'Lax'. */
  sameSite?: 'Lax' | 'Strict' | 'None';
  /** Default '/'. */
  path?: string;
}

/** What a handler can do to the response. / จัดการคำตอบ */
interface SiteResponse {
  /**
   * Sets a cookie; the name is prefixed with `pl_site_` (Secure on https).
   * ตั้ง cookie (ชื่อจะขึ้นต้นด้วย pl_site_ อัตโนมัติ)
   */
  setCookie(name: string, value: string, opts?: CookieOptions): void;
}

interface RouteContext {
  request: SiteRequest;
  response: SiteResponse;
  /** null for anonymous visitors. / null ถ้าไม่ได้ล็อกอิน */
  user: SiteUser | null;
}

/** A full response. / คำตอบแบบกำหนดเอง */
interface RouteResponse {
  /** HTTP status 100-599. */
  status: number;
  headers?: Record<string, string>;
  /**
   * A string is sent as is (text/html unless Content-Type is set); a Uint8Array / ArrayBuffer is sent
   * verbatim (application/octet-stream unless set, at most 20 MB); anything else as JSON.
   * ข้อความส่งตามนั้น, Uint8Array/ArrayBuffer ส่งเป็นไบต์ (ไม่เกิน 20 MB), อย่างอื่นเป็น JSON
   */
  body?: any;
  /** Binary body as base64 (instead of body). / body แบบ base64 */
  bodyBase64?: string;
}

/**
 * What a handler may return: an object/array → JSON 200; a string → text/html 200;
 * `{ status, headers?, body? }` (no other keys) → as given; nothing → 204.
 * A Promise (async handler) is awaited.
 * ค่าที่คืนได้: object/array = JSON, ข้อความ = HTML, {status, headers, body} = ตามที่กำหนด
 */
type RouteResult = RouteResponse | RoutePageResult | object | string | number | boolean | null | undefined | void;

interface RouteOptions {
  /**
   * Who may call the route: `['*']` = anyone, otherwise role slugs (the administrator always may),
   * `'group:<name>'` (visitor groups) and, since 2.0.6, `'cap:<capability>'` — every role holding the
   * capability (Users → Roles), e.g. `['cap:analytics.view']`. A plugin names only its own capabilities.
   * Default: `['administrator']`.
   * ใครเรียกได้: ['*'] = ทุกคน หรือรหัส role; ค่าเริ่มต้น = ผู้ดูแลระบบเท่านั้น
   */
  access?: string[];
}

/**
 * Who made a change (CMS 2.1.0): the user (0 = none: a visitor, db.asSystem, the scheduler) and where
 * it came from: 'admin', 'api' (personal access token), 'code' (Site Code), 'plugin:<id>' (a JS
 * plugin), 'scheduler', 'setup', 'password-reset' or 'system'. / ใครเป็นผู้แก้ไข และมาจากไหน
 */
interface EventActor {
  id: number;
  source: string;
}

/** A post in an event payload. / โพสต์ใน payload ของ event */
interface PostEvent {
  id: number;
  type: string;
  title: string;
  slug: string;
  status: PostStatus;
  url: string;
  /** 2.1.0 */
  authorId: number;
  /** RFC 3339; null when never published. 2.1.0 */
  publishedAt: string | null;
  /** 2.1.0 */
  actor: EventActor;
}

interface UserEvent { id: number; email: string; name: string; role: string; status: string; actor: EventActor }
interface MediaEvent {
  id: number; filename: string; url: string; mime: string; size: number;
  /** image | video | audio | document | archive | other (2.1.0) */
  kind: string; width: number; height: number; alt: string; authorId: number; actor: EventActor;
}
interface TermEvent { id: number; taxonomy: string; name: string; slug: string; actor: EventActor }

/**
 * Core events delivered to site.on. Since 2.1.0 the store fires them for every write (the admin,
 * Site Code, JS plugins, the scheduler); each payload has `actor`, updates carry `previous`.
 * เหตุการณ์ของระบบ (2.1.0: ยิงจากทุกช่องทางที่เขียนข้อมูล พร้อม actor และค่าเดิม)
 */
interface SiteEvents {
  /** A post/page became published (also by the scheduler). / เผยแพร่เนื้อหา */
  'post.published': PostEvent;
  /** A post/page was created or saved (isNew; previous on an update). / บันทึกเนื้อหา */
  'post.saved': PostEvent & { isNew: boolean; previous?: { title: string; slug: string; status: PostStatus } };
  /** A post/page was deleted. / ลบเนื้อหา */
  'post.deleted': PostEvent;
  /** 2.1.0: a post/page was created. / สร้างเนื้อหา */
  'post.created': PostEvent;
  /** 2.1.0: a post/page was updated, with the values before. / แก้ไขเนื้อหา (มีค่าเดิม) */
  'post.updated': PostEvent & { previous: { title: string; slug: string; status: PostStatus; authorId: number; publishedAt: string | null } };
  /** 2.1.0: the status changed (WordPress transition_post_status); from is '' for a new post. / สถานะเปลี่ยน */
  'post.statusChanged': PostEvent & { from: PostStatus | ''; to: PostStatus };
  /** 2.1.0: a post was scheduled (or its time moved). / ตั้งเวลาเผยแพร่ */
  'post.scheduled': PostEvent;
  /** 2.1.0: a published post went back to draft or scheduled. / เลิกเผยแพร่ */
  'post.unpublished': PostEvent;
  /** A user account was created. / สร้างผู้ใช้ */
  'user.created': UserEvent;
  /** 2.1.0: a profile, role or status changed (never the password itself). / แก้ไขผู้ใช้ */
  'user.updated': UserEvent & { previous: { email: string; name: string; role: string; status: string }; passwordChanged: boolean };
  /** 2.1.0 / เปลี่ยน role */
  'user.roleChanged': UserEvent & { from: string; to: string };
  /** 2.1.0: deleted; their content went to reassignedTo. / ลบผู้ใช้ */
  'user.deleted': UserEvent & { reassignedTo: number };
  /** A media file was uploaded. / อัปโหลดไฟล์ */
  'media.uploaded': MediaEvent;
  /** 2.1.0 (alt text) / แก้ไขไฟล์ */
  'media.updated': MediaEvent & { previous: { alt: string } };
  /** 2.1.0 / ลบไฟล์ */
  'media.deleted': MediaEvent;
  /** 2.1.0 / สร้างหมวด/แท็ก */
  'term.created': TermEvent;
  /** 2.1.0 / แก้ไขหมวด/แท็ก */
  'term.updated': TermEvent & { previous: { name: string; slug: string } };
  /** 2.1.0 / ลบหมวด/แท็ก */
  'term.deleted': TermEvent;
  /** 2.1.0: site settings saved in the admin; the changed keys, never the values. / ตั้งค่าเว็บถูกบันทึก */
  'settings.updated': { keys: string[]; actor: EventActor };
  /** 2.1.0 / เปิดปลั๊กอิน */
  'plugin.activated': { id: string; version: string; actor: EventActor };
  /** 2.1.0 / ปิดปลั๊กอิน */
  'plugin.deactivated': { id: string; version: string; actor: EventActor };
}

interface Site {
  /**
   * Registers a route on the public site, e.g. `"GET /hello/{name}"` or `"GET /files/*"`.
   * Methods: GET, POST, PUT, PATCH, DELETE. Reserved paths (/api, /admin, /uploads, the blog
   * and category bases, …) are refused when publishing. Call only at the top level.
   * ลงทะเบียน route บนหน้าเว็บ (เรียกตอนเริ่มต้นของโค้ดเท่านั้น)
   */
  route(spec: string, handler: (ctx: RouteContext) => RouteResult | Promise<RouteResult>): void;
  route(spec: string, opts: RouteOptions, handler: (ctx: RouteContext) => RouteResult | Promise<RouteResult>): void;
  /**
   * Runs after an event happened (asynchronously, 10 s limit). Call only at the top level.
   * ทำงานหลังเกิดเหตุการณ์ (ไม่ขวางผู้ใช้)
   */
  on<E extends keyof SiteEvents>(event: E, handler: (data: SiteEvents[E]) => void | Promise<void>): void;
  on(event: string, handler: (data: any) => void | Promise<void>): void;
  /**
   * A data source for collection nodes in the designer (`code:<name>`).
   * แหล่งข้อมูลให้ block รายการใน designer
   */
  collection(name: string, spec: CollectionSpec): void;
  /** Something a button or form calls (POST /_a/<name>). / สิ่งที่ปุ่ม/ฟอร์มเรียก */
  action(name: string, spec: ActionSpec): void;
  /** `{{page.*}}` data for pages at a path ("/checkout", "/events/{slug}"). / ข้อมูลระดับหน้า */
  page(path: string, spec: PageSpec): void;
  /**
   * A formatter usable in bindings as `| name:arg` (50 ms per call). In a JS plugin it is `| <id>:name`
   * (e.g. `{{ price | thai:baht }}`); the administrator may enable the unprefixed alias.
   * ตัวจัดรูปแบบใน binding (ในปลั๊กอินใช้ | <id>:ชื่อ)
   */
  formatter(name: string, fn: (value: any, ...args: any[]) => string): void;
}

type FieldType = 'text' | 'number' | 'date' | 'datetime' | 'image' | 'url' | 'html' | 'bool' | 'json';

interface CollectionContext {
  /** 1-based. */
  page: number;
  perPage: number;
  /** Declared params only, typed. / เฉพาะ params ที่ประกาศ */
  params: Record<string, any>;
  request: SiteRequest;
  response: SiteResponse;
  user: SiteUser | null;
}

interface CollectionSpec {
  label?: string | { th?: string; en?: string };
  /** Fields the designer can bind ({{item.x}}); 'html' values are sanitised. */
  fields: Record<string, FieldType>;
  /** Accepted params (others are dropped). */
  params?: Record<string, 'text' | 'number' | 'bool'>;
  /** default 10, max 50 (hard cap 100). */
  perPage?: { default?: number; max?: number };
  /** Seconds; cached per params, page, perPage and viewer role. */
  cache?: number;
  /** Default ['administrator']; '*' = anyone. */
  access?: string[];
  /** 2 s limit. A plain array is the whole collection (paged by the system). */
  load(ctx: CollectionContext): { items: object[]; total?: number } | object[] | Promise<{ items: object[]; total?: number } | object[]>;
}

interface ActionResult {
  /** Shown as a toast. */
  message?: string;
  /** Same-site path only ("/thanks"). */
  redirect?: string;
  /** Per-field messages, shown under the inputs. */
  errors?: Record<string, string>;
  /** Collections (site.collection names) to re-render on the page. */
  refresh?: string[];
  /** Layer names to re-render. */
  update?: string[];
  /** Merged into {{page.*}} for the re-render. */
  page?: object;
  /** Returned to the client. */
  data?: any;
}

interface ActionSpec {
  /** Validated before run; '?' = optional. */
  input?: Record<string, 'text' | 'text?' | 'number' | 'number?' | 'bool' | 'email' | 'email?' | 'json' | 'json?'>;
  /** Default ['administrator']. */
  access?: string[];
  run(ctx: {
    input: Record<string, any>; request: SiteRequest; response: SiteResponse; user: SiteUser | null;
    /** The visitor (site.visitor), resolved when first read. / ผู้เยี่ยมชม */
    readonly visitor: SiteVisitor | null;
    /** The signed-in user's admin language when an admin page calls the action (anasaki.call). */
    locale?: 'th' | 'en';
  }): ActionResult | void | Promise<ActionResult | void>;
}

interface PageSpec {
  /** Default ['*']. */
  access?: string[];
  load(ctx: {
    request: SiteRequest; response: SiteResponse; params: Record<string, string>; user: SiteUser | null;
    readonly visitor: SiteVisitor | null;
  }): object | Promise<object>;
}

/** A post or page as scripts see it. / โพสต์หรือหน้า */
interface Post {
  id: number;
  /** 'post', 'page' or a site.postType id. */
  type: string;
  title: string;
  slug: string;
  /**
   * "publish", "draft" or "scheduled" (2.1.0). Drafts and scheduled posts only reach users who may
   * edit them. / สถานะ
   */
  status: PostStatus;
  /** Public path, e.g. "/blog/hello". / path สาธารณะ */
  url: string;
  excerpt: string;
  /** Sanitised HTML. / HTML ที่กรองแล้ว */
  content: string;
  /** Featured image URL ("" = none). / รูปเด่น */
  image: string;
  /** Name of the first category ("" = none). / ชื่อหมวดแรก */
  category: string;
  categories: { slug: string; name: string }[];
  /** Author display name. / ชื่อผู้เขียน */
  author: string;
  /** RFC 3339, null for a draft. For a scheduled post: when it goes live. */
  publishedAt: string | null;
  /** RFC 3339. */
  updatedAt: string;
  /** RFC 3339 (2.1.0). */
  createdAt: string;
  /** Author user id (2.1.0). / id ผู้เขียน */
  authorId: number;
  /** Featured image media id, null = none (2.1.0). / id รูปเด่น */
  imageId: number | null;
  /** Shown first in lists (2.1.0). / ปักหมุด */
  pinned: boolean;
  /** Position in the manual order, 0 = not placed (2.1.0). / ลำดับที่จัดเอง */
  sortOrder: number;
  /** Terms of every taxonomy by taxonomy id (2.1.0). / หมวดทุกชุด */
  terms: Record<string, { id: number; slug: string; name: string; parent: number }[]>;
}

/** 'scheduled' (CMS 2.1.0): waits for publishedAt, then core publishes it. */
type PostStatus = 'draft' | 'publish' | 'scheduled';

interface PostListOptions {
  /** Default 'post'; also a site.postType id. */
  type?: string;
  /** Category slug. / slug ของหมวด */
  category?: string;
  /** Searches title and content. / ค้นหา */
  search?: string;
  /** Default 'newest'. */
  order?: 'newest' | 'oldest' | 'title';
  /** 1-based, default 1. */
  page?: number;
  /** 1-100, default 10. */
  perPage?: number;
  /**
   * 2.1.0: only this status, or 'any'. Without the right to see drafts of the type only 'publish'
   * matches (another status gives an empty list). / กรองสถานะ
   */
  status?: PostStatus | 'any';
  /** 2.1.0: only this author's posts. / เฉพาะผู้เขียนคนนี้ */
  authorId?: number;
  /** 2.1.0: only these posts (at most 200 ids). / เฉพาะ id เหล่านี้ */
  ids?: number[];
}

/** A result row. / แถวผลลัพธ์ */
type Row = Record<string, any>;

/**
 * Row types per table (filled in from your tables after publishing).
 * type ของแถวแต่ละตาราง (สร้างให้อัตโนมัติหลังเผยแพร่)
 */
interface TableRows {
  core_posts: CorePostsRow;
  core_terms: CoreTermsRow;
  core_post_terms: CorePostTermsRow;
  core_users: CoreUsersRow;
}

/** Published posts/pages (read-only view, separate database only). */
interface CorePostsRow {
  id: number; type: string; title: string; slug: string; status: string; excerpt: string | null;
  author_id: number; featured_media_id: number | null; published_at: string | null; created_at: string; updated_at: string;
}
/** Categories and tags (read-only view). */
interface CoreTermsRow { id: number; taxonomy: string; name: string; slug: string; parent: number }
/** Post / term links of published posts (read-only view). */
interface CorePostTermsRow { post_id: number; term_id: number }
/** CMS users without passwords or tokens (read-only view). */
interface CoreUsersRow { id: number; name: string; email: string; role: string; status: string; created_at: string }

/** A raw SQL fragment with bound parameters (db.raw). / ส่วนของ SQL ดิบ */
interface RawExpr { readonly __raw: true }

/** A column name ("col", "t.col", "t.col as x") or db.raw. */
type Column<T = Row> = (keyof T & string) | string | RawExpr;
type Operator = '=' | '!=' | '<>' | '<' | '<=' | '>' | '>=' | 'like' | 'not like' | 'in' | 'not in';
/** A bound value. Date objects and toISOString() strings are sent as DATETIME. */
type SqlValue = string | number | boolean | null | Date | RawExpr | object;

/** A page of rows. / ผลลัพธ์แบบแบ่งหน้า */
interface Page<T> { items: T[]; total: number; page: number; perPage: number }

/**
 * Query builder (synchronous). Values are always bound parameters; identifiers are validated.
 * Results: DECIMAL = string, DATETIME/DATE = ISO string, JSON = parsed, BIGINT = number (string beyond
 * 2^53), TINYINT = boolean. Max 10,000 rows / 10 MB per result; 5 s per statement.
 * ตัวสร้าง query: ค่าทุกค่าเป็น parameter
 */
interface QueryBuilder<T = Row> {
  select(...columns: (Column<T> | Column<T>[])[]): QueryBuilder<any>;
  distinct(): this;
  join(table: string, a: string, b: string): this;
  join(table: string, a: string, op: string, b: string): this;
  leftJoin(table: string, a: string, b: string): this;
  leftJoin(table: string, a: string, op: string, b: string): this;
  rightJoin(table: string, a: string, b: string): this;
  rightJoin(table: string, a: string, op: string, b: string): this;
  where(column: Column<T>, value: SqlValue): this;
  where(column: Column<T>, op: Operator, value: SqlValue | SqlValue[]): this;
  where(conditions: Partial<T> | Record<string, SqlValue>): this;
  where(group: (q: QueryBuilder<T>) => void): this;
  where(raw: RawExpr): this;
  orWhere(column: Column<T>, value: SqlValue): this;
  orWhere(column: Column<T>, op: Operator, value: SqlValue | SqlValue[]): this;
  orWhere(conditions: Partial<T> | Record<string, SqlValue>): this;
  orWhere(group: (q: QueryBuilder<T>) => void): this;
  orWhere(raw: RawExpr): this;
  whereIn(column: Column<T>, values: SqlValue[]): this;
  whereNotIn(column: Column<T>, values: SqlValue[]): this;
  orWhereIn(column: Column<T>, values: SqlValue[]): this;
  orWhereNotIn(column: Column<T>, values: SqlValue[]): this;
  whereNull(column: Column<T>): this;
  whereNotNull(column: Column<T>): this;
  orWhereNull(column: Column<T>): this;
  orWhereNotNull(column: Column<T>): this;
  whereBetween(column: Column<T>, range: [SqlValue, SqlValue]): this;
  whereNotBetween(column: Column<T>, range: [SqlValue, SqlValue]): this;
  orWhereBetween(column: Column<T>, range: [SqlValue, SqlValue]): this;
  /** LIKE with your pattern ('A%'). */
  whereLike(column: Column<T>, pattern: string): this;
  orWhereLike(column: Column<T>, pattern: string): this;
  groupBy(...columns: Column<T>[]): this;
  having(column: Column<any>, value: SqlValue): this;
  having(column: Column<any>, op: Operator, value: SqlValue): this;
  having(raw: RawExpr): this;
  orHaving(column: Column<any>, op: Operator, value: SqlValue): this;
  orderBy(column: Column<T>, direction?: 'asc' | 'desc'): this;
  orderByDesc(column: Column<T>): this;
  /** ORDER BY column (default created_at) DESC. */
  latest(column?: Column<T>): this;
  oldest(column?: Column<T>): this;
  limit(n: number): this;
  take(n: number): this;
  offset(n: number): this;
  skip(n: number): this;
  /** Allows update()/delete() without where (the whole table). / อนุญาตให้แก้/ลบทั้งตาราง */
  all(): this;

  get(): T[];
  first(): T | null;
  pluck<K extends keyof T & string>(column: K): T[K][];
  pluck(column: string): any[];
  /** perPage 1-100 (default 20). */
  paginate(page?: number, perPage?: number): Page<T>;
  count(): number;
  exists(): boolean;
  sum(column: Column<T>): number | null;
  avg(column: Column<T>): number | null;
  min(column: Column<T>): any;
  max(column: Column<T>): any;
  /** Inserts one row or many (max 1000); returns the (first) inserted id. / เพิ่มแถว คืน id */
  insert(row: Partial<T> | Record<string, SqlValue> | (Partial<T> | Record<string, SqlValue>)[]): number;
  /** Needs where() (or all()); returns the number of changed rows. / ต้องมี where */
  update(fields: Partial<T> | Record<string, SqlValue>): number;
  /** Needs where() (or all()); returns the number of deleted rows. */
  delete(): number;
  increment(column: string, by?: number, extra?: Record<string, SqlValue>): number;
  decrement(column: string, by?: number, extra?: Record<string, SqlValue>): number;
  /** The SELECT statement and its parameters (debugging). */
  toSQL(): { sql: string; bindings: any[] };
}

/** JSON documents in kv_docs. / เก็บ JSON ตามชื่อ */
interface DocCollection<T = Record<string, any>> {
  insert(doc: T): number;
  get(id: number): (T & { id: number }) | null;
  /** JSON merge patch; false when the document does not exist. */
  update(id: number, patch: Partial<T>): boolean;
  delete(id: number): boolean;
  /** where: field (dots for nested) = value; orderBy 'field', '-field', 'field desc', 'id', 'createdAt'. */
  find(opts?: { where?: Record<string, any>; orderBy?: string; page?: number; perPage?: number }): Page<T & { id: number }>;
}

/** Table access shared by db and transactions. */
interface DbTables {
  /** A query on one of your tables ("orders" or "orders as o"); core_* views need the separate database. */
  table<K extends keyof TableRows>(name: K): QueryBuilder<TableRows[K]>;
  table(name: string): QueryBuilder<Row>;
  from<K extends keyof TableRows>(name: K): QueryBuilder<TableRows[K]>;
  from(name: string): QueryBuilder<Row>;
  /** A raw SQL fragment, `?` bound to params (separate database only). / SQL ดิบ (เฉพาะฐานข้อมูลแยก) */
  raw(sql: string, params?: SqlValue[]): RawExpr;
  /**
   * Raw SQL as the code database user (separate database only; one statement).
   * SELECT returns rows; anything else { affectedRows, insertId }. / SQL ตรง (เฉพาะฐานข้อมูลแยก)
   */
  query<R = Row>(sql: string, params?: SqlValue[]): any;
  docs<T = Record<string, any>>(collection: string): DocCollection<T>;
}

interface Db extends DbTables {
  /**
   * Runs fn in a transaction: committed when it returns, rolled back when it throws.
   * fn must be synchronous; use `tx` (not `db`) inside. / ทำทั้งหมดหรือไม่ทำเลย
   */
  transaction<R>(fn: (tx: DbTables) => R): R;
  posts: {
    /**
     * Lists posts/pages: published only, unless ctx.user may edit that type.
     * รายการโพสต์ (เฉพาะที่เผยแพร่ ยกเว้นผู้ใช้มีสิทธิ์แก้ไข)
     */
    list(opts?: PostListOptions): { items: Post[]; total: number };
    /** One post/page by id or slug (null when missing or not visible). / ดึงโพสต์ตาม id หรือ slug */
    get(idOrSlug: number | string, opts?: { type?: string }): Post | null;
    /**
     * Creates a post (checked against ctx.user's capabilities — visitors cannot write; see
     * db.asSystem). Runs post.beforeSave; the store fires post.created / post.saved / post.published
     * (actor source 'code', or 'plugin:<id>' in a JS plugin).
     * สร้างโพสต์ (ตรวจสิทธิ์ของผู้เรียก)
     */
    create(post: PostInput): Post;
    /** Updates a post (same checks). / แก้ไขโพสต์ */
    update(id: number, patch: PostInput): Post;
    /** Deletes a post (same checks). / ลบโพสต์ */
    delete(id: number): boolean;
  };
  /** Runs fn with system rights (content writes); audited as `as_system` with file:line. / ทำแทนระบบ */
  asSystem<R>(fn: () => R): R;
  /**
   * Terms of 'category' and the site.taxonomy taxonomies (CMS 2.1.0). Reading needs scope "content";
   * writing needs "terms-write", the right to manage the taxonomy (or db.asSystem) and is limited to
   * 'category' and the code's own taxonomies. / หมวดหมู่
   */
  terms: {
    /** By name; default taxonomy 'category', at most `limit` (default 200, ≤ 1000). */
    list(opts?: { taxonomy?: string; parent?: number; search?: string; postId?: number; ids?: number[]; slugs?: string[]; limit?: number }): Term[];
    get(idOrSlug: number | string, opts?: { taxonomy?: string }): Term | null;
    create(term: TermInput & { taxonomy: string; name: string }): Term;
    update(id: number, patch: TermInput): Term;
    /** Its children move up to its parent. */
    delete(id: number): boolean;
  };
}

interface Logger {
  /** Written to the Site Code log (Logs screen). / เขียนลงบันทึกการทำงาน */
  debug(...args: any[]): void;
  info(...args: any[]): void;
  warn(...args: any[]): void;
  error(...args: any[]): void;
}

interface SiteCache {
  /** The stored value, or undefined (expired / missing). / อ่านค่า */
  get<T = any>(key: string): T | undefined;
  /** Stores a JSON-serialisable value (10 MB for the whole site). ttlSeconds 0/omitted = no expiry. / เก็บค่า */
  set(key: string, value: any, ttlSeconds?: number): void;
  delete(key: string): void;
}

interface SiteCrypto {
  /** `bytes` random bytes (1-1024, default 16) as hex. / สุ่ม byte เป็น hex */
  random(bytes?: number): string;
  /** A random UUID v4. */
  uuid(): string;
  /** Digest of a string as hex. / hash เป็น hex */
  hash(algorithm: 'sha256' | 'sha1' | 'sha512' | 'md5', data: string): string;
  /** HMAC of a string as hex. / HMAC เป็น hex */
  hmac(algorithm: 'sha256' | 'sha1' | 'sha512', key: string, data: string): string;
  /** Constant-time string comparison. / เทียบข้อความแบบเวลาคงที่ */
  timingSafeEqual(a: string, b: string): boolean;
  password: {
    /** bcrypt hash (max 72 bytes). / hash รหัสผ่าน */
    hash(password: string): string;
    /** Checks a password against a bcrypt hash. / ตรวจรหัสผ่าน */
    verify(password: string, hash: string): boolean;
  };
}

/** Routes and events. / route และเหตุการณ์ */
declare const site: Site;
/** CMS posts and your own database (usable only inside handlers, not at the top level). / ข้อมูลของระบบและฐานข้อมูลของคุณ */
declare const db: Db;
/** Logging. / บันทึก */
declare const log: Logger;
/** Same as log (console.log = log.info). */
declare const console: Logger & { log(...args: any[]): void };
/** In-memory cache shared by every request (usable only inside handlers). / แคชในหน่วยความจำ */
declare const cache: SiteCache;
/** Random values, hashes and passwords (usable only inside handlers). / ฟังก์ชันเข้ารหัส */
declare const crypto: SiteCrypto;

// ---------------------------------------------------------------- migrations (migrations/NNN_name.ts)

/** A column being defined. / คอลัมน์ */
interface ColumnBuilder {
  nullable(value?: boolean): this;
  default(value: string | number | boolean | null): this;
  /** DEFAULT CURRENT_TIMESTAMP (datetime). */
  useCurrent(): this;
  /** ON UPDATE CURRENT_TIMESTAMP (datetime). */
  useCurrentOnUpdate(): this;
  unique(): this;
  index(): this;
  unsigned(): this;
  comment(text: string): this;
  /** Position of a new column (schema.table only). */
  after(column: string): this;
  /** Redefines an existing column (schema.table only). / แก้คอลัมน์เดิม */
  change(): this;
  /** Foreign key to one of your tables; core_users / core_posts / core_terms are recorded as metadata only. */
  references(table: string, column?: string): this;
  onDelete(action: 'cascade' | 'restrict' | 'set null'): this;
}

/** The `t` of schema.create / schema.table. */
interface TableBuilder {
  /** BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY (default name "id"). */
  id(name?: string): ColumnBuilder;
  /** VARCHAR(length = 255). */
  string(name: string, length?: number): ColumnBuilder;
  text(name: string): ColumnBuilder;
  integer(name: string): ColumnBuilder;
  bigInteger(name: string): ColumnBuilder;
  /** DECIMAL(precision = 12, scale = 2), read back as a string. */
  decimal(name: string, precision?: number, scale?: number): ColumnBuilder;
  /** TINYINT(1), read back as a boolean. */
  boolean(name: string): ColumnBuilder;
  date(name: string): ColumnBuilder;
  /** DATETIME(3). */
  datetime(name: string): ColumnBuilder;
  json(name: string): ColumnBuilder;
  enum(name: string, values: string[]): ColumnBuilder;
  /** CHAR(36). */
  uuid(name: string): ColumnBuilder;
  /** BIGINT UNSIGNED, usually with .references('table'). */
  foreignId(name: string): ColumnBuilder;
  /** created_at and updated_at DATETIME(3) with defaults / ON UPDATE. */
  timestamps(): void;
  index(columns: string | string[], name?: string): void;
  unique(columns: string | string[], name?: string): void;
  /** Destructive (publish asks for confirmation, the table is backed up first). / ทำข้อมูลหาย */
  dropColumn(name: string | string[]): void;
  /** Destructive. */
  renameColumn(from: string, to: string): void;
  /** Same as column.change(): t.change(t.string('name', 100)). Destructive when the type gets smaller. */
  change(column: ColumnBuilder): ColumnBuilder;
  dropIndex(name: string): void;
  dropUnique(name: string): void;
  /** Drops the foreign key (or core_* relation) of a column. */
  dropForeign(column: string): void;
}

/** The `schema` argument of up() / down(). Names: a-z 0-9 _, not core_ / script_ / sc_ / bak_ / kv_docs. */
interface SchemaBuilder {
  create(table: string, fn: (t: TableBuilder) => void): void;
  table(table: string, fn: (t: TableBuilder) => void): void;
  /** Destructive. */
  drop(table: string): void;
  dropIfExists(table: string): void;
  /** Destructive. */
  rename(from: string, to: string): void;
  hasTable(table: string): boolean;
  hasColumn(table: string, column: string): boolean;
  /** Raw DDL (separate database only). */
  raw(sql: string): void;
}

interface MigrationDefinition {
  up(schema: SchemaBuilder): void;
  down?(schema: SchemaBuilder): void;
}

/**
 * Declares a migration: `export default migration({ up(schema) {...}, down(schema) {...} })`.
 * Pending migrations run in file-name order when publishing, before the new code goes live.
 * ประกาศ migration (รันตอนเผยแพร่ตามลำดับชื่อไฟล์)
 */
declare function migration(def: MigrationDefinition): MigrationDefinition;

// ---------------------------------------------------------------- phase 4a: hooks, visitor, post types, services, jobs, admin pages
// (declaration merging: these add to the interfaces above)

/** The visitor of a request (the site.visitor result); null = anonymous. / ผู้เยี่ยมชม */
interface SiteVisitor {
  id: string | number;
  name?: string;
  /** Group names, e.g. ['member', 'gold']: access 'group:<name>' and {{visitor.is.<name>}}. */
  groups: string[];
  [extra: string]: any;
}

interface RouteContext {
  /** The visitor (site.visitor, resolved lazily, once per request). / ผู้เยี่ยมชม */
  readonly visitor: SiteVisitor | null;
}
interface CollectionContext {
  readonly visitor: SiteVisitor | null;
}

interface SiteRequest {
  /** Exact body as UTF-8 (routes with `rawBody: true`, e.g. webhook signatures). / body ดิบ */
  rawBody?: string;
  /** Exact body bytes as base64 (`rawBody: true`). */
  rawBodyBase64?: string;
  /** The files of a multipart/form-data body (CMS 2.1.0; raise the route's maxBody for big files). / ไฟล์ที่ส่งมา */
  files(): { field: string; filename: string; type: string; size: number; bytes: Uint8Array }[];
  /** The text fields of a multipart/form-data body (CMS 2.1.0). */
  form(): Record<string, string | string[]>;
}

interface RouteOptions {
  /** Adds request.rawBody / rawBodyBase64 (webhooks). */
  rawBody?: boolean;
  /** Body limit in bytes (default 1 MB, at most 5 MB); larger → 413. */
  maxBody?: number;
}

interface Post {
  /** Custom fields (site.postType / site.fields). / ฟิลด์เพิ่มเติม */
  meta: Record<string, any>;
}

type MetaCondition = string | number | boolean | {
  '='?: any; '!='?: any; '<'?: any; '<='?: any; '>'?: any; '>='?: any;
  eq?: any; ne?: any; lt?: any; lte?: any; gt?: any; gte?: any; in?: any[];
};

interface PostListOptions {
  /** Filters on top-level custom fields: { 'meta.date': { gte: '2026-01-01' } }. */
  where?: Record<string, MetaCondition>;
  /** 'meta.<field>' or '-meta.<field>' (descending). */
  orderBy?: string;
}

/** What db.posts.create / update accept. / ข้อมูลโพสต์ที่เขียนได้ */
interface PostInput {
  type?: string;
  title?: string;
  slug?: string;
  /**
   * 'scheduled' needs publishedAt; 'publish' with a future publishedAt is scheduled too (2.1.0).
   * Without the right to publish the type the post is saved as a draft.
   */
  status?: PostStatus;
  excerpt?: string;
  content?: string;
  meta?: Record<string, any>;
  /** 2.1.0: the author (needs the right to edit others' posts). / ผู้เขียน */
  authorId?: number;
  /** 2.1.0: Date, ISO 8601 or epoch ms; in the future it schedules, in the past it back-dates; null clears. */
  publishedAt?: Date | string | number | null;
  /** 2.1.0: featured image media id; null or 0 removes it. / รูปเด่น */
  imageId?: number | null;
  /** 2.1.0 (needs the right to edit others' posts). / ปักหมุด */
  pinned?: boolean;
  /** 2.1.0 (needs the right to edit others' posts). / ลำดับที่จัดเอง */
  sortOrder?: number;
  /** 2.1.0: designer document (needs edit_pages; code blocks need edit_scripts). / ดีไซน์ */
  design?: Record<string, any>;
  /**
   * 2.1.0: terms per taxonomy, ids or slugs of existing terms; replaces that taxonomy's terms. The
   * taxonomy must be 'category' (posts, types supporting categories) or the code's own one listing the type.
   */
  terms?: Record<string, (number | string)[]>;
}

/** Core filter points. / จุด filter ของระบบ */
interface SiteFilters {
  /** Post HTML on the public site (50 ms). ctx {post} */
  'post.content': string;
  /** Before a save (admin or db.posts, 2 s); return {error: '…'} to refuse. ctx {user, isNew} */
  'post.beforeSave': PostInput & { error?: string };
  /** <head> values (50 ms); title is the full <title>. ctx {post, path} */
  'page.head': { title: string; description: string; image: string; noindex: boolean };
  /** Items after a collection load (50 ms). ctx {name, params, page} */
  'collection.items': any[];
  /** ctx {name, user, visitor} */
  'action.input': Record<string, any>;
  /** ctx {name, user, visitor} */
  'action.result': ActionResult;
  /** Return null to cancel. ctx {source} */
  'mail.beforeSend': MailMessage | null;
  /**
   * 2.1.0: before a file joins the media library (admin upload, URL import, media.upload). Change the
   * filename (same extension) or alt, or return {error: '…'} to refuse. ctx {mime, size, kind, source, user}
   */
  'media.beforeSave': { filename: string; alt: string; error?: string };
  /** <head> meta of every public page (50 ms, phase 6). ctx {page, path} */
  'page.meta': PageMeta;
  /** /sitemap.xml entries (paths or {loc, lastmod?, changefreq?, priority?}); core serves the sitemap once a unit filters this. ctx {baseUrl, visible} */
  'sitemap.urls': (string | SitemapEntry)[];
  /** /robots.txt text; core serves it once a unit filters this or sitemap.urls. ctx {baseUrl, visible} */
  'robots.txt': string;
}

/** page.meta value. / ค่า meta ของหน้า */
interface PageMeta {
  title: string;
  /** The <title> text. */
  fullTitle: string;
  description: string;
  image: string;
  /** Absolute or site path ('' = none). */
  canonical: string;
  /** <meta name="robots"> ('' = default). */
  robots: string;
  noindex: boolean;
  /** 'og:*', 'twitter:*', 'article:*' … → content. */
  og: Record<string, string>;
}

interface SitemapEntry {
  /** A site path ('/about') or an absolute URL of this site. */
  loc: string;
  lastmod?: string;
  changefreq?: 'always' | 'hourly' | 'daily' | 'weekly' | 'monthly' | 'yearly' | 'never';
  /** 0..1 */
  priority?: number;
}

/**
 * Since CMS 2.1.0 also: relation (post ids, `postType`), user (user ids), term (term ids, `taxonomy`),
 * file (a media id), gallery (media ids), repeater (rows of `fields`), richtext (sanitised HTML), color.
 * relation / user / term / file store one id, or a list with `multiple: true` (gallery always a list).
 */
type CustomFieldType = 'text' | 'textarea' | 'number' | 'bool' | 'date' | 'datetime' | 'select' | 'url' | 'image' | 'email' | 'json'
  | 'relation' | 'user' | 'term' | 'file' | 'gallery' | 'repeater' | 'richtext' | 'color';
type I18nLabel = string | { th?: string; en?: string };
type CustomField = CustomFieldType | {
  type: CustomFieldType;
  label?: I18nLabel;
  required?: boolean;
  /** number: value range; text/textarea: length; relation / gallery …: how many; repeater: rows. */
  min?: number;
  max?: number;
  options?: (string | { value: string; label?: I18nLabel })[];
  /** relation: the content types to choose from ('post', 'page' or site.postType ids); default 'post'. */
  postType?: string | string[];
  /** relation / user / term / file: a list of ids. */
  multiple?: boolean;
  /** term: the taxonomy; default 'category'. */
  taxonomy?: string;
  /** repeater: the fields of a row (no gallery, repeater, json, richtext, user or term inside). */
  fields?: Record<string, CustomField>;
};

/** site.taxonomy (CMS 2.1.0). / ชุดหมวดหมู่ */
interface TaxonomySpec {
  label: I18nLabel;
  singular?: I18nLabel;
  /** The content types whose posts take these terms: 'post', 'page' or site.postType ids. */
  postTypes: string[];
  /** Terms have parents (like categories); default false (flat, like tags). */
  hierarchical?: boolean;
  /** Archive base /<slug>/<term-slug>; default the id with '-' for '_'. */
  slug?: string;
  /** Archive pages and sitemap entries; default true. */
  public?: boolean;
  /** Who manages the terms: role slugs or 'cap:<capability>'; default editors and administrators. */
  access?: { manage?: string[] };
}

/** A term (db.terms, CMS 2.1.0). / หมวด */
interface Term {
  id: number;
  taxonomy: string;
  name: string;
  slug: string;
  /** Parent term id (0 = top level). */
  parent: number;
  description: string;
  /** Posts having the term. */
  count: number;
}

interface TermInput {
  name?: string;
  /** Default: from the name. */
  slug?: string;
  /** Hierarchical taxonomies only; 0 or null = top level. */
  parent?: number | null;
  description?: string;
}

interface PostTypeSpec {
  label?: I18nLabel;
  singular?: I18nLabel;
  icon?: string;
  /** Public URL prefix (/<slug>/<post-slug>, archive /<slug>); default = the type id (a plugin type: the id with "_" → "-"). A plugin may use one of its own code.routes prefixes. */
  slug?: string;
  /** Default true. false: the CMS does not route /<slug> or /<slug>/<post-slug> — your own routes serve the type's pages. */
  public?: boolean;
  /** Default ['title', 'content', 'excerpt', 'image']. */
  supports?: ('title' | 'content' | 'excerpt' | 'image' | 'categories')[];
  fields?: Record<string, CustomField>;
  /** Roles (or 'cap:<capability>' entries, 2.0.6) that edit (and publish) this type; default ['editor', 'administrator']. */
  access?: { edit?: string[] };
  /**
   * Since 2.0.6: the capabilities that edit / publish the type, e.g. { edit: 'docs.edit', publish: 'docs.publish' }
   * (a plugin's own, declared in the manifest). caps.edit replaces the default role list; caps.publish limits
   * publishing (else whoever edits may publish). The REST /posts endpoints and db.posts enforce them.
   */
  caps?: { edit?: string; publish?: string };
}

interface JobContext {
  data: any;
  /** 1 for the first try. */
  attempt: number;
  jobId: number;
  /** Shown on the Jobs screen. / ความคืบหน้า */
  progress(value: number | string): void;
  user: null;
  visitor: null;
}

interface JobOptions {
  /** Seconds, at most 300 (default 60). */
  timeout?: number;
  /** At most 10 (default 3); retried after 30 s × 2^(attempt-1). */
  retries?: number;
  /** Jobs of this name running at once (default 1). */
  concurrency?: number;
}

interface AdminPageSpec {
  label: I18nLabel;
  icon?: string;
  /** Roles (default ['administrator']). */
  access?: string[];
  /** Returns HTML; inside the page, `anasaki.call(action, input)` runs a site.action as the signed-in user. */
  render(ctx: AdminPageContext):
    string | { html: string; title?: string } | Promise<string | { html: string; title?: string }>;
  /**
   * Optional file download, opened with `anasaki.download(query)` inside the page:
   * GET /ext-ui/code/<ticket>/<id>/download?… (same ticket and access as the page);
   * Content-Disposition: attachment is enforced. / ดาวน์โหลดไฟล์จากหน้าแอดมิน
   */
  download?(ctx: AdminPageContext): RouteResult | Promise<RouteResult>;
}

interface AdminPageContext {
  user: SiteUser;
  query: Record<string, string | string[]>;
  request: SiteRequest;
  /** The page id ('orders' or '<plugin>:orders'). */
  page: string;
  /** The signed-in user's admin language. / ภาษาของผู้ใช้ในแอดมิน */
  locale: 'th' | 'en';
}

interface Site {
  /** Changes a value on its way (lower priority runs first, default 10). / filter */
  filter<K extends keyof SiteFilters>(name: K, fn: (value: SiteFilters[K], ctx: any) => SiteFilters[K] | void, opts?: { priority?: number }): void;
  filter(name: string, fn: (value: any, ctx: any) => any, opts?: { priority?: number }): void;
  /** Runs your own filters ('site.*') inside a handler. */
  applyFilter<T>(name: string, value: T, ctx?: object): T;
  /** Delivers data to site.on(name) handlers ('site.*', asynchronously). */
  emit(name: string, data?: any): void;
  /** Who the visitor is (one per site; 200 ms; once per request, when needed). / ระบุตัวผู้เยี่ยมชม */
  visitor(fn: (ctx: { request: SiteRequest }) => SiteVisitor | null | Promise<SiteVisitor | null>): void;
  /** Declares visitor group names (columns on the Access screen). */
  visitorGroups(groups: string[]): void;
  /** Export a function other units can import (a plugin: only names listed in code.exports); it runs in this unit's context with JSON arguments / result. / ส่งออกฟังก์ชันให้โค้ดอื่นเรียก */
  export(name: string, fn: (...args: any[]) => any): void;
  /** A custom post type (admin menu, editor with fields, /<slug>/<post-slug>). In a JS plugin (scope "post-types", CMS 2.0.5) the id must be "<plugin id, - as _>_<name>" (≤ 20 characters), e.g. docs_doc. / ประเภทเนื้อหาใหม่ */
  postType(id: string, spec: PostTypeSpec): void;
  /**
   * A taxonomy for one or more content types (CMS 2.1.0): an admin terms screen, a panel in the post
   * editor, db.terms and, when public, archive pages /<slug>/<term-slug>. In a JS plugin (scope
   * "taxonomies") the id follows the post type rule, e.g. shop_brand. / ชุดหมวดหมู่ใหม่
   */
  taxonomy(id: string, spec: TaxonomySpec): void;
  /** Custom fields for 'post', 'page', a custom type, or '*' = every type, also ones registered later (CMS 2.0.8). / ฟิลด์เพิ่มเติม */
  fields(type: string, fields: Record<string, CustomField>): void;
  /** A background job (jobs.enqueue, site.cron). / งานเบื้องหลัง */
  job(name: string, fn: (ctx: JobContext) => any): void;
  job(name: string, opts: JobOptions, fn: (ctx: JobContext) => any): void;
  /** 5-field cron in the site timezone ('0 8 * * *', '@daily'): runs a job (by name) or a function. */
  cron(expr: string, target: string | (() => any)): void;
  /** A page in the admin (Developer menu), shown in a sandboxed iframe. / หน้าในแอดมิน */
  adminPage(id: string, spec: AdminPageSpec): void;
}

interface MailMessage {
  /** At most 20 recipients. */
  to: string | string[];
  subject: string;
  html?: string;
  text?: string;
  replyTo?: string;
}

interface FetchOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD';
  headers?: Record<string, string>;
  /** A string, or an object sent as JSON. */
  body?: string | object;
  /** Milliseconds, at most 10000 (default 5000; never past the handler's own limit). */
  timeout?: number;
}

interface FetchResponse {
  status: number;
  ok: boolean;
  /** Lower-case names. */
  headers: Record<string, string>;
  /** At most 5 MB. */
  body: string;
  url: string;
  json<T = any>(): T;
  text(): string;
}

/** A media library item (CMS 2.1.0). / ไฟล์ในคลังสื่อ */
interface MediaItem {
  id: number; url: string; filename: string; mime: string; size: number;
  /** image | video | audio | document | archive | other */
  kind: string; ext: string; width: number; height: number; alt: string; authorId: number; createdAt: string;
}

/**
 * The media library (CMS 2.1.0). Reading needs scope "media"; upload / update / replace / delete need
 * "media-write" and the caller's upload_files (own files, or edit_others_posts) — or db.asSystem. The
 * site's allowed file types and size limit apply. / คลังสื่อ
 */
declare const media: {
  /** Newest first; at most `limit` (default 100, ≤ 500). */
  list(opts?: { kind?: string; search?: string; ids?: number[]; limit?: number }): MediaItem[];
  get(id: number): MediaItem | null;
  /** The file's bytes (≤ 20 MB). */
  read(id: number): Uint8Array;
  /** From bytes, base64 or a public http(s) URL (internal addresses refused); runs media.beforeSave. */
  upload(file: { bytes?: Uint8Array; base64?: string; url?: string; filename?: string; alt?: string }): MediaItem;
  update(id: number, patch: { alt: string }): MediaItem;
  /** New content for the same file and URL (same type, e.g. an optimised image); fires media.updated {replaced: true}. */
  replace(id: number, bytes: Uint8Array): MediaItem;
  delete(id: number): boolean;
  /** Scales a JPEG / PNG down to fit (never up) and re-encodes it (JPEG quality 1-100, default 82). No scope. */
  resize(bytes: Uint8Array, opts: { maxWidth?: number; maxHeight?: number; quality?: number }): { bytes: Uint8Array; format: 'jpeg' | 'png'; width: number; height: number; size: number };
};

/**
 * The code's own files (CMS 2.1.0, scope "files"): private ones in the server's data folder, public ones
 * served at /uploads/plugins/<plugin id or "site">/<path> (jpg, png, gif, webp, pdf, mp3, mp4, webm, json,
 * txt, csv, css, woff2 only). 100 MB per plugin. Paths: a-z A-Z 0-9 . _ - / without "..". / ไฟล์ของโค้ด
 */
declare const files: {
  put(path: string, data: Uint8Array | string, opts?: { public?: boolean }): { path: string; size: number; public: boolean; url?: string };
  get(path: string): Uint8Array | null;
  text(path: string): string | null;
  list(prefix?: string): { path: string; size: number; public: boolean; url?: string; modifiedAt: string }[];
  delete(path: string): boolean;
  /** The URL of a public file (null for a private or missing one). */
  url(path: string): string | null;
};

/** Email through the site's SMTP settings (100 / hour by default). / ส่งอีเมล */
declare const mail: { send(msg: MailMessage): { ok: boolean; cancelled?: boolean } };
/** https requests to allowlisted hosts (Site code → Settings); internal addresses are refused. / เรียก API ภายนอก */
declare const http: {
  fetch(url: string, opts?: FetchOptions): FetchResponse;
  /**
   * Queues a POST of `data` as JSON to any public http(s) URL (5 s, X-Anasaki-Event: event; with secret also
   * X-Anasaki-Signature: sha256=<hex HMAC-SHA256(secret, body)>). At most 10 per handler run; failures are logged.
   * Scope "webhooks". / ส่ง webhook
   */
  webhook(url: string, event: string, data?: any, opts?: { secret?: string }): void;
};
/** Background jobs. / คิวงาน */
declare const jobs: {
  /** Returns the job id; with key, a pending job of the same name and key is reused. delay: seconds or '10m', '24h', '7d'. */
  enqueue(name: string, data?: any, opts?: { delay?: number | string; key?: string }): number;
};

// ---------------------------------------------------------------- phase 4b: JS plugins and interop

/** Other plugins at run time. / เรียกปลั๊กอินอื่น */
declare const plugins: {
  /**
   * A JS plugin's export (a name in its code.exports): runs in that plugin (JSON arguments / result,
   * time counted against yours, errors with its file:line) — no "requires" entry needed.
   * A WASM / service plugin's service.functions: POSTs {input, user, visitor} (signed), returns its
   * "result"; in a plugin the id must be in "requires" or "optionalRequires". Time: the handler's
   * remaining time, at most 10 s. Not installed / not active → throws PluginUnavailable
   * (`error.code === 'unavailable'`).
   */
  call<T = any>(pluginId: string, fn: string, ...args: any[]): T;
  /** The plugin is installed, active and running. / มีปลั๊กอินนี้ทำงานอยู่ไหม */
  has(pluginId: string): boolean;
  /** Its version, or null when not installed / not active. */
  version(pluginId: string): string | null;
};

/** Thrown by plugins.call when the target is not installed or not active. */
interface PluginUnavailable extends Error {
  name: 'PluginUnavailable';
  code: 'unavailable';
  plugin: string;
}

// ---------------------------------------------------------------- phase 6: assets, page hooks

/** A file of the calling plugin's package (site.asset). / ไฟล์ในแพ็กเกจ */
interface PackageAsset {
  path: string;
  size: number;
  /** Media type, e.g. 'font/ttf'. */
  type: string;
  /** Public URL (/plugin-assets/<id>/<path>?v=<version>) when that file type is served, else ''. */
  url: string;
  /** A new Uint8Array copy of the bytes. */
  bytes(): Uint8Array;
  /** The file as UTF-8 text. */
  text(): string;
}

/** What site.middleware sees. / ctx ของ middleware */
interface MiddlewareContext {
  /** No body (it stays for the route); plus ip, userAgent, referrer, url (path + query). */
  request: SiteRequest & { ip: string; userAgent: string; referrer: string; url: string };
  response: SiteResponse;
  /** A signed-in CMS user (admin bearer token, e.g. previews); null otherwise. */
  user: SiteUser | null;
  isAdminUser: boolean;
  readonly visitor: SiteVisitor | null;
}

/** What site.afterResponse receives. / ข้อมูลหลังตอบคำขอ */
interface ResponseInfo {
  method: string;
  path: string;
  /** Raw query string. */
  query: string;
  status: number;
  contentType: string;
  referrer: string;
  userAgent: string;
  ip: string;
  durationMs: number;
  /** Title of the page the site rendered ('' otherwise). */
  title: string;
  isAdminUser: boolean;
  /** RFC 3339 UTC. */
  at: string;
}

/** The page site.head / site.bodyEnd (and page.meta ctx.page) see. / ข้อมูลหน้า */
interface PageInfo {
  /** page | post | archive | 404 | plugin | <custom type> */
  type: string;
  /** 0 when not a post/page. */
  id: number;
  slug?: string;
  title: string;
  description: string;
  /** Absolute URL. */
  url: string;
  path: string;
  /** Absolute image URL ('' = none). */
  image: string;
  lang: string;
  isHome: boolean;
  /** 'category' | 'blog' | <custom type> on archives, '' otherwise. */
  archiveType: string;
  status: number;
  siteTitle: string;
}

interface Site {
  /**
   * A file of this plugin's package (not code/, migrations/ or manifest.json; at most 5 MB, cached per
   * version). Site Code has no package: throws. / อ่านไฟล์ในแพ็กเกจของปลั๊กอิน
   */
  asset(path: string): PackageAsset;
  /**
   * Runs before routing for public requests (not /api, the admin, /ext-ui, uploads, plugin assets):
   * return nothing to continue, or a RouteResult to answer now. 20 ms each; failures count against
   * the circuit breaker (open → skipped). Lower priority first (default 10). Scope "middleware".
   * ทำงานก่อนจัดเส้นทางทุกคำขอ (คืนค่า = ตอบทันที)
   */
  middleware(fn: (ctx: MiddlewareContext) => RouteResult | void, opts?: { priority?: number }): void;
  /** After the response was sent, in a queue (never delays it; 500 ms each). Scope "middleware". / หลังตอบคำขอ */
  afterResponse(fn: (info: ResponseInfo) => void): void;
  /**
   * HTML for <head> of every public page (50 ms). Without scope "head-scripts" only <meta>, <link>
   * (not stylesheets) and JSON-LD scripts are kept. / แทรก HTML ใน <head>
   */
  head(fn: (page: PageInfo) => string | null | void): void;
  /** HTML before </body> (50 ms); sanitised like post content unless scope "head-scripts". / แทรกก่อน </body> */
  bodyEnd(fn: (page: PageInfo) => string | null | void): void;
}

/** Modules of active JS plugins: `import { money } from 'money'` (also 'anasaki:money'). A plugin's library is bundled into your code; names in its code.exports run in the plugin (JSON arguments / results). */
declare module 'anasaki:*';

// ---------------------------------------------------------------- phase 7: joints for the official plugins
// (generic: any JS plugin or Site Code may use them; declaration merging)

interface SiteRequest {
  /** Client address (same rules as site.middleware: trusted proxies only). / IP ผู้เรียก */
  ip: string;
  /** Host name of the request (X-Forwarded-Host behind a trusted proxy). / ชื่อโดเมนของคำขอ */
  host: string;
}

interface SiteResponse {
  /**
   * `return response.page({ title, html })` — the HTML is shown inside the site's header and footer
   * (same as returning `{ page: { title, html }, status? }`). / แสดงในเลย์เอาต์ของเว็บ
   */
  page(page: { title?: string; html: string }, status?: number): { page: { title?: string; html: string }; status?: number };
}

/** A route / middleware result shown inside the site's layout. / ผลลัพธ์ที่แสดงในเลย์เอาต์ของเว็บ */
interface RoutePageResult {
  /** postId (CMS 2.0.8): the published post the page shows — page hooks get its id / type (ctx.page), so page.meta filters such as SEO apply its overrides. */
  page: { title?: string; html: string; postId?: number };
  /** 200-599, default 200. */
  status?: number;
  headers?: Record<string, string>;
}

interface ResponseInfo {
  /** Host name of the request. */
  host: string;
  /** A few request headers (lower-case): purpose, sec-purpose, x-moz, x-purpose, accept-language, dnt, sec-fetch-dest, sec-fetch-mode. */
  headers: Record<string, string>;
}

interface PageInfo {
  /** The site's tagline (Settings → General). / คำโปรยของเว็บ */
  siteTagline: string;
}

/** site.info(). / ข้อมูลของเว็บ */
interface SiteInfo {
  title: string;
  tagline: string;
  /** Site URL without a trailing slash ('' when unknown, e.g. in jobs without SITE_URL). */
  url: string;
  /** 'th' | 'en' … */
  language: string;
  timezone: string;
  /** The admin email (Settings → General) — Site Code and plugins with the "mail" scope only. */
  adminEmail?: string;
}

/** What site.adminNotice returns. / ประกาศในหน้าแอดมิน */
interface AdminNotice {
  level?: 'info' | 'warning' | 'danger' | 'success';
  text: I18nLabel;
  /** href: a site path ('/…', also admin paths) or an http(s) URL. */
  link?: { label: I18nLabel; href: string };
}

interface DashboardWidgetSpec {
  label: I18nLabel;
  icon?: string;
  /** Roles (default ['administrator']). */
  access?: string[];
  /** Initial iframe height in px (60-2000, default 220); pages using the admin UI kit then fit their content. */
  height?: number;
  /** HTML of the card (same document, kit and anasaki.call as admin pages). */
  render(ctx: AdminPageContext & { widget: string }): string | { html: string } | Promise<string | { html: string }>;
}

/** A designer block prop / setting field (the manifest "settings" format). */
interface BlockProp {
  key: string;
  type: 'text' | 'textarea' | 'number' | 'color' | 'boolean' | 'select' | 'url' | 'image';
  label?: I18nLabel;
  help?: I18nLabel;
  default?: string | number | boolean;
  /** select: fixed options, or filled by the block's options(ctx). */
  options?: { value: string; label?: I18nLabel }[];
  placeholder?: string;
  min?: number;
  max?: number;
  maxLength?: number;
}

interface BlockSpec {
  label: I18nLabel;
  icon?: string;
  props?: BlockProp[];
  /** Dynamic select options, resolved when the designer loads its blocks: { <prop key>: [{ value, label }] }. */
  options?(ctx: { locale: string }): Record<string, ({ value: string | number; label?: I18nLabel } | string)[]>;
  /**
   * Server-side HTML of the block on public pages and in the designer (200 ms). Sanitised (forms, inputs, data-*
   * kept; scripts, styles and on* removed) unless the plugin has the "head-scripts" scope; css is scoped under
   * .pb-<plugin>-<block>.
   */
  render(ctx: { site: Record<string, string>; post: Record<string, string> | null; item: Record<string, string> | null; path: string; user: SiteUser | null;
      /** Query string (first value per key) — only for blocks without `cache`. */ query?: Record<string, string> },
    props: Record<string, any>): string | { html: string; css?: string };
  /** A .js file of the package loaded with the block (e.g. 'assets/form.js'). */
  script?: string;
  /** Seconds to reuse a render per props and post (0-3600). */
  cache?: number;
}

interface Site {
  /**
   * A setting. In a plugin: the value saved on Plugins → <plugin> → Settings (manifest "settings"; booleans and
   * numbers typed, an empty text counts as unset), else Site code → Settings `<id>.<key>`, else the manifest
   * default. In Site Code: Site code → Settings. Without a key: every value as an object.
   * ค่าตั้งค่า (ปลั๊กอิน: หน้า ปลั๊กอิน → ตั้งค่า ก่อน แล้วจึง Site code → Settings แล้วจึงค่าเริ่มต้น)
   */
  settings(key: string): string | number | boolean | null;
  settings(): Record<string, string | number | boolean>;
  /**
   * Saves the calling plugin's manifest settings (partial; validated like the Settings form, audited). Only in
   * admin page code (render, an action the page calls, download) for a user with manage_options. Returns every value.
   * บันทึกการตั้งค่าของปลั๊กอินจากหน้าแอดมิน
   */
  saveSettings(values: Record<string, string | number | boolean>): Record<string, string | number | boolean>;
  /** Site title, tagline, URL, language, timezone (+ adminEmail with scope "mail"). / ข้อมูลของเว็บ */
  info(): SiteInfo;
  /** A plugin's own custom fields of a post (keys without the "<id>." prefix), null when the post does not exist. Scope "content-fields". */
  meta(postId: number): Record<string, any> | null;
  /**
   * Writes the plugin's own custom fields of a post (validated like the editor; null/'' removes a value). The
   * caller must be allowed to edit the post (or run as system: db.asSystem, the legacy import). Scope "content-fields".
   */
  setMeta(postId: number, values: Record<string, any>): Record<string, any>;
  /** A notice in the admin shell for the roles in access (default ['administrator']); return null for none (300 ms). Scope "admin-pages". */
  adminNotice(fn: (ctx: { user: SiteUser; locale: 'th' | 'en' }) => AdminNotice | null | void, opts?: { access?: string[] }): void;
  /** A card on the admin dashboard (sandboxed like admin pages). Scope "admin-pages". / การ์ดบนแดชบอร์ด */
  dashboardWidget(id: string, spec: DashboardWidgetSpec): void;
  /** A designer block rendered on the server (JS plugins; the node is {plugin, block, values}). / บล็อกดีไซเนอร์ที่เรนเดอร์ฝั่ง server */
  block(type: string, spec: BlockSpec): void;
  /**
   * Since 2.0.6: whether user (ctx.user; null → false) holds a capability — a core one or one of the plugin's own
   * ("<id>.<capability>", manifest "capabilities"). Feature-detect with `typeof site.can === 'function'`.
   * ตรวจสิทธิ์ของผู้ใช้ (ปลั๊กอินตรวจได้เฉพาะสิทธิ์ของตัวเอง)
   */
  can(user: SiteUser | null | undefined, cap: string): boolean;
}


interface QueryBuilder<T = Row> {
  /** COUNT(*) (or COUNT(column)) AS alias (default "count"); valid without the separate database. */
  selectCount(alias?: string, column?: Column<T>): QueryBuilder<any>;
  selectCountDistinct(column: Column<T>, alias?: string): QueryBuilder<any>;
  /** SUM(column) AS alias (a number, not a DECIMAL string). */
  selectSum(column: Column<T>, alias?: string): QueryBuilder<any>;
  selectAvg(column: Column<T>, alias?: string): QueryBuilder<any>;
  selectMin(column: Column<T>, alias?: string): QueryBuilder<any>;
  selectMax(column: Column<T>, alias?: string): QueryBuilder<any>;
}

interface ColumnBuilder {
  /** Case-sensitive comparison (COLLATE utf8mb4_bin): '/About' and '/about' differ. string/text/enum/uuid. */
  binary(): this;
  /** 'utf8mb4_bin' | 'utf8mb4_unicode_ci' (the default) | 'utf8mb4_general_ci'. */
  collation(name: 'utf8mb4_bin' | 'utf8mb4_unicode_ci' | 'utf8mb4_general_ci'): this;
}

// ---------------------------------------------------------------- phase 5: the browser side (types only)
// Not globals of your server code: these describe `window.anasaki` on public
// pages (Frontend code block, your own <script>) and in admin pages.
// ไม่ใช่ตัวแปรของโค้ดฝั่ง server — อธิบาย window.anasaki บนหน้าเว็บ

/** What `anasaki.call` resolves to. / ผลของ anasaki.call */
interface AnasakiCallResult<T = any> {
  /** HTTP 2xx and no field errors. */
  ok: boolean;
  /** The action's `data`. */
  data?: T;
  /** Per-field messages (the action's `errors`). */
  errors?: Record<string, string>;
  /** The action's message, or the error message. */
  message?: string;
  /** Same-site path the action asked to go to (not followed automatically). */
  redirect?: string;
  /** HTTP status. */
  status: number;
}

/**
 * `window.anasaki` on public pages (and `anasaki` inside a Frontend code block).
 * In admin pages (site.adminPage) only `call`, `download(query)` and `downloadUrl(query)` exist
 * (plus `page`, `locale`); the admin UI kit is `window.AK` there (docs/site-code.md).
 */
interface AnasakiClient {
  /** Runs a site.action (POST /_a/<name>, CSRF + same-origin); collections it refreshes are re-rendered. */
  call<T = any>(action: string, input?: Record<string, any>): Promise<AnasakiCallResult<T>>;
  /** GET a site.route path (same origin, cookies sent) → parsed JSON (text when not JSON); rejects on HTTP ≥ 400 (error.status, error.body). */
  get<T = any>(route: string, params?: Record<string, string | number | boolean>): Promise<T>;
  /** POST JSON to a site.route path (same origin, X-CSRF-Token sent) → parsed JSON; rejects on HTTP ≥ 400. */
  post<T = any>(route: string, body?: any): Promise<T>;
  /** Re-renders the collections of the page with this source ('events' = code:events, 'shop:orders', 'table:x') or node id. */
  refresh(collection: string): Promise<void>;
  /** Listens to 'refresh' / 'action' (DOM events anasaki:refresh / anasaki:action); returns a function that stops listening. */
  on(event: 'refresh' | 'action' | 'anasaki:refresh' | 'anasaki:action' | string, fn: (detail: any, event: Event) => void): () => void;
}
