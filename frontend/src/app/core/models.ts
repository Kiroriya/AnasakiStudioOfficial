import { DNode } from '../editor/design-types';

/** Built-in roles; custom roles are any other slug. */
export type Role = 'administrator' | 'editor' | 'author' | 'contributor' | 'subscriber' | (string & {});
/** Built-in types plus post types registered by Site Code (site.postType). */
export type PostType = 'post' | 'page' | 'template' | (string & {});
export const BUILTIN_POST_TYPES = ['post', 'page', 'template'];
/** 'scheduled' (CMS 2.1.0): waits for publishedAt, then the server publishes it. */
export type PostStatus = 'draft' | 'publish' | 'scheduled';

export interface User {
  id: number;
  email: string;
  name: string;
  role: Role;
  locale?: 'th' | 'en';
  /** Capabilities granted by the server (role + plugins). */
  caps?: string[];
  firstName?: string;
  lastName?: string;
  bio?: string;
  website?: string;
  avatar?: string;
  status?: 'active' | 'suspended';
  lastLoginAt?: string | null;
  postCount?: number;
  createdAt: string;
}

export interface UserList { items: User[]; total: number; counts: Record<string, number> }

export interface RoleInfo { slug: string; name: string; builtin: boolean; caps: string[]; userCount: number }
/**
 * A capability of the catalog (GET /roles). Plugin capabilities (CMS 2.0.6) have group "plugins", key
 * "<plugin>.<id>" and carry the plugin and the manifest's th/en label and description.
 */
export interface CapInfo {
  key: string;
  group: string;
  plugin?: string;
  pluginName?: string;
  pluginIcon?: string;
  label?: { th?: string; en?: string };
  description?: { th?: string; en?: string };
  default?: string[];
}
export interface RolesResponse { roles: RoleInfo[]; caps: CapInfo[]; builtin?: string[] }

/** GET /setup: `version` and `tokenRequired` (SETUP_TOKEN is set) only while the site still needs setup. */
export interface SetupStatus { needsSetup: boolean; product: string; version?: string; tokenRequired?: boolean }
/** GET /api/public/site: `version` only for a signed-in session; the admin path is never public. */
export interface PublicSite { siteTitle: string; tagline: string; siteIcon: string; siteLogo: string; product: string; version?: string; demo?: DemoStatus }

/** Demo mode (public playground): shared visitor account and periodic reset. */
export interface DemoStatus { enabled: boolean; email: string; password: string; everyMinutes: number; nextReset: string; baselineAt: string }

export interface Term {
  id: number;
  taxonomy: string;
  name: string;
  slug: string;
  count: number;
  /** Parent term id in a hierarchical taxonomy, 0 = top level (CMS 2.1.0). */
  parent?: number;
  description?: string;
}

/** "category" or a taxonomy registered with site.taxonomy (CMS 2.1.0). */
export interface Taxonomy {
  id: string;
  label: { th?: string; en?: string };
  singular: { th?: string; en?: string };
  hierarchical: boolean;
  postTypes: string[];
  slug: string;
  public: boolean;
  builtin: boolean;
  canManage: boolean;
  plugin?: string;
  pluginName?: string;
}

export interface Post {
  id: number;
  type: PostType;
  title: string;
  slug: string;
  status: PostStatus;
  excerpt: string;
  content: string;
  design?: DNode | null;
  featuredMediaId: number | null;
  featuredImage: string;
  authorId: number;
  authorName: string;
  categories: Term[];
  /** Terms of every taxonomy by taxonomy id (CMS 2.1.0). */
  terms?: Record<string, Term[]>;
  /** Shown first in post lists on the site (WordPress "sticky"). */
  pinned?: boolean;
  /** Position in the hand-made ("manual") order; 0 = not placed. */
  sortOrder?: number;
  createdAt: string;
  updatedAt: string;
  publishedAt: string | null;
  /** Custom field values (phase 4a; present when the type has fields). */
  meta?: Record<string, unknown>;
}

