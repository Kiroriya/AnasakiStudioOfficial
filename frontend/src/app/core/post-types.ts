import { Injectable, computed, inject, signal } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { Api } from './api';
import { Auth } from './auth';
import { CodeAdminPage, CodeFieldSpec, CodePostType } from './code-models';
import { SchemaField } from './models';
import { PluginFieldGroup, asList, fieldsToSchema, groupFields, normalizeAdminPages, normalizePostTypes } from './post-type-utils';

/**
 * What the live Site Code adds to the admin (phase 4a): post types
 * (sidebar items, list/editor screens), custom fields of built-in types
 * (site.fields), declared visitor groups and admin pages (site.adminPage).
 *
 * Read from GET /code/bindings when the user may (edit_pages); otherwise
 * GET /post-types and GET /code/admin-pages. Any failure = nothing extra.
 */
@Injectable({ providedIn: 'root' })
export class PostTypes {
  private api = inject(Api);
  readonly list = signal<CodePostType[]>([]);
  /** site.fields('post' | 'page' | '<type>', …). */
  readonly extraFields = signal<Record<string, Record<string, CodeFieldSpec>>>({});
  readonly visitorGroups = signal<string[]>([]);
  readonly adminPages = signal<CodeAdminPage[]>([]);
  /** Types the signed-in user may edit (sidebar items). */
  readonly editable = computed(() => this.list().filter(p => p.canEdit !== false));
  private loading: Promise<void> | null = null;
  private loaded = false;

  load(): Promise<void> {
    this.loading ??= this.fetch().finally(() => { this.loaded = true; this.loading = null; });
    return this.loading;
  }

  async ensureLoaded() { if (!this.loaded) await this.load(); }

  get(type: string): CodePostType | undefined { return this.list().find(p => p.type === type); }

  /** Field declarations of a type: its own fields, plus site.fields(type, …). */
  fieldSpecs(type: string): Record<string, CodeFieldSpec> {
    return { ...(this.get(type)?.fields ?? {}), ...(this.extraFields()[type] ?? {}) };
  }

  schema(type: string): SchemaField[] { return fieldsToSchema(this.fieldSpecs(type)); }

  /** The type's fields split into the site's own and one group per JS plugin. */
  groups(type: string): { site: SchemaField[]; plugins: PluginFieldGroup[] } { return groupFields(this.fieldSpecs(type)); }

  private async fetch() {
    const b = await this.api.codeBindings().catch(() => null);
    let types: unknown = b?.postTypes;
    let fields: unknown = b?.postFields;
    let groups: unknown = b?.visitorGroups;
    if (!Array.isArray(types)) {
      const raw = await this.api.postTypes().catch(() => null);
      types = raw;
      if (raw && !Array.isArray(raw)) {
        const env = raw as Record<string, unknown>;
        fields ??= env['postFields'];
        groups ??= env['visitorGroups'];
      }
    }
    let pages: unknown = b?.adminPages;
    if (!Array.isArray(pages)) pages = await this.api.codeAdminPages().catch(() => []);
    this.list.set(normalizePostTypes(types));
    this.extraFields.set(fields && typeof fields === 'object' && !Array.isArray(fields) ? (fields as Record<string, Record<string, CodeFieldSpec>>) : {});
    this.visitorGroups.set(asList<unknown>(groups, ['groups']).filter((g): g is string => typeof g === 'string' && !!g));
    this.adminPages.set(normalizeAdminPages(pages));
  }
}

/** /content/:type/… — only types the live code registers and the user may edit. */
export const postTypeGuard: CanActivateFn = async route => {
  const auth = inject(Auth);
  const types = inject(PostTypes);
  const router = inject(Router);
  await auth.restore();
  await types.ensureLoaded();
  const pt = types.get(route.paramMap.get('type') ?? '');
  return (!!pt && pt.canEdit !== false) || router.parseUrl('/');
};
