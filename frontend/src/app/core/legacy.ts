import { Injectable, computed, inject, signal } from '@angular/core';
import { Api } from './api';

/**
 * Phase 7: the former built-in plugins (analytics, seo, forms, redirects, cookie-consent,
 * maintenance, custom-code) are official JS plugins from the Store. A site upgraded from a
 * version that had them gets them installed and its old data imported by a background task
 * (GET /api/v1/legacy-migrations); old tables/options stay until the admin cleans them up.
 */
export type LegacyStatus = 'pending' | 'installing' | 'importing' | 'done' | 'failed';

export interface LegacyItem {
  id: string;
  name: string;
  nameEn: string;
  icon: string;
  status: LegacyStatus;
  error: string;
  attempts: number;
  wasActive: boolean;
  hadData: boolean;
  imported: number;
  source: '' | 'store' | 'upload';
  version: string;
  approvedBy: string;
  approvedAt: string | null;
  approved: { permissions: string[]; scopes: string[]; hosts: string[] } | null;
  nextAttemptAt: string | null;
  updatedAt: string;
  doneAt: string | null;
  cleanedAt: string | null;
  cleanable: boolean;
}

export interface LegacySummary {
  /** Migrations not done yet ("N plugins are waiting to be moved"). */
  waiting: number;
  running: boolean;
  items: LegacyItem[];
}

export const EMPTY_LEGACY: LegacySummary = { waiting: 0, running: false, items: [] };

/** Items that still wait (not done), in the server's order. */
export function legacyWaiting(s: LegacySummary): LegacyItem[] {
  return s.items.filter(i => i.status !== 'done');
}

/** Done migrations whose old tables / options can be removed. */
export function legacyCleanable(s: LegacySummary): LegacyItem[] {
  return s.items.filter(i => i.status === 'done' && i.cleanable && !i.cleanedAt);
}

/** Poll while the task works (a run in progress or a row in an active step). */
export function legacyShouldPoll(s: LegacySummary): boolean {
  return s.running || s.items.some(i => i.status === 'installing' || i.status === 'importing' || (i.status === 'pending' && !i.attempts));
}

/** Plain-language reason of a failure: the Store could not be reached, or the message as is. */
export function legacyErrorKind(it: Pick<LegacyItem, 'error' | 'status'>): 'store' | 'other' | '' {
  if (it.status !== 'failed' || !it.error) return '';
  return /store unreachable|store unavailable|plugin store: status 5\d\d/i.test(it.error) ? 'store' : 'other';
}

/** Display name for the current language. */
export function legacyName(it: Pick<LegacyItem, 'name' | 'nameEn' | 'id'>, locale: string): string {
  return (locale === 'en' ? it.nameEn : it.name) || it.name || it.id;
}

/** The migration state shared by the admin banner and the Plugins page. */
@Injectable({ providedIn: 'root' })
export class LegacyMigrations {
  private api = inject(Api);
  readonly summary = signal<LegacySummary>(EMPTY_LEGACY);
  readonly waiting = computed(() => legacyWaiting(this.summary()));
  readonly cleanable = computed(() => legacyCleanable(this.summary()));
  readonly busy = signal(false);
  private timer: ReturnType<typeof setTimeout> | null = null;

  /** (Re)loads the state; keeps polling every 3 s while the task works. */
  async load(): Promise<void> {
    try {
      this.set(await this.api.legacyMigrations());
    } catch {
      this.set(EMPTY_LEGACY);
    }
  }

  private set(s: LegacySummary) {
    this.summary.set({ ...EMPTY_LEGACY, ...s, items: s.items ?? [] });
    if (this.timer) { clearTimeout(this.timer); this.timer = null; }
    if (legacyShouldPoll(this.summary())) this.timer = setTimeout(() => void this.load(), 3000);
  }

  /** Retry now (all waiting plugins, or one). */
  async retry(id?: string) {
    this.busy.set(true);
    try { this.set(await this.api.legacyRetry(id)); } finally { this.busy.set(false); }
  }

  /** Upload an official plugin zip (sites without Store access); the task picks it up. */
  async upload(file: File) {
    this.busy.set(true);
    try { this.set(await this.api.legacyUpload(file)); } finally { this.busy.set(false); }
  }

  /** Drop the old tables / options of a migrated plugin (after a confirmation in the UI). */
  async cleanup(id: string) {
    this.busy.set(true);
    try { this.set(await this.api.legacyCleanup(id)); } finally { this.busy.set(false); }
  }
}

// ---------------------------------------------------------------- setup wizard: recommended plugins

export interface RecommendedItem {
  id: string;
  /** As the Store sends it: a plain string or { th, en }. */
  name: string | { th?: string; en?: string };
  description: string | { th?: string; en?: string };
  icon: string;
  iconUrl?: string;
  /** Ticked by default (the Store's "recommendedChecked"). */
  checked: boolean;
  /** The Store's "recommendedOrder" (the list is already sorted). */
  order?: number;
  available: boolean;
  version: string;
  permissions: string[];
  scopes: string[];
  hosts: string[];
  installed: boolean;
}

export interface RecommendedList { storeAvailable: boolean; items: RecommendedItem[] }
export interface RecommendedResult { id: string; ok: boolean; version?: string; error?: string }
export interface RecommendedInstall { storeAvailable: boolean; results: RecommendedResult[] }

/** The ticked ids of the checklist at first (server defaults; already installed ones are left out). */
export function defaultSelection(items: RecommendedItem[]): Set<string> {
  return new Set(items.filter(i => i.checked && !i.installed).map(i => i.id));
}

/** Ids to install, in the list order (only offered, not yet installed ones). */
export function selectionIds(items: RecommendedItem[], selected: Set<string>): string[] {
  return items.filter(i => selected.has(i.id) && !i.installed).map(i => i.id);
}

/** What the wizard says after installing. */
export function installOutcome(r: RecommendedInstall): 'ok' | 'partial' | 'store' {
  if (!r.storeAvailable) return 'store';
  return r.results.every(x => x.ok) ? 'ok' : 'partial';
}

// ---------------------------------------------------------------- Export / import site (core tool)

export interface PackageImport {
  id: string;
  name: string;
  version: string;
  active: boolean;
  source: string;
  settings: Record<string, unknown>;
  status: 'installed' | 'missing';
  installedVersion?: string;
}

export interface ToolsImportResult {
  imported: Record<string, number>;
  packages: PackageImport[];
  siteCode: { added: string[]; skipped: number };
}