export interface PostInput {
  type?: PostType;
  title?: string;
  slug?: string;
  status?: PostStatus;
  excerpt?: string;
  content?: string;
  design?: DNode;
  featuredMediaId?: number | null;
  clearFeatured?: boolean;
  categoryIds?: number[];
  pinned?: boolean;
  /** Publish date (ISO): in the future the post is scheduled (CMS 2.1.0). */
  publishedAt?: string;
  /** Term ids per taxonomy; a taxonomy left out is unchanged (CMS 2.1.0). */
  terms?: Record<string, number[]>;
  /** Custom field values (site.postType fields / site.fields), phase 4a. */
  meta?: Record<string, unknown>;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
}

export interface Revision {
  id: number;
  postId: number;
  title: string;
  authorName: string;
  createdAt: string;
  /** Only when fetched one by one (post design or component node). */
  design?: DNode | null;
  content?: string;
}

export interface Media {
  id: number;
  filename: string;
  url: string;
  mime: string;
  size: number;
  width: number;
  height: number;
  alt: string;
  kind?: 'image' | 'video' | 'audio' | 'document' | 'archive' | 'other';
  ext?: string;
  downloads?: number;
  authorId: number;
  createdAt: string;
}

export interface MenuItem {
  label: string;
  url: string;
}

export interface Menu {
  id: number;
  name: string;
  items: MenuItem[];
  updatedAt?: string;
}

export interface DesignComponent {
  id: number;
  name: string;
  node: DNode;
  updatedAt?: string;
}

export type Settings = Record<string, string>;

export interface Stats {
  posts: number;
  pages: number;
  templates: number;
  media: number;
  components: number;
}

// ---------------------------------------------------------------- plugins

export interface PluginMenuItem { path: string; label: string; labelEn: string; icon: string; cap: string }

export interface PluginInfo {
  id: string;
  name: string;
  description: string;
  version: string;
  author: string;
  icon: string;
  active: boolean;
  menu: PluginMenuItem[];
  nameEn?: string;
  descriptionEn?: string;
  /** Has a generic settings screen at /p/:id. */
  hasSettings?: boolean;
  category?: string;
  /** Data sources plugins expose to the designer's collection block. */
  collections?: { source: string; label: string }[];
  /** "service" = an active service plugin (a package with a server program). */
  kind?: string;
}

/** The server-program part of a service plugin (admin only). */
export interface ServiceView {
  url: string; secret: string; public: string[]; events: string[];
  collections: { source: string; label: string }[];
  admin: { path: string; label: string; labelEn?: string; icon?: string; cap?: string }[];
  api: string[];
  /** "wasm" = the plugin's code runs inside the CMS (nothing to connect). */
  runtime?: string;
  hosts?: string[];
}
export interface ServiceHealth { ok: boolean; status: number; name: string; version: string; error: string; ms: number }

// ---------------------------------------------------------------- plugin platform

export interface I18nText { th: string; en: string }

/** Settings / block prop field (plugin manifest schema). */
export interface SchemaField {
  key: string;
  /** 'secret' (CMS 2.1.0, plugin settings): a password box; the value is never sent back. */
  type: 'text' | 'textarea' | 'number' | 'color' | 'boolean' | 'select' | 'url' | 'image' | 'date' | 'datetime' | 'email' | 'json' | 'secret'
    // CMS 2.1.0 custom fields: ids of posts / users / terms / media, rows of fields, HTML
    | 'relation' | 'user' | 'term' | 'file' | 'gallery' | 'repeater' | 'richtext';
  postTypes?: string[];
  multiple?: boolean;
  taxonomy?: string;
  /** repeater: each row's fields. */
  fields?: SchemaField[];
  label: I18nText | string;
  default?: unknown;
  options?: { value: string; label: I18nText | string }[];
  help?: I18nText | string;
  /** Custom fields of post types (validated by the server too). */
  required?: boolean;
  min?: number;
  max?: number;
  placeholder?: string;
}

export interface PluginBlock { type: string; name: I18nText | string; icon: string; props: SchemaField[] }

