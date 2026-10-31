import { Injectable, computed, effect, inject, signal, untracked } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeBindings, CodeFormatter, CodePreviewRequest, CodeViewAs } from '../core/code-models';
import { normalizeFormatters, suggestFormatterNames } from '../code/formatter-utils';
import { tr } from '../core/i18n';
import { BindCtx } from './design-css';
import { DNode, findNode } from './design-types';
import { EditorStore } from './editor-store';
import { EvalOptions, ExprCtx } from './expr';
import {
  SuggestSource, collectionPreviewRequest, docQueryKeys, docUsesCode, enclosingCollection, isCodeSource, matchPagePath, parseSource, previewVisitor, sourceFields,
} from './code-logic';
import { codeBlockAccess } from './codeblock';
import './messages-sitecode';

export interface PreviewState { loading: boolean; items: Record<string, unknown>[]; total: number; error?: string }

/** Items kept per preview request; more requests than this drop the oldest. */
const PREVIEW_LIMIT = 60;

/**
 * Site Code data of one editing session: what the code offers (GET
 * /code/bindings), collection / page previews (POST /code/preview), the
 * "view as" and "draft" switches and the "show hidden layers" toggle.
 * Provided next to EditorStore by the editor component.
 */
@Injectable()
export class CodeData {
  private api = inject(Api);
  private auth = inject(Auth);
  private store = inject(EditorStore);

  readonly bindings = signal<CodeBindings | null>(null);
  readonly bindingsError = signal<string | null>(null);
  /** Phase 6: GET /code/formatters — plugin formatters (`thai:baht`) and their enabled aliases for the autocomplete. */
  readonly formatterList = signal<CodeFormatter[]>([]);
  private readonly formatterNames = computed(() => suggestFormatterNames(this.bindings()?.formatters, this.formatterList()));
  /** Preview as an anonymous visitor, as the signed-in user, or as a site visitor in some groups (site.visitor). */
  readonly viewAs = signal<'anonymous' | 'me' | 'group'>('anonymous');
  /** Groups of the "Visitor in group…" preview. */
  readonly viewGroups = signal<string[]>([]);
  /** viewAs as sent to POST /code/preview (`{groups}` for a visitor in groups). */
  readonly previewViewAs = computed<CodeViewAs>(() => (this.viewAs() === 'group' ? { groups: this.viewGroups() } : (this.viewAs() as 'anonymous' | 'me')));
  /** `visitor` in canvas expressions (null unless previewing a visitor in groups). */
  readonly visitor = computed(() => previewVisitor(this.previewViewAs(), tr('sc.visitor.sample')));
  /** site.visitorGroups() of the code. */
  readonly visitorGroups = computed(() => this.bindings()?.visitorGroups ?? []);
  /** Preview the draft code (users with edit_scripts only). */
  readonly draft = signal(false);
  /** Layers hidden by their condition: dimmed with a badge (true) or hidden like on the site (false). */
  readonly showHidden = signal(true);
  readonly canDraft = computed(() => this.auth.can('edit_scripts'));
  readonly canManageAccess = computed(() => this.auth.can('manage_scripts'));
  /** Frontend code blocks: insert / edit (others see them locked). */
  readonly codeBlock = computed(() => codeBlockAccess(c => this.auth.can(c)));

  private readonly previews = signal<Record<string, PreviewState>>({});
  private order: string[] = [];
  private inflight = new Set<string>();
  /** site.page() data for the page being designed ({{page.*}}). */
  readonly pageData = signal<Record<string, unknown> | null>(null);
  readonly pageError = signal<string | null>(null);
  /** URL query of the preview (the canvas has no URL; empty unless set). */
  readonly query = signal<Record<string, string>>({});

  readonly uses = computed(() => docUsesCode(this.store.doc()));

  /** Month names follow the site language; dates use the site time zone. */
  readonly evalOpts = computed<EvalOptions>(() => {
    const s = this.store.settings();
    return { locale: s['language'] === 'en' ? 'en' : 'th', timeZone: s['timezone'] || 'Asia/Bangkok' };
  });

