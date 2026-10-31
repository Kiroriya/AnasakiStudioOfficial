import { ChangeDetectionStrategy, Component, computed, inject, output, signal } from '@angular/core';
import { TPipe, localized } from '../core/i18n';
import { BlockInfo, DesignComponent } from '../core/models';
import { DNode, NODE_ICON, NodeType, findNode, newId } from './design-types';
import { CodeData } from './code-data';
import { collectionChoices } from './code-logic';
import { categoryFilterPreset, createPagination, searchBoxPreset, submitButtonPreset } from './code-presets';
import { EditorStore } from './editor-store';
import { INTERACTIVE, InteractiveKind, createInteractive } from './prototype-presets';
import { NewComponent, NewComponentResult } from './new-component';
import { PRESETS, createNode, FRAME_KINDS, FrameKind, createFrame } from './presets';
import './messages-editor';

interface Tile { type: NodeType; hint: string }

/** Assets: sections, elements, dynamic blocks, plugin blocks and components — click or drag. */
@Component({
  selector: 'rea-insert-panel',
  imports: [TPipe, NewComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="insert">
      <p class="insert-tip" [innerHTML]="'ed.as.tip' | t"></p>

      <h4>{{ 'ed.as.frames' | t }} <small>{{ 'ed.as.framesHint' | t }}</small></h4>
      <div class="tile-grid frame-grid">
        @for (f of frameKinds; track f.kind) {
          <button class="tile tile-frame" draggable="true" (dragstart)="drag($event, frameFactory(f.kind))" (dragend)="store.assetDrag.set(null)"
                  (click)="store.insert(createFrame(f.kind))" [title]="('ed.frame.' + f.kind + '.d') | t">
            <span class="tile-icon">{{ f.icon }}</span><span>{{ ('ed.frame.' + f.kind + '.l') | t }}</span>
          </button>
        }
      </div>

      <h4>{{ 'ed.as.sections' | t }} <small>{{ 'ed.as.sectionsHint' | t }}</small></h4>
      <div class="preset-list">
        @for (p of presets(); track p.name) {
          <button class="preset" draggable="true" (dragstart)="drag($event, p.build, true)" (dragend)="store.assetDrag.set(null)" (click)="store.insertSection(p.build())">
            <span class="preset-icon">{{ p.icon }}</span>
            <span><b>{{ p.name | t }}</b><small>{{ p.desc | t }}</small></span>
          </button>
        }
      </div>

      <h4>{{ 'ic.section' | t }} <small>{{ 'ic.sectionHint' | t }}</small></h4>
      <div class="tile-grid">
        @for (d of decor; track d.label) {
          <button class="tile" draggable="true" (dragstart)="drag($event, decorFactory(d))" (dragend)="store.assetDrag.set(null)"
                  (click)="store.insert(decorFactory(d)())" [title]="d.hint | t">
            <span class="tile-icon">{{ d.icon }}</span><span>{{ d.label | t }}</span>
          </button>
        }
      </div>

      <h4>{{ 'pp.section' | t }} <small>{{ 'pp.sectionHint' | t }}</small></h4>
      <div class="tile-grid">
        @for (k of interactive; track k.kind) {
          <button class="tile" draggable="true" (dragstart)="drag($event, interactiveFactory(k.kind))" (dragend)="store.assetDrag.set(null)"
                  (click)="store.insertSection(interactiveFactory(k.kind)())">
            <span class="tile-icon">{{ k.icon }}</span><span>{{ 'pp.k.' + k.kind | t }}</span>
          </button>
        }
      </div>

      <h4>{{ 'ed.as.basic' | t }}</h4>
      <div class="tile-grid">
        @for (el of elements; track el.type) {
          <button class="tile" draggable="true" (dragstart)="drag($event, factory(el.type))" (dragend)="store.assetDrag.set(null)" (click)="add(el.type)" [title]="el.hint | t">
            <span class="tile-icon">{{ icon(el.type) }}</span><span>{{ 'ed.node.' + el.type | t }}</span>
          </button>
        }
      </div>

      <h4>{{ 'ed.as.data' | t }} <small>{{ 'ed.as.dataHint' | t }}</small></h4>
      <div class="tile-grid">
        @for (el of cmsElements; track el.type) {
          <button class="tile tile-cms" draggable="true" (dragstart)="drag($event, factory(el.type))" (dragend)="store.assetDrag.set(null)" (click)="add(el.type)" [title]="el.hint | t">
            <span class="tile-icon">{{ icon(el.type) }}</span><span>{{ 'ed.node.' + el.type | t }}</span>
          </button>
        }
      </div>

      <h4 id="ins-code-head">{{ 'sc.ins.section' | t }} <small>{{ 'sc.ins.sectionHint' | t }}</small></h4>
      <div class="tile-grid" id="ins-code">
        @if (hasCodeSources()) {
          <button class="tile tile-cms" id="ins-code-collection" draggable="true" (dragstart)="drag($event, factory('collection'))" (dragend)="store.assetDrag.set(null)" (click)="add('collection')" [title]="'sc.ins.collectionTip' | t">
            <span class="tile-icon">{{ icon('collection') }}</span><span>{{ 'ed.node.collection' | t }}</span>
          </button>
        }
        <button class="tile tile-cms" id="ins-pagination" draggable="true" (dragstart)="drag($event, paginationFactory)" (dragend)="store.assetDrag.set(null)" (click)="store.insert(paginationFactory())" [title]="'ed.help.pagination' | t">
          <span class="tile-icon">{{ icon('pagination') }}</span><span>{{ 'ed.node.pagination' | t }}</span>
        </button>
        <button class="tile tile-cms" id="ins-input" draggable="true" (dragstart)="drag($event, factory('input'))" (dragend)="store.assetDrag.set(null)" (click)="add('input')" [title]="'ed.help.input' | t">
          <span class="tile-icon">{{ icon('input') }}</span><span>{{ 'ed.node.input' | t }}</span>
        </button>
        @if (code.codeBlock().insert) {
          <button class="tile tile-cms" id="ins-codeblock" data-insert="codeblock" draggable="true" (dragstart)="drag($event, factory('codeblock'))" (dragend)="store.assetDrag.set(null)" (click)="add('codeblock')" [title]="'cb.insTip' | t">
            <span class="tile-icon">{{ icon('codeblock') }}</span><span>{{ 'ed.node.codeblock' | t }}</span>
          </button>
        }
      </div>
      <div class="preset-list" id="ins-code-presets">
        @for (p of codePresets; track p.id) {
          <button class="preset" [id]="'ins-preset-' + p.id" draggable="true" (dragstart)="drag($event, p.make)" (dragend)="store.assetDrag.set(null)" (click)="store.insert(p.make())">
            <span class="preset-icon">{{ p.icon }}</span>
            <span><b>{{ p.name | t }}</b><small>{{ p.desc | t }}</small></span>
          </button>
        }
      </div>

      @if (hasSources()) {
        <h4>{{ 'ed.as.pluginData' | t }} <small>{{ 'ed.as.pluginDataHint' | t }}</small></h4>
        <div class="tile-grid">
          <button class="tile tile-cms" draggable="true" (dragstart)="drag($event, factory('collection'))" (dragend)="store.assetDrag.set(null)" (click)="add('collection')" [title]="'ed.el.collection' | t">
            <span class="tile-icon">{{ icon('collection') }}</span><span>{{ 'ed.node.collection' | t }}</span>
          </button>
        </div>
      }

      @if (store.blocks().length) {
        <h4>{{ 'ed.as.plugins' | t }} <small>{{ 'ed.as.pluginsHint' | t }}</small></h4>
        <div class="tile-grid">
          @for (b of store.blocks(); track b.plugin + '/' + b.type) {
            <button class="tile tile-plugin" draggable="true" (dragstart)="drag($event, blockFactory(b))" (dragend)="store.assetDrag.set(null)" (click)="store.insert(blockFactory(b)())" [title]="b.pluginName">
              <span class="tile-icon">{{ b.icon || '🧩' }}</span><span>{{ name(b) }}</span>
            </button>
          }
        </div>
      }

      <h4>{{ 'ed.as.components' | t }} <small>{{ 'ed.as.componentsHint' | t }}</small></h4>
      <button class="btn btn-sm btn-block nc-open" (click)="newOpen.set(true)">＋ {{ 'cs.title' | t }}</button>
      <div class="preset-list">
        @for (c of store.placeableComponents(); track c.id) {
          <div class="comp-row">
            <button class="preset" draggable="true" (dragstart)="drag($event, instance(c))" (dragend)="store.assetDrag.set(null)" (click)="store.insertInstance(c)"><span class="preset-icon">◈</span> <b>{{ c.name }}</b></button>
            @if (c.id !== currentComponentId()) {
              <button class="icon-btn" [title]="'ed.as.editMain' | t" (click)="editComponent.emit(c.id)">✎</button>
            }
          </div>
        } @empty {
          <p class="hint">{{ 'ed.as.noComponents' | t }}</p>
        }
      </div>
      @if (newOpen()) {
        <rea-new-component [startGroup]="'header'" [placeHere]="true" (closed)="newOpen.set(false)" (created)="made($event)" />
      }
    </div>
  `,
})
export class InsertPanel {
  readonly store = inject(EditorStore);
  readonly code = inject(CodeData);
  readonly editComponent = output<number>();
  readonly newOpen = signal(false);

  /** A component from the gallery: placed on the page (or swapped in as the site header/footer). */
  async made(r: NewComponentResult) {
    this.newOpen.set(false);
    await this.store.adoptNewComponent(r.component, r.siteRole, r.replacedId);
  }
  readonly frameKinds = FRAME_KINDS;
  readonly decor: { type: 'icon' | 'shape'; icon: string; label: string; hint: string; name?: string }[] = [
    { type: 'icon', icon: '★', label: 'ic.tIcon', hint: 'ic.tIconT', name: 'star' },
    { type: 'shape', icon: '〰', label: 'ic.tWave', hint: 'ic.tDividerT', name: 'wave' },
    { type: 'shape', icon: '◠', label: 'ic.tCurve', hint: 'ic.tDividerT', name: 'curve' },
    { type: 'shape', icon: '◢', label: 'ic.tTilt', hint: 'ic.tDividerT', name: 'tilt' },
    { type: 'shape', icon: '●', label: 'ic.tBlob', hint: 'ic.tBlobT', name: 'blob1' },
    { type: 'shape', icon: '⠿', label: 'ic.tDots', hint: 'ic.tBlobT', name: 'dots' },
  ];
  decorFactory(d: { type: 'icon' | 'shape'; name?: string }) {
    return () => createNode(d.type, d.type === 'icon' ? { icon: d.name } : { shape: d.name });
  }
  readonly interactive = INTERACTIVE;
  interactiveFactory(k: InteractiveKind) { return () => createInteractive(k); }
  readonly createFrame = createFrame;
  frameFactory(kind: FrameKind) { return () => createFrame(kind); }

  readonly elements: Tile[] = [
    { type: 'text', hint: 'ed.el.text' }, { type: 'image', hint: 'ed.el.image' }, { type: 'button', hint: 'ed.el.button' },
    { type: 'rect', hint: 'ed.el.rect' }, { type: 'embed', hint: 'ed.el.embed' },
  ];
  readonly cmsElements: Tile[] = [
    { type: 'postList', hint: 'ed.el.postList' }, { type: 'menu', hint: 'ed.el.menu' }, { type: 'postContent', hint: 'ed.el.postContent' },
    { type: 'download', hint: 'ed.el.download' }, { type: 'breadcrumb', hint: 'ed.el.breadcrumb' },
  ];
  /** Collection block: only when an active plugin provides data. */
  readonly hasSources = computed(() => this.store.plugins.list().some(p => p.active && (p.collections?.length ?? 0) > 0));

  /** Collection block from Site Code: when the live code offers collections or tables. */
  readonly hasCodeSources = computed(() => {
    const b = this.code.bindings();
    return !!b && (b.collections.length > 0 || b.tables.length > 0);
  });

  /** New pagination bound to the selected collection (or the page's first one). */
  readonly paginationFactory = () => {
    const sel = this.store.selectedId();
    const n = sel ? findNode(this.store.doc(), sel)?.node : undefined;
    return createPagination(n?.type === 'collection' ? n.id : collectionChoices(this.store.doc())[0]?.id);
  };

  /** Quick presets: search box, category filter, submit button wired to an action. */
  readonly codePresets = [
    { id: 'search', icon: '🔍', name: 'sc.preset.search', desc: 'sc.preset.searchD', make: () => searchBoxPreset(this.store.doc(), this.code.bindings()) },
    { id: 'category', icon: '🏷', name: 'sc.preset.category', desc: 'sc.preset.categoryD', make: () => categoryFilterPreset(this.store.doc(), this.code.bindings()) },
    { id: 'submit', icon: '⚡', name: 'sc.preset.submit', desc: 'sc.preset.submitD', make: () => submitButtonPreset(this.code.bindings()) },
  ];

  /** Plugin sections only appear while their plugin is active. */
  readonly presets = computed(() => PRESETS.filter(p => !p.plugin || this.store.plugins.isActive(p.plugin)));

  currentComponentId() {
    return this.store.kind() === 'component' ? this.store.component()?.id : undefined;
  }

  icon(t: NodeType) { return NODE_ICON[t]; }

  name(b: BlockInfo) { return localized(b.name); }

  /** New pluginBlock node with the block's default prop values. */
  blockFactory(b: BlockInfo) {
    const values: Record<string, unknown> = {};
    for (const p of b.props ?? []) if (p.default !== undefined) values[p.key] = p.default;
    return () => createNode('pluginBlock', { plugin: b.plugin, block: b.type, blockName: this.name(b), values });
  }

  factory(type: NodeType) { return () => createNode(type, { menuId: this.store.menus()[0]?.id }); }

  instance(c: DesignComponent) {
    return (): DNode => ({
      id: newId(), type: 'instance', name: c.name,
      width: c.node.width.mode === 'fill' ? { mode: 'fill' } : c.node.width, height: { mode: 'hug' },
      props: { componentId: c.id },
    });
  }

  /** Start dragging an asset onto the canvas (dropped by EditorCanvas). */
  drag(e: DragEvent, make: () => DNode, section = false) {
    this.store.assetDrag.set({ make, section });
    e.dataTransfer?.setData('text/plain', 'rea-asset');
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'copy';
  }

  add(type: NodeType) {
    this.store.insert(createNode(type, { menuId: this.store.menus()[0]?.id }));
  }
}