export interface InstalledPackage {
  id: string; name: string; version: string; description: string; author: string; icon: string; iconUrl?: string;
  category: string; permissions: string[]; active: boolean; source: 'upload' | 'store'; installedAt: string;
  settingsSchema: SchemaField[]; settings: Record<string, unknown>; blocks: PluginBlock[];
  /** Settings of type 'secret' that have a value (CMS 2.1.0). */
  secretsSet?: string[];
  service?: ServiceView;
  /** Set after an upgrade that asks for more than was approved: the package stays off until re-approved. */
  reconsent?: Reconsent;
  /** Phase 4b: 'js' = a JavaScript plugin run by the Site Code engine (older servers leave it out). */
  runtime?: string;
  /** Plugin ids this one needs → semver range (a map as in the manifest, or a resolved list). */
  requires?: PackageRequiresRaw;
  /** Installed plugins that require this one. */
  dependents?: string[];
  /** Phase 6: plugins this one works with when present (never blocks install) — same shapes as `requires`. */
  optionalRequires?: PackageRequiresRaw;
  /** JS plugins: the manifest `code` section. */
  code?: PackageCode;
  /** Why a JS plugin could not be activated (build/migration/visitor/route errors); it stays off. */
  activationError?: string | null;
  /** CMS 2.0.6: what the plugin lets roles do (ticked per role in Users → Roles). */
  capabilities?: PluginCapability[];
}

/** A capability a plugin declares in its manifest (CMS 2.0.6); key = "<plugin>.<id>" (installed packages). */
export interface PluginCapability {
  id: string;
  key?: string;
  label: { th?: string; en?: string } | string;
  description?: { th?: string; en?: string } | string | null;
  /** Roles granted the capability when it first appears. */
  default?: string[];
}

/** Phase 4b: what a JS plugin's code asks for (manifest `code`). */
export interface PackageCode {
  scopes?: string[]; hosts?: string[]; routes?: string[]; exports?: string[]; library?: string | null;
  hooks?: { filters?: string[]; events?: string[] };
}
/** One `requires` entry as the APIs may send it. */
export interface PackageRequireEntry {
  id: string; range: string;
  /** Installed version (when installed). */
  installed?: string | null;
  /** Latest compatible version published in the store (store detail). */
  version?: string | null;
  latest?: string | null;
  name?: string;
}
export type PackageRequiresRaw = Record<string, string> | PackageRequireEntry[];

export interface Reconsent {
  fromVersion: string; permissions?: string[]; api?: string[]; hosts?: string[];
  /** Phase 4b: code scopes the new version asks for beyond the approved ones. */
  scopes?: string[];
}

export interface BlockInfo { plugin: string; pluginName: string; type: string; name: I18nText | string; icon: string; props: SchemaField[] }

export interface StoreSummary {
  id: string; name: string; version: string; description: string; author: string; icon: string; iconUrl?: string;
  category: string; tags: string[]; downloads: number; views: number; rating: number; ratingCount: number;
  verified: boolean; official?: boolean; updatedAt: string; installed?: boolean; installedVersion?: string; updateAvailable?: boolean;
  /** Store publisher id; publisherChanged: it differs from the one the installed plugin is bound to (no update offered). */
  publisher?: string; publisherChanged?: boolean; installedPublisher?: string;
}
export interface StoreDetail extends StoreSummary {
  readme: string; homepage: string; license: string; permissions: string[]; screenshots: string[];
  versions: { version: string; size: number; minCore: string; createdAt: string; changelog?: string }[];
  /** Phase 4b (optional): JS plugins. */
  runtime?: string;
  requires?: PackageRequiresRaw;
  /** Phase 6: "works with" plugins (optional, never blocking). */
  optionalRequires?: PackageRequiresRaw;
  code?: PackageCode;
  /** Store 1.2.2: the capabilities the plugin declares (manifest `capabilities`). */
  capabilities?: PluginCapability[];
}
export interface StoreCategory { slug: string; name: string; nameEn: string; icon: string; count: number }
export interface StoreReview { stars: number; comment: string; author: string; createdAt: string }

/** Personal access token (GET /auth/tokens) — the secret itself is only returned once, on creation. */
export interface ApiToken {
  id: number;
  name: string;
  /** First characters of the token, shown so it can be recognised (e.g. "ast_Ab12"). */
  prefix: string;
  scopes: string[];
  lastUsedAt?: string | null;
  expiresAt?: string | null;
  createdAt: string;
}

/** POST /auth/tokens body; expiresInDays 0 = never. */
export interface ApiTokenCreate { name: string; scopes: string[]; expiresInDays: number; currentPassword?: string }