  /** Path of the page being designed ('/slug', '/' for home, the sample post for templates). */
  readonly pagePath = computed(() => {
    const p = this.store.post();
    if (!p) return '';
    if (p.type === 'template') return String(this.store.pageCtx()?.['url'] ?? '');
    return String(this.store.pageCtx()?.['url'] ?? '/' + this.store.slug());
  });

  constructor() {
    effect(() => {
      const draft = this.draft() && this.canDraft();
      untracked(() => void this.loadBindings(draft));
    });
    // page data (site.page) for this page, when a registered pattern matches
    effect(() => {
      const b = this.bindings(), path = this.pagePath(), viewAs = this.previewViewAs(), draft = this.draft() && this.canDraft();
      untracked(() => void this.loadPage(b, path, viewAs, draft));
    });
  }

  async loadBindings(draft: boolean) {
    try {
      const b = await this.api.codeBindings(draft);
      this.bindings.set({
        collections: b?.collections ?? [], actions: b?.actions ?? [], pages: b?.pages ?? [], formatters: b?.formatters ?? [], tables: b?.tables ?? [],
        postTypes: b?.postTypes ?? [], postFields: b?.postFields ?? {}, visitorGroups: (b?.visitorGroups ?? []).filter(g => typeof g === 'string'),
      });
      this.bindingsError.set(null);
      // older servers have no /code/formatters: the bindings list is enough
      this.api.codeFormatters().then(r => this.formatterList.set(normalizeFormatters(r)), () => this.formatterList.set([]));
    } catch (e) {
      // no Site Code (older server, no permission, demo): the designer works without it
      this.bindings.set(null);
      this.bindingsError.set(errorMessage(e));
    }
  }

  private async loadPage(b: CodeBindings | null, path: string, viewAs: CodeViewAs, draft: boolean) {
    if (!b || !path || !b.pages.some(p => matchPagePath(p.path, path))) { this.pageData.set(null); this.pageError.set(null); return; }
    try {
      const r = await this.api.codePreview({ kind: 'page', path, viewAs, ...(draft ? { draft: true } : {}) });
      if (r?.error) { this.pageData.set(null); this.pageError.set(r.error); return; }
      this.pageData.set(r?.data ?? {});
      this.pageError.set(null);
    } catch (e) {
      this.pageData.set(null);
      this.pageError.set(errorMessage(e));
    }
  }

  /** Page-level binding context (no item): params, conditions outside collections. */
  readonly pageCtx = computed<BindCtx>(() => ({
    site: { ...this.store.siteCtx(), language: this.store.settings()['language'] || 'th' },
    post: this.store.pageCtx(),
    page: this.pageData(),
    query: this.query(),
    visitor: this.visitor(),
    opts: this.evalOpts(),
  }));

  pageExprCtx(): ExprCtx {
    const c = this.pageCtx();
    return { site: c.site, post: c.post ?? null, page: c.page ?? {}, query: c.query ?? {}, visitor: c.visitor ?? null, item: null, $empty: false };
  }

  /** Custom field names of the page being designed (site.fields / site.postType) for `post.meta.` suggestions. */
  readonly postMetaKeys = computed(() => {
    const b = this.bindings(), p = this.store.post();
    if (!b || !p) return [];
    const type = p.type === 'template' ? 'post' : p.type;
    const own = b.postTypes?.find(t => t.type === type)?.fields ?? {};
    return [...new Set([...Object.keys(own), ...Object.keys(b.postFields?.[type] ?? {})])];
  });

  // ------------------------------------------------------------------ previews

  /** The preview request of a collection node (null for plugin sources). */
  requestFor(n: DNode): CodePreviewRequest | null {
    return collectionPreviewRequest(n, this.bindings(), this.pageExprCtx(), this.previewViewAs(), this.draft() && this.canDraft());
  }

  /** Request for the options of a select input (`optionsFrom`). */
  sourceRequest(source: string, perPage = 50): CodePreviewRequest | null {
    if (!isCodeSource(source)) return null;
    const r: CodePreviewRequest = { kind: 'collection', source, page: 1, perPage, params: {}, viewAs: this.previewViewAs() };
    if (this.draft() && this.canDraft()) r.draft = true;
    return r;
  }

  /** Preview state of a request (undefined until loaded; read from computeds). */
  preview(req: CodePreviewRequest | null): PreviewState | undefined {
    return req ? this.previews()[JSON.stringify(req)] : undefined;
  }

  /** Loads a preview once per distinct request (call from effects, never from computeds). */
  ensure(req: CodePreviewRequest | null) {
    if (!req) return;
    const key = JSON.stringify(req);
    if (this.inflight.has(key) || this.previews()[key]) return;
    this.inflight.add(key);
    this.set(key, { loading: true, items: [], total: 0 });
    this.api.codePreview(req).then(
      r => this.set(key, r?.error ? { loading: false, items: [], total: 0, error: r.error } : { loading: false, items: r?.items ?? [], total: r?.total ?? r?.items?.length ?? 0 }),
      e => this.set(key, { loading: false, items: [], total: 0, error: errorMessage(e) }),
    ).finally(() => this.inflight.delete(key));
  }

  /** Forget loaded previews (the Refresh button) — the canvas asks again. */
  refresh() {
    this.previews.set({});
    this.order = [];
    void this.loadBindings(this.draft() && this.canDraft());
  }

  private set(key: string, v: PreviewState) {
    this.order = [...this.order.filter(k => k !== key), key];
    this.previews.update(p => {
      const out = { ...p, [key]: v };
      while (this.order.length > PREVIEW_LIMIT) delete out[this.order.shift()!];
      return out;
    });
  }

  // ------------------------------------------------------------------ autocomplete / context

  /** Item fields of a collection node: declared fields, else the keys of its first preview item. */
  itemFields(collection: DNode | null): string[] {
    if (!collection) return [];
    const declared = Object.keys(sourceFields(collection.props?.source, this.bindings(), collection.props?.table));
    if (declared.length) return declared;
    const s = parseSource(collection.props?.source);
    if (s.kind === 'plugin') {
      const items = this.store.itemsFor(collection);
      return items.length ? Object.keys(items[0]) : ['title', 'image', 'url', 'text'];
    }
    return Object.keys(this.preview(this.requestFor(collection))?.items[0] ?? {});
  }

  /** First preview item of a collection (sample for nested keys and condition previews). */
  firstItem(collection: DNode | null): Record<string, unknown> | null {
    if (!collection) return null;
    if (isCodeSource(collection.props?.source)) return this.preview(this.requestFor(collection))?.items[0] ?? null;
    return this.store.itemsFor(collection)[0] ?? null;
  }

  /** What the binding autocomplete offers for a layer. */
  suggestSource(nodeId: string | null | undefined, opts: { pageOnly?: boolean } = {}): SuggestSource {
    const root = this.store.doc();
    const col = nodeId && !opts.pageOnly ? enclosingCollection(root, nodeId) : null;
    return {
      inCollection: !!col,
      itemFields: this.itemFields(col),
      item: this.firstItem(col),
      page: this.pageData(),
      queryKeys: [...new Set([...docQueryKeys(root), ...Object.keys(this.query()), 'q', 'page'])],
      formatters: this.formatterNames().names,
      formatterDetail: this.formatterNames().detail,
      visitorGroups: this.visitorGroups(),
      postMeta: this.postMetaKeys(),
    };
  }

  /** Expression context of a layer as its first rendered copy sees it (conditions panel). */
  nodeExprCtx(nodeId: string): ExprCtx {
    const root = this.store.doc();
    const col = enclosingCollection(root, nodeId);
    const c = this.pageExprCtx();
    if (!col) return c;
    const item = this.firstItem(col);
    return { ...c, item, $index: 0, $empty: !item };
  }

  collectionNode(id: string | undefined): DNode | null {
    return id ? findNode(this.store.doc(), id)?.node ?? null : null;
  }
}
