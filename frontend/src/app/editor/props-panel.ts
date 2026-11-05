import { VariantPicker } from './variants';
import { PrototypePanel } from './prototype';
import { MaskPanel } from './mask-panel';
import { PageSettings } from './page-settings';
import { LinkInput } from './link-input';
import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, output, signal, untracked } from '@angular/core';
import { Dialog } from '../core/dialog';
import { Media } from '../core/models';
import { MediaPicker } from '../shared/media-picker';
import { BINDINGS, BLEND_MODES, FONTS, THEME_COLOR_KEYS, Theme, ThemeColorKey, bind, tokenOf } from './design-css';
import { CARD_STYLES } from './presets';
import { I18n, TPipe, localized, tr } from '../core/i18n';
import { SchemaForm } from '../shared/schema-form';
import { BREAKPOINTS, DNode, Layout, Override, SizeMode, Style, findNode } from './design-types';
import './messages-editor';
import './messages-editor222';
import { AlignDir, EditorStore } from './editor-store';
import { Scrub } from './scrub';
import { StyleFx } from './style-fx';
import { VectorPanel } from './vector-panel';
import { LayoutGridEditor } from './layout-grid';
import { IconPicker } from './icon-picker';
import { StyleLibrary, TextStylePicker } from './text-styles';
import { BlockParts, InstanceParts, LayerPath, firstCard } from './asset-parts';
import { Shadow, formatShadows, parseColor, parseShadows, toCss, contrast } from './color-utils';
import { SelectValue } from '../shared/select-value';
import { BindingInput } from './binding-input';
import { CollectionPanel, ConditionsPanel, InputPanel, PaginationPanel } from './code-panels';
import { CodeBlockPanel } from './codeblock-panel';
import { PostTypes } from '../core/post-types';
import { CodePostType } from '../core/code-models';
import '../admin/messages-content-types';


/** Figma's names for layer types (Figma users recognise these). */
const FIGMA_TYPE: Record<string, string> = {
  frame: 'Frame', text: 'Text', rect: 'Rectangle', image: 'Image', button: 'Button', menu: 'Menu',
  postList: 'Post list', postContent: 'Post content', instance: 'Instance', embed: 'Embed',
  collection: 'Collection', pluginBlock: 'Plugin block', download: 'Download',
  breadcrumb: 'Breadcrumb', vector: 'Vector', pagination: 'Pagination', input: 'Input', codeblock: 'Code',
};

/** Human names of breakpoint overrides (chips in the tablet/mobile banner). */
const OVR_LABEL: Record<string, string> = {
  x: 'ed.ovr.pos', y: 'ed.ovr.pos', width: 'ed.ovr.width', height: 'ed.ovr.height', visible: 'ed.ovr.visible',
  'layout.mode': 'ed.ovr.direction', 'layout.gap': 'ed.ovr.gap', 'layout.padding': 'ed.ovr.padding', 'layout.columns': 'ed.ovr.columns',
  'layout.align': 'ed.ovr.align', 'layout.justify': 'ed.ovr.align', 'layout.wrap': 'ed.ovr.direction',
  'style.fontSize': 'ed.ovr.fontSize', 'style.fontWeight': 'ed.ovr.fontWeight', 'style.lineHeight': 'ed.ovr.lineHeight',
  'style.textAlign': 'ed.ovr.textAlign', 'style.color': 'ed.ovr.color', 'style.fill': 'ed.ovr.fill', 'style.radius': 'ed.ovr.radius',
  'style.maxWidth': 'ed.ovr.maxWidth', 'style.minHeight': 'ed.ovr.height', 'style.minHeightScreen': 'ed.ovr.screenH', 'style.minHeightOffset': 'ed.ovr.screenH', 'style.opacity': 'ed.ovr.opacity',
};

@Component({
  selector: 'rea-props-panel',
  imports: [SelectValue, MediaPicker, Scrub, TPipe, SchemaForm, StyleFx, IconPicker, LayerPath, InstanceParts, BlockParts, TextStylePicker, StyleLibrary, LayoutGridEditor, VariantPicker, VectorPanel, PrototypePanel, MaskPanel, PageSettings, LinkInput, BindingInput, CollectionPanel, PaginationPanel, InputPanel, ConditionsPanel, CodeBlockPanel],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="pp-tabs">
      <button [class.on]="tab() === 'design'" (click)="pickTab('design')" [title]="'ed.pp.designTip' | t">{{ 'ed.pp.tabDesign' | t }}</button>
      <button [class.on]="tab() === 'content'" (click)="pickTab('content')" [title]="'ed.pp.contentTip' | t">{{ 'ed.pp.tabContent' | t }}</button>
      <button [class.on]="tab() === 'prototype'" (click)="pickTab('prototype')" [title]="'pr.tabTip' | t">{{ 'pr.tab' | t }}</button>
    </div>

    @if (sel(); as s) {
      @let n = s.node;
      @let e = eff()!;
      @if (count() === 1) { <rea-layer-path [node]="n" /> }

      @if (tab() === 'design') {
        <!-- alignment row (Figma: top of the Design panel) -->
        <div class="fg-align">
          @for (a of alignBtns; track a.dir) {
            <button [title]="(a.title | t) + ' (' + a.keys + ')'" [attr.aria-label]="(a.title | t) + ' (' + a.keys + ')'" [disabled]="!s.parent" (click)="store.align(a.dir)">{{ a.icon }}</button>
          }
          <span class="sep"></span>
          <button [title]="('ed.cmd.distH' | t) + ' (Alt+Shift+H)'" [attr.aria-label]="('ed.cmd.distH' | t) + ' (Alt+Shift+H)'" [disabled]="count() < 3" (click)="store.distribute('h')">⫴</button>
          <button [title]="('ed.cmd.distV' | t) + ' (Alt+Shift+V)'" [attr.aria-label]="('ed.cmd.distV' | t) + ' (Alt+Shift+V)'" [disabled]="count() < 3" (click)="store.distribute('v')">☰</button>
        </div>

        @if (store.bp() !== 'desktop') {
          <div class="bp-banner">
            <b>{{ bpIcon() }} {{ 'ed.pp.bpBanner' | t: { bp: bpLabel() } }}</b>
            @if (overrides().length) {
              <span class="bp-sub">{{ 'ed.pp.bpChanged' | t }}</span>
              <div class="ovr-chips">
                @for (o of overrides(); track o.id) {
                  <button class="ovr-chip" [title]="'ed.pp.bpResetOne' | t" [attr.aria-label]="'ed.pp.bpResetOne' | t" (click)="store.resetOverride(n.id, o.group, o.key)">{{ o.label }} ✕</button>
                }
              </div>
              <button class="link" (click)="store.resetOverrides(n.id)">↺ {{ 'ed.pp.bpResetAll' | t }}</button>
            } @else {
              <span class="bp-sub">{{ 'ed.pp.bpSame' | t }}</span>
            }
          </div>
        } @else if (elsewhere(); as where) {
          <div class="bp-note">📱 {{ 'ed.pp.bpElsewhere' | t: { bp: where.label } }}
            <button class="link" (click)="store.bp.set(where.bp)">{{ 'ed.pp.bpGo' | t }}</button></div>
        }

        <div class="fg-type">
          <span class="fg-type-name">{{ count() > 1 ? ('ed.pp2.nLayers' | t: { n: count() }) : figmaType(n) }}</span>
          @if (n.type === 'instance') { <span class="fg-chip inst">◇ {{ componentName(n) }}</span> }
        </div>

        <!-- Position -->
        <section class="fg-sec">
          <h5>{{ 'ed.pp2.position' | t }}</h5>
          @if (pMode() === 'none') {
            <div class="fg-row2">
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'x')"><span [reaScrub]="e.x ?? 0" (scrub)="store.setPos(ids(), $event, null)">X</span>
                <input type="number" [value]="e.x ?? 0" (change)="store.setPos(ids(), num($event) ?? 0, null)" /></label>
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'x')"><span [reaScrub]="e.y ?? 0" (scrub)="store.setPos(ids(), null, $event)">Y</span>
                <input type="number" [value]="e.y ?? 0" (change)="store.setPos(ids(), null, num($event) ?? 0)" /></label>
            </div>
            <div class="fg-anchor" [class.ovr]="store.isOverridden(n, 'style', 'anchorX')" [title]="'ed.pp.anchorHint' | t">
              <span>{{ 'ed.pp.anchor' | t }}</span>
              <div class="fg-seg">
                @for (a of anchors; track a.id) {
                  <button [class.on]="(e.style?.anchorX ?? 'left') === a.id" [title]="('ed.pp.anchor.' + a.id) | t" [attr.aria-label]="('ed.pp.anchor.' + a.id) | t" (click)="store.setAnchor(ids(), a.id)">{{ a.icon }}</button>
                }
              </div>
            </div>
          }
          <div class="fg-row2">
            <label class="fg-field" [title]="'ed.pp.rotation' | t"><span [reaScrub]="e.style?.rotation ?? 0" (scrub)="style({ rotation: $event || undefined })">↻</span>
              <input [attr.aria-label]="'ed.pp.rotation' | t" type="number" [value]="e.style?.rotation ?? 0" (change)="style({ rotation: num($event) || undefined })" /></label>
            @if (s.parent && parentAuto()) {
              <button class="fg-toggle" [class.on]="!!n.absolute" (click)="store.setAbsolute(n.id, !n.absolute)"
                      [title]="'ed.pp.absolute' | t">⌖ {{ 'ed.pp2.absolute' | t }}</button>
            }
            @if (isFloating(n)) { <p class="hint warn-float">⚠ {{ 'fl.one' | t }} <button class="fg-link" (click)="store.unfloat([n.id])">{{ 'fl.fixOne' | t }}</button></p> }
            @if ((s.parent && parentAuto() && !n.absolute) || (!s.parent && store.kind() === 'component')) {
              <button class="fg-toggle" [class.on]="!!e.style?.sticky" (click)="style({ sticky: !e.style?.sticky || undefined })"
                      [title]="'cs.stickyHint' | t">📌 {{ 'cs.sticky' | t }}</button>
            }
          </div>
        </section>

        <!-- Layout: W/H with Fixed / Hug / Fill -->
        <section class="fg-sec">
          <h5>{{ 'ed.pp2.layout' | t }}</h5>
          <div class="fg-row2">
            @for (axis of axes; track axis) {
              <div class="fg-size" [class.ovr]="store.isOverridden(n, axis)">
                <label class="fg-field">
                  <span [reaScrub]="sizeVal(axis)" [minValue]="1" (scrub)="store.setSize(ids(), axis, { mode: 'fixed', value: $event })">{{ axis === 'width' ? 'W' : 'H' }}</span>
                  <input type="number" min="1" [value]="sizeVal(axis)" (change)="store.setSize(ids(), axis, { mode: 'fixed', value: num($event) ?? 1 })" />
                </label>
                <select class="fg-mode" [value]="e[axis].mode" (change)="sizeMode(axis, $any(val($event)))" [title]="(axis === 'width' ? 'ed.pp2.resizeH' : 'ed.pp2.resizeV') | t">
                  <option value="fixed">{{ (axis === 'width' ? 'ed.pp2.fixedW' : 'ed.pp2.fixedH') | t }}</option>
                  <option value="hug">{{ 'ed.pp2.hug' | t }}</option>
                  <option value="fill">{{ 'ed.pp2.fill' | t }}</option>
                </select>
              </div>
            }
          </div>
          <div class="fg-row2">
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'maxWidth')" [title]="'ed.pp.maxW' | t">
              <span>Max W</span><input [attr.aria-label]="'ed.pp.maxW' | t" type="number" min="0" [value]="e.style?.maxWidth ?? ''" placeholder="—" (change)="style({ maxWidth: num($event) })" /></label>
            @if (e.style?.minHeightScreen) {
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'minHeightOffset')" [title]="'ed.pp.screenOffset' | t">
                <span>{{ 'ed.pp.screenMinus' | t }}</span><input type="number" min="0" id="pp-screen-offset" [attr.aria-label]="'ed.pp.screenOffset' | t" [value]="e.style.minHeightOffset || ''" placeholder="0" (change)="style({ minHeightOffset: num($event) || undefined })" /><em>px</em></label>
            } @else {
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'minHeight')" [title]="'ed.pp.minH' | t">
                <span>Min H</span><input type="number" min="0" [attr.aria-label]="'ed.pp.minH' | t" [value]="e.style?.minHeight || ''" placeholder="—" (change)="style({ minHeight: num($event) || 0 })" /></label>
            }
          </div>
          <!-- 2.2.2: min W / max H, aspect ratio (Figma: min/max sizes, lock ratio) -->
          <div class="fg-row2">
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'minWidth')" [title]="'ed.pp.minW' | t">
              <span>Min W</span><input id="pp-min-w" [attr.aria-label]="'ed.pp.minW' | t" type="number" min="0" [value]="e.style?.minWidth || ''" placeholder="—" (change)="style({ minWidth: num($event) || undefined })" /></label>
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'maxHeight')" [title]="'ed.pp.maxH' | t">
              <span>Max H</span><input id="pp-max-h" [attr.aria-label]="'ed.pp.maxH' | t" type="number" min="0" [value]="e.style?.maxHeight || ''" placeholder="—" (change)="style({ maxHeight: num($event) || undefined })" /></label>
          </div>
          @if (n.type === 'frame' || n.type === 'image' || n.type === 'rect' || n.type === 'embed') {
            <label class="fg-field pp-aspect" [class.ovr]="store.isOverridden(n, 'style', 'aspect')" [title]="'ed.pp.aspectTip' | t">
              <span>⧉</span>
              <select id="pp-aspect" [attr.aria-label]="'ed.pp.aspect' | t" [value]="e.style?.aspect ?? ''" (change)="style({ aspect: val($event) || undefined })">
                <option value="">{{ 'ed.pp.aspectFree' | t }}</option>
                @for (a of aspects; track a) { <option [value]="a">{{ a.replace('/', ':') }}</option> }
              </select></label>
          }
          <label class="fg-check pp-screen" [class.ovr]="store.isOverridden(n, 'style', 'minHeightScreen')" [title]="'ed.pp.screenTip' | t">
            <input type="checkbox" id="pp-screen-h" [checked]="!!e.style?.minHeightScreen" (change)="style({ minHeightScreen: checked($event) || (store.bp() === 'desktop' ? undefined : false) })" /> {{ 'ed.pp.screenH' | t }}
          </label>
          <div class="fg-row2">
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'radius')" [title]="'ed.pp.radius' | t">
              <span [reaScrub]="e.style?.radius ?? 0" [minValue]="0" (scrub)="style({ radius: $event || undefined })">◜</span>
              <input [attr.aria-label]="'ed.pp.radius' | t" type="number" min="0" [value]="e.style?.radius ?? 0" (change)="style({ radius: num($event) || undefined })" /></label>
          </div>
          @if (n.type === 'frame' || n.type === 'postList') {
            <label class="fg-check"><input type="checkbox" [checked]="!!e.style?.clip" (change)="style({ clip: checked($event) || undefined })" /> {{ 'ed.pp2.clip' | t }}</label>
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'scroll')" [title]="'ed.pp.scrollTip' | t">
              <span>⇆</span>
              <select id="pp-scroll" [attr.aria-label]="'ed.pp.scroll' | t" [value]="e.style?.scroll ?? ''" (change)="style({ scroll: $any(val($event)) || undefined })">
                <option value="">{{ 'ed.pp.scrollNone' | t }}</option>
                <option value="x">{{ 'ed.pp.scrollX' | t }}</option>
                <option value="y">{{ 'ed.pp.scrollY' | t }}</option>
                <option value="both">{{ 'ed.pp.scrollBoth' | t }}</option>
              </select></label>
          }
        </section>

        <!-- Auto layout -->
        @if (hasLayout(n)) {
          <section class="fg-sec">
            <h5>
              {{ (padOnly(n) ? 'ed.pp2.padding' : 'ed.pp2.autoLayout') | t }}
              @if (!padOnly(n)) {
                @if (isAuto()) { <button class="fg-add" [title]="('ed.cmd.removeAutoLayout' | t) + ' (Alt+Shift+A)'" [attr.aria-label]="('ed.cmd.removeAutoLayout' | t) + ' (Alt+Shift+A)'" (click)="store.setLayoutMode(n.id, 'none')">−</button> }
                @else { <button class="fg-add" [title]="('ed.cmd.autoLayout' | t) + ' (Shift+A)'" [attr.aria-label]="('ed.cmd.autoLayout' | t) + ' (Shift+A)'" (click)="store.setLayoutMode(n.id, 'vertical')">+</button> }
              }
            </h5>
            @if (isAuto()) {
              <div class="fg-al">
                <div class="fg-al-left">
                  <div class="fg-seg" [class.ovr]="store.isOverridden(n, 'layout', 'mode')">
                    <button [class.on]="e.layout?.mode === 'vertical'" [title]="'ed.pp2.vertical' | t" [attr.aria-label]="'ed.pp2.vertical' | t" (click)="store.setLayoutMode(n.id, 'vertical')">↓</button>
                    <button [class.on]="e.layout?.mode === 'horizontal' && !e.layout.wrap" [title]="'ed.pp2.horizontal' | t" [attr.aria-label]="'ed.pp2.horizontal' | t" (click)="layout({ mode: 'horizontal', wrap: undefined })">→</button>
                    <button [class.on]="e.layout?.mode === 'horizontal' && !!e.layout.wrap" [title]="'ed.pp2.wrap' | t" [attr.aria-label]="'ed.pp2.wrap' | t" (click)="layout({ mode: 'horizontal', wrap: true })">↵</button>
                  </div>
                  <div class="fg-row2 tight">
                    <label class="fg-field" [class.ovr]="store.isOverridden(n, 'layout', 'gap')" [title]="'ed.pp.gap' | t">
                      <span [reaScrub]="e.layout?.gap ?? 0" [minValue]="0" (scrub)="layout({ gap: $event })">⇕</span>
                      @if (e.layout?.justify === 'between') { <input [attr.aria-label]="'ed.pp.gap' | t" [value]="'ed.pp2.auto' | t" readonly (click)="layout({ justify: undefined })" [title]="'ed.pp.gapAuto' | t" /> }
                      @else { <input [attr.aria-label]="'ed.pp.gap' | t" type="number" min="0" [value]="e.layout?.gap ?? 0" (change)="layout({ gap: num($event) })" /> }
                    </label>
                    <button class="fg-toggle" [class.on]="e.layout?.justify === 'between'" (click)="layout({ justify: e.layout?.justify === 'between' ? undefined : 'between' })" [title]="'ed.pp2.autoSpacing' | t">{{ 'ed.pp2.auto' | t }}</button>
                  </div>
                </div>
                <div class="fg-grid" [title]="'ed.pp2.alignment' | t">
                  @for (r of [0, 1, 2]; track r) {
                    @for (c of [0, 1, 2]; track c) {
                      <button [class.on]="gridOn(r, c)" [attr.aria-pressed]="gridOn(r, c)" [attr.aria-label]="gridLabel(r, c)" [title]="gridLabel(r, c)" (click)="gridSet(r, c)"><i></i></button>
                    }
                  }
                </div>
              </div>
            }
            @if (e.layout?.mode === 'horizontal' && !padOnly(n)) {
              <label class="fg-cols" [class.ovr]="store.isOverridden(n, 'layout', 'columns')" [title]="'ed.pp.columnsTip' | t">
                <span>▦ {{ 'ed.pp.columns' | t }}</span>
                <select [value]="'' + (e.layout.columns ?? '')" (change)="layout({ columns: num($event) })">
                  <option value="">{{ 'ed.pp.columnsAuto' | t }}</option>
                  @for (c of [1, 2, 3, 4, 5, 6]; track c) { <option [value]="'' + c">{{ c }}</option> }
                </select>
              </label>
            }
            <div class="fg-row2">
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'layout', 'padding')" [title]="'ed.pp.padH' | t">
                <span [reaScrub]="pad()[3]" [minValue]="0" (scrub)="setPad([null, $event, null, $event])">⇿</span>
                <input [attr.aria-label]="'ed.pp.padH' | t" type="number" min="0" [value]="pad()[1] === pad()[3] ? pad()[3] : ''" [placeholder]="pad()[1] === pad()[3] ? '' : ('ed.pp2.mixed' | t)" (change)="setPad([null, num($event) ?? 0, null, num($event) ?? 0])" /></label>
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'layout', 'padding')" [title]="'ed.pp.padV' | t">
                <span [reaScrub]="pad()[0]" [minValue]="0" (scrub)="setPad([$event, null, $event, null])">⇳</span>
                <input [attr.aria-label]="'ed.pp.padV' | t" type="number" min="0" [value]="pad()[0] === pad()[2] ? pad()[0] : ''" [placeholder]="pad()[0] === pad()[2] ? '' : ('ed.pp2.mixed' | t)" (change)="setPad([num($event) ?? 0, null, num($event) ?? 0, null])" /></label>
            </div>
            @if (isAuto()) { <p class="hint">{{ 'ed.pp.spacingTip' | t }}</p> }
            <button class="fg-link" (click)="padOpen.set(!padOpen())">{{ padOpen() ? '▾' : '▸' }} {{ 'ed.pp2.padEach' | t }}</button>
            @if (padOpen()) {
              <div class="fg-row4">
                @for (side of [0, 1, 2, 3]; track side) {
                  <label class="fg-field" [title]="sides[side] | t"><span>{{ sideIcons[side] }}</span>
                    <input [attr.aria-label]="sides[side] | t" type="number" min="0" [value]="pad()[side]" (change)="setPadSide(side, num($event) ?? 0)" /></label>
                }
              </div>
            }
          </section>
        }

        @if (!hasLayout(n) || count() > 1) {
          <section class="fg-sec">
            <h5>{{ 'ed.pp2.autoLayout' | t }} <button class="fg-add" [title]="('ed.cmd.autoLayout' | t) + ' (Shift+A)'" [attr.aria-label]="('ed.cmd.autoLayout' | t) + ' (Shift+A)'" (click)="store.addAutoLayout()">+</button></h5>
            <p class="hint">{{ (count() > 1 ? 'ed.pp.wrapMany' : 'ed.pp.wrapOne') | t }}</p>
            <div class="fg-row2">
              <button class="fg-toggle" (click)="wrapAuto('vertical')" [title]="'ed.pp.down' | t">↓ {{ 'ed.pp2.verticalShort' | t }}</button>
              <button class="fg-toggle" (click)="wrapAuto('horizontal')" [title]="'ed.pp.right' | t">→ {{ 'ed.pp2.horizontalShort' | t }}</button>
            </div>
          </section>
        }

        <!-- Layer -->
        <section class="fg-sec">
          <h5>{{ 'ed.pp2.layer' | t }}</h5>
          <div class="fg-row2">
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'opacity')" [title]="'ed.pp.opacity' | t">
              <span [reaScrub]="opacityPct()" [minValue]="0" (scrub)="setOpacity($event)">◐</span>
              <input [attr.aria-label]="'ed.pp.opacity' | t" type="number" min="0" max="100" [value]="opacityPct()" (change)="setOpacity(num($event) ?? 100)" /><em>%</em></label>
            <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'blend')" [title]="'ed.pp.blend' | t">
              <span>◑</span>
              <select id="pp-blend" [attr.aria-label]="'ed.pp.blend' | t" [value]="e.style?.blend ?? ''" (change)="style({ blend: val($event) || undefined })">
                <option value="">{{ 'ed.pp.blendNormal' | t }}</option>
                @for (b of blends; track b) { <option [value]="b">{{ b }}</option> }
              </select></label>
            @if (s.parent) {
              <button class="fg-toggle" [class.on]="e.visible === false" (click)="store.toggleVisible()" [title]="'ed.pp.toggleVis' | t: { bp: bpLabel() }">
                {{ e.visible === false ? '◌ ' + ('ed.pp2.hidden' | t) : '👁 ' + ('ed.pp2.visible' | t) }}</button>
            }
          </div>
        </section>

        <!-- Fill -->
        <section class="fg-sec">
          <h5>{{ 'ed.pp2.fillSec' | t }}
            @if (!e.style?.fill) { <button class="fg-add" [title]="'ed.pp2.addFill' | t" [attr.aria-label]="'ed.pp2.addFill' | t" (click)="style({ fill: '#d9d9d9' })">+</button> }
          </h5>
          @if (e.style?.fill; as f) {
            <div class="fg-paint" [class.ovr]="store.isOverridden(n, 'style', 'fill')">
              @if (tokenOf(f); as k) {
                <span class="fg-token"><i [style.background]="f"></i>{{ tokenName(k) }}</span>
                <button class="fg-link" [title]="'ed.theme.detachTip' | t" (click)="style({ fill: themeColor(k) })">{{ 'ed.theme.detach' | t }}</button>
              } @else if (parsed(f); as c) {
                <input type="color" [value]="c.hex" (input)="liveColor('fill', $event, c.a)" (change)="commitColor()" (blur)="commitColor()" />
                <input class="fg-hex" [value]="c.hex.slice(1).toUpperCase()" (change)="style({ fill: toCss('#' + val($event), c.a) })" />
                <label class="fg-pct"><input type="number" min="0" max="100" [value]="round(c.a * 100)" (change)="style({ fill: toCss(c.hex, (num($event) ?? 100) / 100) })" />%</label>
              } @else {
                <span class="fg-swatch" [style.background]="f"></span>
                <input class="fg-hex wide" [value]="f" (change)="style({ fill: val($event) })" [title]="'ed.pp2.gradient' | t" />
              }
              <button class="fg-rm" [title]="'ed.pp2.removeFill' | t" [attr.aria-label]="'ed.pp2.removeFill' | t" (click)="style({ fill: undefined })">−</button>
            </div>
          }
          <div class="swatches">
            @for (k of themeKeys; track k) { <button class="tok" [style.background]="'var(--c-' + k + ')'" [title]="('ed.theme.' + k) | t" [attr.aria-label]="('ed.theme.' + k) | t" (click)="style({ fill: 'var(--c-' + k + ')' })"></button> }
            @for (p of theme().palette; track p.id) { <button class="tok" [style.background]="p.value" [title]="p.name" [attr.aria-label]="p.name" (click)="style({ fill: 'var(--c-' + p.id + ')' })"></button> }
            <span class="sw-sep"></span>
            @for (c of swatches; track c) { <button [style.background]="c" [title]="c" [attr.aria-label]="c" (click)="style({ fill: c })"></button> }
          </div>
          @if (n.type === 'image') {
            <div class="fg-image">
              @if (isStatic(n.props?.src)) { <img [src]="n.props!.src" alt="" /> } @else { <span>{{ n.props?.src ? ('ed.pp.boundImage' | t) : ('ed.pp.noImage' | t) }}</span> }
              <button class="btn btn-sm" (click)="pickFor.set('src')">{{ 'ed.pp.chooseImage' | t }}</button>
            </div>
          } @else {
            <div class="fg-row2">
              <button class="fg-link" (click)="pickFor.set('bg')">+ {{ 'ed.pp2.imageFill' | t }}</button>
              @if (e.style?.backgroundImage) { <button class="fg-link" (click)="style({ backgroundImage: undefined })">{{ 'ed.pp2.removeImage' | t }}</button> }
            </div>
          }
        </section>

        <!-- Stroke -->
        <section class="fg-sec">
          <h5>{{ 'ed.pp2.stroke' | t }}
            @if (!e.style?.stroke) { <button class="fg-add" [title]="'ed.pp2.addStroke' | t" [attr.aria-label]="'ed.pp2.addStroke' | t" (click)="style({ stroke: '#000000', strokeWidth: 1 })">+</button> }
          </h5>
          @if (e.style?.stroke; as st) {
            <div class="fg-paint">
              @if (tokenOf(st); as k) {
                <span class="fg-token"><i [style.background]="st"></i>{{ tokenName(k) }}</span>
              } @else if (parsed(st); as c) {
                <input type="color" [value]="c.hex" (input)="liveColor('stroke', $event, c.a)" (change)="commitColor()" (blur)="commitColor()" />
                <input class="fg-hex" [value]="c.hex.slice(1).toUpperCase()" (change)="style({ stroke: toCss('#' + val($event), c.a) })" />
              } @else {
                <input class="fg-hex wide" [value]="st" (change)="style({ stroke: val($event) })" />
              }
              <label class="fg-field small" [title]="'ed.pp2.strokeWeight' | t"><span [reaScrub]="e.style.strokeWidth ?? 1" [minValue]="0" (scrub)="style({ strokeWidth: $event })">≡</span>
                <input type="number" min="0" [value]="e.style.strokeWidth ?? 1" (change)="style({ strokeWidth: num($event) })" /></label>
              <button class="fg-rm" [title]="'ed.pp2.removeStroke' | t" [attr.aria-label]="'ed.pp2.removeStroke' | t" (click)="style({ stroke: undefined, strokeWidth: undefined })">−</button>
            </div>
          }
        </section>

        <!-- Effects -->
        <section class="fg-sec">
          <h5>{{ 'ed.pp2.effects' | t }} <button class="fg-add" [title]="'ed.pp2.addShadow' | t" [attr.aria-label]="'ed.pp2.addShadow' | t" (click)="addShadow()">+</button></h5>
          @for (sh of shadows(); track $index; let i = $index) {
            <div class="fg-effect">
              <div class="fg-row2">
                <select [value]="sh.inset ? 'inner' : 'drop'" (change)="editShadow(i, { inset: val($event) === 'inner' })">
                  <option value="drop">{{ 'ed.pp2.dropShadow' | t }}</option><option value="inner">{{ 'ed.pp2.innerShadow' | t }}</option>
                </select>
                <button class="fg-rm" [title]="'ed.pp2.removeEffect' | t" [attr.aria-label]="'ed.pp2.removeEffect' | t" (click)="removeShadow(i)">−</button>
              </div>
              <div class="fg-row4">
                <label class="fg-field"><span [reaScrub]="sh.x" (scrub)="editShadow(i, { x: $event })">X</span><input type="number" [value]="sh.x" (change)="editShadow(i, { x: num($event) ?? 0 })" /></label>
                <label class="fg-field"><span [reaScrub]="sh.y" (scrub)="editShadow(i, { y: $event })">Y</span><input type="number" [value]="sh.y" (change)="editShadow(i, { y: num($event) ?? 0 })" /></label>
                <label class="fg-field" [title]="'ed.pp2.blur' | t"><span [reaScrub]="sh.blur" [minValue]="0" (scrub)="editShadow(i, { blur: $event })">B</span><input [attr.aria-label]="'ed.pp2.blur' | t" type="number" min="0" [value]="sh.blur" (change)="editShadow(i, { blur: num($event) ?? 0 })" /></label>
                <label class="fg-field" [title]="'ed.pp2.spread' | t"><span [reaScrub]="sh.spread" (scrub)="editShadow(i, { spread: $event })">S</span><input [attr.aria-label]="'ed.pp2.spread' | t" type="number" [value]="sh.spread" (change)="editShadow(i, { spread: num($event) ?? 0 })" /></label>
              </div>
              <div class="fg-paint">
                @if (parsed(sh.color); as c) {
                  <input type="color" [value]="c.hex" (change)="editShadow(i, { color: toCss(val($event), c.a) })" />
                  <input class="fg-hex" [value]="c.hex.slice(1).toUpperCase()" (change)="editShadow(i, { color: toCss('#' + val($event), c.a) })" />
                  <label class="fg-pct"><input type="number" min="0" max="100" [value]="round(c.a * 100)" (change)="editShadow(i, { color: toCss(c.hex, (num($event) ?? 100) / 100) })" />%</label>
                }
              </div>
            </div>
          }
        </section>

        @if (n.type === 'frame' && count() === 1) { <rea-layout-grid-editor [node]="n" /> }

        <!-- Text -->
        @if (hasText(n)) {
          <section class="fg-sec">
            <h5>{{ (n.type === 'text' ? 'ed.pp2.text' : 'ed.pp2.textInherited') | t }}</h5>
            @if (n.type === 'text' || n.type === 'button') { <rea-text-style [node]="n" /> }
            <select class="fg-font" [value]="e.style?.fontFamily ?? ''" (change)="style({ fontFamily: val($event) })" [class.ovr]="store.isOverridden(n, 'style', 'fontFamily')">
              <option value="">{{ 'ed.pp2.fontDefault' | t }}</option>
              @for (f of fonts; track f) { <option [value]="f">{{ f }}</option> }
            </select>
            <div class="fg-row2">
              <select [value]="e.style?.fontWeight ?? ''" (change)="style({ fontWeight: num($event) })" [class.ovr]="store.isOverridden(n, 'style', 'fontWeight')">
                <option value="">{{ tsv(n, 'fontWeight') ? ('ed.pp2.styleValue' | t: { v: tsv(n, 'fontWeight') ?? '' }) : ('ed.pp2.w400' | t) }}</option>
                @for (w of weights; track w.v) { <option [value]="w.v">{{ ('ed.pp2.w' + w.v) | t }}</option> }
              </select>
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'fontSize')" [title]="'ed.pp2.fontSize' | t">
                <span [reaScrub]="e.style?.fontSize ?? 16" [minValue]="1" (scrub)="style({ fontSize: $event })">Aa</span>
                <input [attr.aria-label]="'ed.pp2.fontSize' | t" type="number" min="1" [value]="e.style?.fontSize ?? ''" [placeholder]="tsv(n, 'fontSize') ?? 16" (change)="style({ fontSize: num($event) })" /></label>
            </div>
            <div class="fg-row2">
              <label class="fg-field" [title]="'ed.pp2.lineHeight' | t" [class.ovr]="store.isOverridden(n, 'style', 'lineHeight')">
                <span [reaScrub]="e.style?.lineHeight ?? 1.5" [step]="0.05" [minValue]="0" (scrub)="style({ lineHeight: $event })">↕A</span>
                <input [attr.aria-label]="'ed.pp2.lineHeight' | t" type="number" step="0.05" min="0" [value]="e.style?.lineHeight ?? ''" [placeholder]="tsv(n, 'lineHeight') ?? ('ed.pp2.auto' | t)" (change)="style({ lineHeight: num($event) })" /></label>
              <label class="fg-field" [title]="'ed.pp2.letterSpacing' | t">
                <span [reaScrub]="e.style?.letterSpacing ?? 0" [step]="0.1" (scrub)="style({ letterSpacing: $event || undefined })">|A|</span>
                <input [attr.aria-label]="'ed.pp2.letterSpacing' | t" type="number" step="0.1" [value]="e.style?.letterSpacing ?? 0" (change)="style({ letterSpacing: num($event) || undefined })" /></label>
            </div>
            @if (n.type === 'text') {
              <label class="fg-field" [class.ovr]="store.isOverridden(n, 'style', 'maxLines')" [title]="'ed.pp.maxLinesTip' | t">
                <span>≡…</span><input id="pp-max-lines" [attr.aria-label]="'ed.pp.maxLines' | t" type="number" min="0" max="20" [value]="e.style?.maxLines || ''" [placeholder]="'ed.pp.maxLinesAll' | t" (change)="style({ maxLines: num($event) || undefined })" /></label>
            }
            <div class="fg-paint" [class.ovr]="store.isOverridden(n, 'style', 'color')">
              @if (tokenOf(e.style?.color); as k) {
                <span class="fg-token"><i [style.background]="e.style!.color"></i>{{ tokenName(k) }}</span>
                <button class="fg-link" [title]="'ed.theme.detachTip' | t" (click)="style({ color: themeColor(k) })">{{ 'ed.theme.detach' | t }}</button>
              } @else if (parsed(e.style?.color || '#111827'); as c) {
                <input type="color" [value]="c.hex" (input)="liveColor('color', $event, c.a)" (change)="commitColor()" (blur)="commitColor()" />
                <input class="fg-hex" [value]="e.style?.color ? c.hex.slice(1).toUpperCase() : ''" [placeholder]="'ed.pp2.inherit' | t" (change)="style({ color: val($event) ? toCss('#' + val($event), c.a) : undefined })" />
              }
              @if (e.style?.color) { <button class="fg-rm" [title]="'ed.pp.inheritColor' | t" [attr.aria-label]="'ed.pp.inheritColor' | t" (click)="style({ color: undefined })">−</button> }
            </div>
            <div class="swatches small">
              @for (k of themeKeys; track k) { <button class="tok" [style.background]="'var(--c-' + k + ')'" [title]="('ed.theme.' + k) | t" [attr.aria-label]="('ed.theme.' + k) | t" (click)="style({ color: 'var(--c-' + k + ')' })"></button> }
              @for (p of theme().palette; track p.id) { <button class="tok" [style.background]="p.value" [title]="p.name" [attr.aria-label]="p.name" (click)="style({ color: 'var(--c-' + p.id + ')' })"></button> }
            </div>
            <div class="fg-row2">
              <div class="fg-seg" [class.ovr]="store.isOverridden(n, 'style', 'textAlign')">
                @for (a of aligns; track a.id) {
                  <button [class.on]="(e.style?.textAlign ?? 'left') === a.id" [title]="a.title | t" [attr.aria-label]="a.title | t" (click)="style({ textAlign: a.id === 'left' ? undefined : a.id })">{{ a.icon }}</button>
                }
              </div>
              @if (n.type === 'text') {
                <div class="fg-seg" [title]="'ed.pp2.textResizing' | t">
                  <button [class.on]="e.width.mode === 'hug'" [title]="'ed.pp2.autoW' | t" [attr.aria-label]="'ed.pp2.autoW' | t" (click)="textSizing('autoW')">↔</button>
                  <button [class.on]="e.width.mode !== 'hug' && e.height.mode === 'hug'" [title]="'ed.pp2.autoH' | t" [attr.aria-label]="'ed.pp2.autoH' | t" (click)="textSizing('autoH')">↕</button>
                  <button [class.on]="e.width.mode !== 'hug' && e.height.mode !== 'hug'" [title]="'ed.pp2.fixedSize' | t" [attr.aria-label]="'ed.pp2.fixedSize' | t" (click)="textSizing('fixed')">▢</button>
                </div>
              }
            </div>
            <select [value]="e.style?.textTransform ?? ''" (change)="style({ textTransform: val($event) })" [title]="'ed.pp2.case' | t">
              <option value="">{{ 'ed.pp2.caseNone' | t }}</option><option value="uppercase">UPPER CASE</option><option value="lowercase">lower case</option><option value="capitalize">Title Case</option>
            </select>
          </section>
        }
        <rea-style-fx [node]="n" [eff]="e" />
        @if (count() === 1 && (n.type === 'image' || n.type === 'frame' || n.type === 'rect')) { <rea-mask-panel [node]="n" /> }
        <rea-vector-panel [node]="n" />
      } @else if (tab() === 'prototype') {
        @if (count() === 1) { <rea-prototype-panel [node]="n" /> }
      } @else {
        <!-- ===================== Content tab (CMS) ===================== -->
        <p class="pp-help">{{ help(n) }}</p>
        @if (count() === 1 && n.id !== store.doc().id) {
          <!-- 2.2.0: a stable name code blocks find the layer by (data-ref) -->
          <section class="fg-sec pp-ref">
            <h5>{{ 'ed.ref.title' | t }}</h5>
            <input id="pp-ref" class="mono" [value]="n.props?.ref ?? ''" [placeholder]="'ed.ref.placeholder' | t" spellcheck="false" autocomplete="off"
                   [attr.aria-invalid]="refError() ? true : null" [attr.aria-label]="'ed.ref.title' | t" (change)="setRef(n, $event)" />
            @if (refError(); as e) { <p class="pp-help danger" role="alert">{{ e }}</p> }
            @else if (n.props?.ref) { <p class="pp-help"><code>anasaki.ref('{{ n.props!.ref }}')</code> · <code>[data-ref="{{ n.props!.ref }}"]</code></p> }
            @else { <p class="pp-help">{{ 'ed.ref.hint' | t }}</p> }
          </section>
        }
        @switch (n.type) {
          @case ('text') {
            <section class="fg-sec">
              <h5>{{ 'ed.pp.text' | t }}</h5>
              @if (n.props?.rich) {
                <div class="rich-note">✨ {{ 'ed.pp.richNote' | t }}
                  <button class="link" (click)="props({ rich: undefined })">{{ 'ed.pp.richClear' | t }}</button></div>
              } @else {
                <rea-binding-input [multiline]="true" [rows]="4" [nodeId]="n.id" [value]="n.props?.text ?? ''" inputId="pp-text" (changed)="props({ text: $event })" />
              }
              <p class="hint">{{ 'ed.pp.dblHintRich' | t }}</p>
              <label>{{ 'ed.pp.binding' | t }}
                <select (change)="insertBinding(n, $event)">
                  <option value="">{{ 'ed.pp.bindingPh' | t }}</option>
                  @for (b of bindings; track b.token) { <option [value]="b.token">{{ b.label | t }}</option> }
                </select>
              </label>
              <label>{{ 'ed.pp.tag' | t }}
                <select [value]="n.props?.tag || 'p'" (change)="props({ tag: val($event) })">
                  @for (t of textTags; track t.id) { <option [value]="t.id">{{ t.label | t }}</option> }
                </select>
              </label>
              <label>{{ 'ed.pp.link' | t }} <rea-link-input [value]="n.props?.href ?? ''" (changed)="props({ href: $event || undefined })" /></label>
            </section>
          }
          @case ('button') {
            <section class="fg-sec">
              <h5>{{ 'ed.pp.button' | t }}</h5>
              <label>{{ 'ed.pp.buttonText' | t }} <rea-binding-input [nodeId]="n.id" [value]="n.props?.text ?? ''" inputId="pp-button-text" (changed)="props({ text: $event })" /></label>
              <label>{{ 'ed.pp.link' | t }} <rea-link-input [value]="n.props?.href ?? ''" (changed)="props({ href: $event || undefined })" /></label>
              <label class="fg-check"><input type="checkbox" [checked]="!!n.props?.newTab" (change)="props({ newTab: checked($event) || undefined })" /> {{ 'ed.pp.newTab' | t }}</label>
            </section>
            @if (count() === 1) { <rea-icon-picker [node]="n" [forButton]="true" /> }
          }
          @case ('image') {
            <section class="fg-sec">
              <h5>{{ 'ed.pp.image' | t }}</h5>
              <div class="fg-image">
                @if (resolvedSrc(n.props?.src); as src) { <img [src]="src" alt="" /> }
                @else { <span>{{ n.props?.src ? ('ed.pp.boundImage' | t) : ('ed.pp.noImage' | t) }}</span> }
              </div>
              @if (sourceLabel(n.props?.src); as label) { <p class="hint">🔗 {{ label | t }}</p> }
              <button class="btn btn-sm btn-block" (click)="pickFor.set('src')">🖼 {{ 'ed.pp.chooseImage' | t }}</button>
              <div class="fg-row2">
                <button class="btn btn-sm" [class.on]="n.props?.src === siteLogo" (click)="props({ src: siteLogo, alt: n.props?.alt || '{{site.title}}' })" [title]="'ed.pp.useLogoTip' | t">⭐ {{ 'ed.pp.useLogo' | t }}</button>
                <button class="btn btn-sm" [class.on]="n.props?.src === postImage" (click)="props({ src: postImage })">{{ 'ed.pp.usePostImage' | t }}</button>
              </div>
              @if (n.props?.src === siteLogo && !store.siteCtx()['logo']) { <p class="hint warn">{{ 'ed.pp.noLogo' | t }}</p> }
              <label>{{ 'ed.pp.alt' | t }} <rea-binding-input [nodeId]="n.id" [value]="n.props?.alt ?? ''" inputId="pp-alt" (changed)="props({ alt: $event })" /></label>
              <label>{{ 'ed.pp.fit' | t }}
                <select [value]="e.style?.objectFit || 'cover'" (change)="style({ objectFit: $any(val($event)) })">
                  <option value="cover">{{ 'ed.pp.fitCover' | t }}</option><option value="contain">{{ 'ed.pp.fitContain' | t }}</option><option value="fill">{{ 'ed.pp2.stretch' | t }}</option>
                </select>
              </label>
              <label>{{ 'ed.pp.link' | t }} <rea-link-input [value]="n.props?.href ?? ''" (changed)="props({ href: $event || undefined })" /></label>
              <label>URL <rea-binding-input [nodeId]="n.id" [value]="n.props?.src ?? ''" inputId="pp-src" (changed)="props({ src: $event })" /></label>
            </section>
          }
          @case ('menu') {
            <section class="fg-sec">
              <h5>{{ 'ed.pp.menu' | t }}</h5>
              <select [value]="n.props?.menuId ?? ''" (change)="props({ menuId: num($event) })">
                <option value="">{{ 'ed.ph.chooseMenu' | t }}</option>
                @for (m of store.menus(); track m.id) { <option [value]="m.id">{{ m.name }}</option> }
              </select>
              <p class="hint">{{ 'ed.pp.menuHint' | t }}</p>
              <label class="fg-check"><input type="checkbox" [checked]="!!n.props?.burger" (change)="props({ burger: checked($event) || undefined })" /> {{ 'cs.burger' | t }}</label>
              <p class="hint">{{ 'cs.burgerHint' | t }}</p>
            </section>
            <!-- 2.2.0: every link's padding / corners, and the look of the current page's link -->
            <section class="fg-sec" id="pp-menu-links">
              <h5>{{ 'ed.menu.links' | t }}</h5>
              <div class="fg-row2">
                <label class="fg-field" [title]="'ed.menu.padY' | t"><span>↕</span><input type="number" min="0" id="pp-menu-pady" [attr.aria-label]="'ed.menu.padY' | t" [value]="n.style?.linkPadding?.[0] ?? ''" placeholder="0" (change)="linkPad(n, 0, $event)" /></label>
                <label class="fg-field" [title]="'ed.menu.padX' | t"><span>↔</span><input type="number" min="0" id="pp-menu-padx" [attr.aria-label]="'ed.menu.padX' | t" [value]="n.style?.linkPadding?.[1] ?? ''" placeholder="0" (change)="linkPad(n, 1, $event)" /></label>
              </div>
              <label class="fg-field" [title]="'ed.menu.radius' | t"><span>◜</span><input type="number" min="0" id="pp-menu-radius" [attr.aria-label]="'ed.menu.radius' | t" [value]="n.style?.linkRadius ?? ''" placeholder="0" (change)="store.setStyle(n.id, { linkRadius: num($event) || undefined })" /></label>
              <h5>{{ 'ed.menu.current' | t }}</h5>
              <div class="fx-colors">
                <label>{{ 'fx.fill' | t }}<input type="color" id="pp-menu-cur-fill" [attr.aria-label]="'ed.menu.curFill' | t" [value]="hexOr(n.style?.active?.fill, '#ff7a7a')" (change)="menuActive(n, { fill: val($event) })" /></label>
                <label>{{ 'fx.text' | t }}<input type="color" id="pp-menu-cur-color" [attr.aria-label]="'ed.menu.curColor' | t" [value]="hexOr(n.style?.active?.color, '#ffffff')" (change)="menuActive(n, { color: val($event) })" /></label>
                <label>{{ 'fx.strokeShort' | t }}<input type="color" [attr.aria-label]="'ed.menu.curStroke' | t" [value]="hexOr(n.style?.active?.stroke, '#ff7a7a')" (change)="menuActive(n, { stroke: val($event) })" /></label>
                @if (n.style?.active) { <button class="fg-link" (click)="store.setStyle(n.id, { active: undefined })">{{ 'pt.reset' | t }}</button> }
              </div>
              <p class="hint">{{ 'ed.menu.currentHint' | t }}</p>
            </section>
          }
          @case ('postList') {
            @if (n.children?.length) {
              <section class="fg-sec">
                <button class="btn btn-sm btn-block" (click)="editCard(n)">✏️ {{ 'pt.editCard' | t }}</button>
                <p class="hint">{{ 'pt.editCardHint' | t }}</p>
              </section>
            }
            <section class="fg-sec">
              <h5>{{ 'ed.pp.cardStyle' | t }}</h5>
              <div class="card-styles">
                @for (c of cardStyles; track c.id) {
                  <button (click)="applyCard(n, c)" [title]="('ed.card.' + c.id + '.d') | t"><span>{{ c.icon }}</span>{{ ('ed.card.' + c.id) | t }}</button>
                }
              </div>
              <p class="hint">{{ 'ed.pp.cardStyleHint' | t }}</p>
              @if (e.layout?.mode === 'horizontal') {
                <label>{{ 'ed.pp.columnsFor' | t: { bp: bpLabel() } }}
                  <select [value]="'' + (e.layout.columns ?? '')" (change)="layout({ columns: num($event) })">
                    <option value="">{{ 'ed.pp.columnsAuto' | t }}</option>
                    @for (c of [1, 2, 3, 4, 5, 6]; track c) { <option [value]="'' + c">{{ c }}</option> }
                  </select>
                </label>
              }
            </section>
            <section class="fg-sec">
              <h5>{{ 'ed.pp.query' | t }}</h5>
              <p class="hint">{{ 'ed.pp.queryHint' | t }}</p>
              <div class="fg-row2">
                <label>{{ 'ed.pp.show' | t }}
                  <select [value]="n.props?.query?.postType || 'post'" (change)="query(n, { postType: val($event) })">
                    <option value="post">{{ 'ed.pp.posts' | t }}</option><option value="page">{{ 'ed.pp.pages' | t }}</option>
                    @if (postTypes.list().length) {
                      <optgroup [label]="'ed.pp.customTypes' | t">
                        @for (pt of postTypes.list(); track pt.type) { <option [value]="pt.type">{{ pt.icon || '📦' }} {{ typeName(pt) }}</option> }
                      </optgroup>
                    }
                  </select>
                </label>
                <label>{{ 'ed.pp.category' | t }}
                  <select [value]="n.props?.query?.category || ''" (change)="query(n, { category: val($event) || undefined })">
                    <option value="">{{ 'ed.pp.all' | t }}</option><option value="current">{{ 'ed.pp.currentCat' | t }}</option>
                    @for (t of store.terms(); track t.id) { <option [value]="t.slug">{{ t.name }}</option> }
                  </select>
                </label>
              </div>
              <div class="fg-row2">
                <label>{{ 'ed.pp.limit' | t }} <input type="number" min="1" max="50" [value]="n.props?.query?.limit ?? 6" (change)="query(n, { limit: num($event) })" /></label>
                <label>{{ 'ed.pp.order' | t }}
                  <select [value]="n.props?.query?.order || 'newest'" (change)="query(n, { order: $any(val($event)) })">
                    <option value="newest">{{ 'ed.pp.newest' | t }}</option><option value="oldest">{{ 'ed.pp.oldest' | t }}</option><option value="title">{{ 'ed.pp.byTitle' | t }}</option>
                    <option value="manual">{{ 'ed.pp.manual' | t }}</option>
                  </select>
                </label>
              </div>
              <p class="hint">📌 {{ 'ed.pp.pinHint' | t }}</p>
              <label>{{ 'ed.pp.empty' | t }} <input [value]="n.props?.emptyText ?? ''" (change)="props({ emptyText: val($event) })" /></label>
            </section>
          }
          @case ('codeblock') {
            <rea-codeblock-panel [node]="n" (runPreview)="runPreview.emit()" />
          }
          @case ('embed') {
            <section class="fg-sec">
              <h5>HTML</h5>
              <textarea rows="7" class="mono" [value]="n.props?.html ?? ''" (change)="props({ html: val($event) })"></textarea>
              <p class="hint">{{ 'ed.pp.embedHint' | t }}</p>
            </section>
          }
          @case ('instance') {
            <section class="fg-sec">
              <h5>{{ 'ed.pp2.componentNamed' | t: { name: componentName(n) } }}</h5>
              <div class="fg-row2">
                <button class="btn btn-sm" (click)="editComponent.emit(n.props?.componentId!)">{{ 'ed.pp2.editMain' | t }}</button>
                <button class="btn btn-sm" title="Ctrl+Alt+B" (click)="store.detachInstance(n.id)">{{ 'ed.cmd.detach' | t }}</button>
              </div>
              <p class="hint">{{ 'ed.pp.componentHint' | t }}</p>
            </section>
            <rea-variant-picker [node]="n" />
            <rea-instance-parts [node]="n" />
          }
          @case ('frame') {
            <section class="fg-sec">
              <h5>{{ 'ed.node.frame' | t }}</h5>
              <label>{{ 'ed.pp.frameLink' | t }} <rea-link-input [value]="n.props?.href ?? ''" (changed)="props({ href: $event || undefined })" /></label>
              <label>{{ 'ed.pp2.htmlTag' | t }}
                <select [value]="n.props?.tag || 'div'" (change)="props({ tag: val($event) })">
                  @for (t of frameTags; track t.id) { <option [value]="t.id">{{ t.label | t }}</option> }
                </select>
              </label>
            </section>
          }
          @case ('collection') { <rea-collection-panel [node]="n" /> }
          @case ('pagination') { <rea-pagination-panel [node]="n" /> }
          @case ('input') { <rea-input-panel [node]="n" /> }
          @case ('icon') { <rea-icon-picker [node]="n" /> }
          @case ('shape') { <rea-icon-picker [node]="n" /> }
          @case ('pluginBlock') {
            <section class="fg-sec">
              <h5>🧩 {{ blockTitle(n) }}</h5>
              @if (store.blockInfo(n); as b) {
                <p class="hint">{{ 'ed.pp.blockFrom' | t: { name: b.pluginName } }}</p>
                <rea-schema-form [fields]="b.props" [values]="n.props?.values ?? {}" [compact]="true" (changed)="props({ values: $event })" />
              } @else {
                <p class="hint">{{ 'ed.pp.blockMissing' | t }}</p>
              }
            </section>
            <rea-block-parts [node]="n" />
          }
          @case ('download') {
            <section class="fg-sec">
              <h5>⬇ {{ 'ed.node.download' | t }}</h5>
              <p class="hint">{{ 'ed.pp.downloadHint' | t }}</p>
              @if (store.fileInfo(n.props?.mediaId); as f) {
                <div class="dl-file"><b>📎 {{ f.filename }}</b><small>⬇ {{ f.downloads ?? 0 }}</small></div>
              }
              <button class="btn btn-sm" (click)="pickFor.set('file')">📁 {{ (n.props?.mediaId ? 'ed.pp.changeFile' : 'ed.pp.chooseFile') | t }}</button>
              <label>{{ 'ed.pp.buttonText' | t }} <input [value]="n.props?.text ?? ''" [placeholder]="'ed.pp.fileNameDefault' | t" (change)="props({ text: val($event) })" /></label>
              <label class="check"><input type="checkbox" [checked]="!!n.props?.showSize" (change)="props({ showSize: $any($event.target).checked })" /> {{ 'ed.pp.showSize' | t }}</label>
            </section>
          }
          @case ('breadcrumb') {
            <section class="fg-sec">
              <h5>» {{ 'ed.node.breadcrumb' | t }}</h5>
              <p class="hint">{{ 'ed.pp.bcHint' | t }}</p>
              <label>{{ 'ed.pp.bcHome' | t }} <input [value]="n.props?.homeLabel ?? ''" [placeholder]="'ed.def.home' | t" (change)="props({ homeLabel: val($event) })" /></label>
              <label>{{ 'ed.pp.bcSep' | t }}
                <select [value]="n.props?.separator || '›'" (change)="props({ separator: val($event) })">
                  @for (sp of separators; track sp) { <option [value]="sp">{{ sp }}</option> }
                </select>
              </label>
              <label>{{ 'ed.pp.bcCurrent' | t }}</label>
              <div class="swatches">
                @for (k of themeKeys; track k) { <button class="tok" [class.on]="n.props?.currentColor === 'var(--c-' + k + ')'" [style.background]="'var(--c-' + k + ')'" [title]="('ed.theme.' + k) | t" [attr.aria-label]="('ed.theme.' + k) | t" (click)="props({ currentColor: 'var(--c-' + k + ')' })"></button> }
                <button class="none" [title]="'ed.pp.inheritColor' | t" [attr.aria-label]="'ed.pp.inheritColor' | t" (click)="props({ currentColor: undefined })">∅</button>
              </div>
              <label class="fg-check"><input type="checkbox" [checked]="!!n.props?.hideCategory" (change)="props({ hideCategory: checked($event) || undefined })" /> {{ 'ed.pp.bcHideCat' | t }}</label>
            </section>
          }
          @default { <p class="hint pad">{{ 'ed.pp.nothing' | t }}</p> }
        }
        @if (count() === 1 && s.parent) { <rea-conditions-panel [node]="n" /> }
        @if (s.parent && n.type !== 'instance') {
          <section class="fg-sec">
            <button class="btn btn-sm" (click)="makeComponent(n)" title="Ctrl+Alt+K">◇ {{ 'ed.cmd.component' | t }}</button>
          </section>
        }
      }
    } @else {
      @if (store.kind() === 'post') { <rea-page-settings /> }
      <div class="pp-empty">
        <h3>{{ 'ed.pp.start' | t }}</h3>
        <ul class="tips">
          <li><kbd>V</kbd> {{ 'ed.cmd.move' | t }} · <kbd>H</kbd>/<kbd>Space</kbd> {{ 'ed.cmd.hand' | t }} · <kbd>F</kbd> {{ 'ed.cmd.toolFrame' | t }} · <kbd>R</kbd> {{ 'ed.cmd.toolRect' | t }} · <kbd>O</kbd> {{ 'ed.cmd.toolEllipse' | t }} · <kbd>T</kbd> {{ 'ed.cmd.toolText' | t }}</li>
          <li [innerHTML]="'ed.pp.tip1' | t"></li>
          <li [innerHTML]="'ed.pp.tip2' | t"></li>
          <li [innerHTML]="'ed.pp.tip3' | t"></li>
          <li [innerHTML]="'ed.pp.tip4' | t"></li>
          <li [innerHTML]="'ed.pp.tip5' | t"></li>
          <li><kbd>Ctrl</kbd>+<kbd>Z</kbd> / <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Z</kbd> {{ 'ed.cmd.undo' | t }} / {{ 'ed.cmd.redo' | t }}</li>
        </ul>
        <button class="btn btn-sm" (click)="showShortcuts.emit()">{{ 'ed.pp.allShortcuts' | t }}</button>
      </div>

      <section class="fg-sec theme-ed">
        <h5>🎨 {{ 'ed.theme.title' | t }}</h5>
        <p class="hint">{{ 'ed.theme.hint' | t }}</p>
        @for (k of themeKeys; track k) {
          <label class="theme-row">
            <input type="color" [value]="theme().colors[k]" [title]="theme().colors[k]" (input)="setThemeColor(k, val($event))" />
            <span>{{ ('ed.theme.' + k) | t }}<small>{{ ('ed.theme.' + k + '.d') | t }}</small></span>
          </label>
        }
        <label>{{ 'ed.theme.fontHeading' | t }}
          <select [value]="theme().fontHeading" (change)="setThemeFont('fontHeading', val($event))">
            <option value="">{{ 'ed.theme.fontDefault' | t }}</option>
            @for (f of fonts; track f) { <option [value]="f">{{ f }}</option> }
          </select>
        </label>
        <label>{{ 'ed.theme.fontBody' | t }}
          <select [value]="theme().fontBody" (change)="setThemeFont('fontBody', val($event))">
            <option value="">{{ 'ed.theme.fontDefault' | t }}</option>
            @for (f of fonts; track f) { <option [value]="f">{{ f }}</option> }
          </select>
        </label>
        <!-- 2.2.0: where the tablet / mobile designs start on the site -->
        <label>{{ 'ed.theme.bpMobile' | t }}
          <select id="theme-bp-mobile" [value]="theme().breakpoints?.mobile ?? 640" (change)="setThemeBp('mobile', val($event))">
            @for (w of [640, 767, 820]; track w) { <option [value]="w">≤ {{ w }} px{{ w === 640 ? ' · ' + ('ed.theme.bpDefault' | t) : w === 767 ? ' · Tailwind md' : '' }}</option> }
          </select>
        </label>
        <label>{{ 'ed.theme.bpTablet' | t }}
          <select id="theme-bp-tablet" [value]="theme().breakpoints?.tablet ?? 1024" (change)="setThemeBp('tablet', val($event))">
            @for (w of [960, 1024, 1180, 1280]; track w) { <option [value]="w">≤ {{ w }} px{{ w === 1024 ? ' · ' + ('ed.theme.bpDefault' | t) : '' }}</option> }
          </select>
        </label>
        <p class="hint">{{ 'ed.theme.bpHint' | t }}</p>
        <rea-style-library />
        @if (store.themeDraft()) {
          <div class="fg-row2">
            <button class="btn btn-sm btn-primary" (click)="saveTheme()">{{ 'ed.theme.save' | t }}</button>
            <button class="btn btn-sm" (click)="store.themeDraft.set(null)">{{ 'common.cancel' | t }}</button>
          </div>
          <p class="hint">{{ 'ed.theme.unsaved' | t }}</p>
        }
      </section>
    }

    <rea-media-picker [kind]="pickFor() === 'file' ? '' : 'image'" [open]="pickFor() !== null" (openChange)="!$event && pickFor.set(null)" (picked)="picked($event)" />
  `,
})
export class PropsPanel {
  readonly store = inject(EditorStore);
  readonly i18n = inject(I18n);
  /** Custom post types (site code and JS plugins) for the post list's type picker. */
  readonly postTypes = inject(PostTypes);
  readonly editComponent = output<number>();
  readonly showShortcuts = output<void>();
  /** "Run in preview" of a frontend code block (the editor opens the page preview). */
  readonly runPreview = output<void>();

  readonly tab = signal<'design' | 'content' | 'prototype'>('design');
  /** A plugin block part double-clicked on the canvas: show the Content tab where its parts are. */
  private readonly partTab = effect(() => { if (this.store.partPick()) untracked(() => this.tab.set('content')); });
  readonly padOpen = signal(false);
  readonly pickFor = signal<'src' | 'bg' | 'file' | null>(null);

  readonly sel = computed(() => this.store.selected());
  readonly ids = computed(() => this.store.selectedIds());
  readonly count = computed(() => this.ids().length);
  readonly eff = computed(() => {
    const s = this.sel();
    return s ? this.store.eff(s.node) : null;
  });
  readonly pMode = computed(() => {
    const s = this.sel();
    return s ? this.store.parentMode(s.node.id) : '';
  });
  readonly parentAuto = computed(() => {
    const p = this.sel()?.parent;
    return !!p && (this.store.eff(p).layout?.mode ?? 'none') !== 'none';
  });
  readonly isAuto = computed(() => {
    const m = this.eff()?.layout?.mode;
    return m === 'horizontal' || m === 'vertical';
  });
  readonly pad = computed<[number, number, number, number]>(() => (this.eff()?.layout?.padding ?? [0, 0, 0, 0]) as [number, number, number, number]);
  readonly opacityPct = computed(() => Math.round((this.eff()?.style?.opacity ?? 1) * 100));
  readonly shadows = computed(() => parseShadows(this.eff()?.style?.shadow));
  readonly bpKey = computed(() => this.store.bp() as 'tablet' | 'mobile');
  readonly bpLabel = computed(() => BREAKPOINTS.find(b => b.id === this.store.bp())!.label);

  readonly axes = ['width', 'height'] as const;
  /** 2.2.2: aspect ratios offered (Figma "lock ratio"), blend modes (Figma layer blend). */
  readonly aspects = ['1/1', '4/3', '3/2', '16/9', '21/9', '3/4', '2/3', '9/16'];
  readonly blends = [...BLEND_MODES];

  readonly anchors: { id: 'left' | 'center' | 'right' | 'scale'; icon: string }[] = [
    { id: 'left', icon: '⇤' }, { id: 'center', icon: '⇔' }, { id: 'right', icon: '⇥' }, { id: 'scale', icon: '↔%' },
  ];
  /** `title` = i18n key (the shortcut is appended as typed). */
  readonly alignBtns: { dir: AlignDir; icon: string; title: string; keys: string }[] = [
    { dir: 'left', icon: '⎸◧', title: 'ed.cmd.alignLeft', keys: 'Alt+A' },
    { dir: 'hcenter', icon: '◫', title: 'ed.cmd.alignHC', keys: 'Alt+H' },
    { dir: 'right', icon: '◨⎹', title: 'ed.cmd.alignRight', keys: 'Alt+D' },
    { dir: 'top', icon: '⬒', title: 'ed.cmd.alignTop', keys: 'Alt+W' },
    { dir: 'vcenter', icon: '⊟', title: 'ed.cmd.alignVC', keys: 'Alt+V' },
    { dir: 'bottom', icon: '⬓', title: 'ed.cmd.alignBottom', keys: 'Alt+S' },
  ];
  readonly aligns = [
    { id: 'left' as const, icon: '⯇', title: 'ed.pp2.taLeft' }, { id: 'center' as const, icon: '≡', title: 'ed.pp2.taCenter' },
    { id: 'right' as const, icon: '⯈', title: 'ed.pp2.taRight' }, { id: 'justify' as const, icon: '☰', title: 'ed.pp2.taJustify' },
  ];
  readonly sides = ['ed.pp2.top', 'ed.pp2.right', 'ed.pp2.bottom', 'ed.pp2.left'];
  readonly sideIcons = ['⊤', '⊣', '⊥', '⊢'];
  readonly textTags = [
    { id: 'p', label: 'ed.pp2.tagP' }, { id: 'h1', label: 'ed.tag.h1' }, { id: 'h2', label: 'ed.pp2.tagH2' },
    { id: 'h3', label: 'ed.pp2.tagH3' }, { id: 'h4', label: 'ed.pp2.tagH4' }, { id: 'span', label: 'ed.pp2.tagSpan' }, { id: 'blockquote', label: 'ed.pp2.tagQuote' },
  ];
  readonly frameTags = [
    { id: 'div', label: 'div' }, { id: 'section', label: 'section' }, { id: 'header', label: 'ed.tag.header' },
    { id: 'footer', label: 'ed.tag.footer' }, { id: 'nav', label: 'ed.tag.nav' }, { id: 'main', label: 'main' },
    { id: 'article', label: 'article' }, { id: 'aside', label: 'aside' },
  ];
  readonly weights = [
    { v: 300, label: 'Light' }, { v: 400, label: 'Regular' }, { v: 500, label: 'Medium' },
    { v: 600, label: 'Semi Bold' }, { v: 700, label: 'Bold' }, { v: 800, label: 'Extra Bold' },
  ];
  readonly swatches = ['#ffffff', '#f3f4f6', '#d9d9d9', '#111827', '#4f46e5', '#0d99ff', '#14ae5c', '#ffcd29', '#f24822', '#ec4899', 'linear-gradient(135deg,#4f46e5,#c026d3)'];
  readonly fonts = FONTS;
  readonly bindings = BINDINGS;
  readonly round = Math.round;
  readonly postImage = '{{post.image}}';
  readonly siteLogo = '{{site.logo}}';
  readonly themeKeys = THEME_COLOR_KEYS;
  readonly cardStyles = CARD_STYLES;
  readonly separators = ['›', '/', '»', '→', '·', '|'];
  /** "var(--c-x)" → x for theme colors and color styles; else null. */
  tokenOf(v?: string): string | null {
    const k = tokenOf(v) ?? /^var\(--c-([a-z][a-z0-9-]*)\)$/.exec(v?.trim() ?? '')?.[1] ?? null;
    return k && ((this.themeKeys as readonly string[]).includes(k) || this.theme().palette.some(c => c.id === k)) ? k : null;
  }
  tokenName(k: string) {
    return (this.themeKeys as readonly string[]).includes(k) ? this.i18n.t('ed.theme.' + k) : this.theme().palette.find(c => c.id === k)?.name || k;
  }
  readonly theme = computed(() => this.store.liveTheme());
  readonly bpIcon = computed(() => BREAKPOINTS.find(b => b.id === this.store.bp())!.icon);

  /** What the selected layer changes at the current breakpoint, as removable chips. */
  readonly overrides = computed(() => {
    const n = this.sel()?.node;
    const bp = this.store.bp();
    const o = bp === 'desktop' ? undefined : n?.responsive?.[bp];
    if (!o) return [];
    const out: { id: string; group: string; key?: string; label: string }[] = [];
    const add = (group: string, key?: string) => {
      const id = key ? `${group}.${key}` : group;
      const label = this.i18n.t(OVR_LABEL[id] ?? 'ed.ovr.other');
      if (group === 'y' && out.some(x => x.group === 'x')) return; // one "position" chip
      out.push({ id, group, key, label: key && !OVR_LABEL[id] ? `${label} (${key})` : label });
    };
    for (const k of Object.keys(o) as (keyof Override)[]) {
      const v = o[k];
      if ((k === 'layout' || k === 'style') && v && typeof v === 'object') Object.keys(v).forEach(kk => add(k, kk));
      else add(k);
    }
    return out;
  });

  /** On desktop: the breakpoint where the selected layer is set differently (first one). */
  readonly elsewhere = computed(() => {
    const r = this.sel()?.node.responsive;
    if (!r) return null;
    const bp = (['mobile', 'tablet'] as const).find(b => r[b]);
    return bp ? { bp, label: BREAKPOINTS.find(b => b.id === bp)!.label } : null;
  });

  /** The user picked a tab themselves: selecting layers no longer switches it for them. */
  private tabChosen = false;
  pickTab(t: 'design' | 'content' | 'prototype') {
    this.tabChosen = true;
    this.tab.set(t);
  }

  constructor() {
    void this.postTypes.ensureLoaded();
    // content-only layers open on the Content tab — when a layer gets selected (not on every
    // edit), and only until the user has picked a tab themselves
    effect(() => {
      const id = this.store.selectedId();
      untracked(() => {
        const t = id ? findNode(this.store.doc(), id)?.node.type : undefined;
        if (!this.tabChosen && t && ['menu', 'postList', 'postContent', 'embed', 'codeblock', 'breadcrumb', 'pagination', 'input'].includes(t)) this.tab.set('content');
      });
    });
  }

  isStatic(src?: string) { return !!src && !src.includes('{{'); }
  figmaType(n: DNode) { return n.type === 'rect' && (n.style?.radius ?? 0) >= 999 ? 'Ellipse' : FIGMA_TYPE[n.type]; }
  help(n: DNode) { return this.i18n.t('ed.help.' + n.type); }
  /** Selects the card template of a post list (edited like any frame). */
  editCard(n: DNode) { firstCard(this.store, n); this.tab.set('design'); }

  typeName(pt: CodePostType) { this.i18n.locale(); return localized(pt.label) || pt.type; }
  hasLayout(n: DNode) { return ['frame', 'postList', 'menu', 'button', 'input', 'pagination'].includes(n.type); }
  /** Layers whose layout section is padding only (no auto layout direction / gap). */
  padOnly(n: DNode) { return n.type === 'button' || n.type === 'input'; }
  hasText(n: DNode) { return !['image', 'rect', 'shape'].includes(n.type); }
  componentName(n: DNode) { return this.store.componentMap().get(n.props?.componentId ?? -1)?.name ?? '(missing)'; }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  checked(e: Event) { return (e.target as HTMLInputElement).checked; }
  num(e: Event): number | undefined {
    const v = (e.target as HTMLInputElement).value;
    return v === '' ? undefined : Number(v);
  }

  style(patch: Partial<Style>) {
    const color = patch.fill !== undefined && patch.color === undefined ? this.readableText(patch.fill) : null;
    this.store.setStyle(this.ids(), color ? { ...patch, color } : patch);
  }

  /** A solid colour for a fill / colour value (theme tokens resolved, first colour of a gradient). */
  private solid(v?: string): string | null {
    if (!v) return null;
    const t = /var\(--c-([a-z][a-z0-9-]*)\)/.exec(v);
    if (t) v = this.themeColor(t[1]);
    const m = /#[0-9a-f]{3,8}|rgba?\([^)]*\)/i.exec(v ?? '');
    const c = m ? parseColor(m[0]) : null;
    return c && c.a > 0.4 ? c.hex : null;
  }

  private contrastToast = 0;
  /**
   * New fill on a button (or a box with its own text colour): when its text
   * would be hard to read, switch the text to black or white.
   */
  private readableText(fill?: string): string | null {
    const n = this.sel()?.node;
    if (!n || this.count() !== 1 || !(n.type === 'button' || n.type === 'download' || n.style?.color)) return null;
    const bg = this.solid(fill), fg = this.solid(this.eff()?.style?.color ?? (n.type === 'button' ? '#ffffff' : undefined));
    if (!bg || !fg || contrast(bg, fg) >= 2.5) return null;
    const pick = contrast(bg, '#111827') >= contrast(bg, '#ffffff') ? '#111827' : '#ffffff';
    if (Date.now() - this.contrastToast > 4000) {
      this.contrastToast = Date.now();
      this.store.notify(this.i18n.t(pick === '#ffffff' ? 'fl.textWhite' : 'fl.textDark'));
    }
    return pick;
  }
  layout(patch: Partial<Layout>) { this.store.setLayout(this.ids(), patch); }
  props(patch: DNode['props']) { this.store.setProps(this.ids(), patch!); }

  /** "Top left" … "Bottom right" for the 3×3 alignment buttons (screen readers, tests — 2.2.0). */
  gridLabel(r: number, c: number): string {
    return tr('ed.al.cell', { v: tr(['ed.al.top', 'ed.al.middle', 'ed.al.bottom'][r]), h: tr(['ed.al.left', 'ed.al.center', 'ed.al.right'][c]) });
  }

  linkPad(n: DNode, i: 0 | 1, e: Event) {
    // the stored value, not the template's node: two quick edits (top/bottom, then left/right) must both stay
    const cur = findNode(this.store.doc(), n.id)?.node.style?.linkPadding ?? [0, 0];
    const p: [number, number] = [cur[0] ?? 0, cur[1] ?? 0];
    p[i] = this.num(e) || 0;
    this.store.setStyle(n.id, { linkPadding: p[0] || p[1] ? p : undefined });
  }
  menuActive(n: DNode, patch: { fill?: string; color?: string; stroke?: string }) {
    this.store.setStyle(n.id, { active: { ...n.style?.active, ...patch } });
  }
  hexOr(c: string | undefined, def: string) { return c && /^#[0-9a-f]{6}$/i.test(c) ? c : def; }

  /** "Name for code" (props.ref → data-ref): same rule as RefNameRe in render.go; unique on the page. */
  readonly refError = signal('');
  setRef(n: DNode, e: Event) {
    const v = (e.target as HTMLInputElement).value.trim();
    if (v && !/^[a-z][a-zA-Z0-9_-]{0,39}$/.test(v)) { this.refError.set(tr('ed.ref.bad')); return; }
    const taken = v ? findRef(this.store.doc(), v, n.id) : null;
    if (taken) { this.refError.set(tr('ed.ref.taken', { name: taken.name || taken.type })); return; }
    this.refError.set('');
    this.store.setProps(n.id, { ref: v || undefined });
  }
  query(n: DNode, patch: object) { this.store.setProps(n.id, { query: { ...n.props?.query, ...patch } }); }

  sizeVal(axis: 'width' | 'height'): number {
    const e = this.eff();
    if (!e) return 0;
    if (e[axis].mode === 'fixed') return e[axis].value ?? 0;
    const r = this.store.measure?.(e.id);
    return r ? Math.round(axis === 'width' ? r.w : r.h) : 0;
  }

  sizeMode(axis: 'width' | 'height', mode: SizeMode) {
    if (mode === 'fixed') this.store.setSize(this.ids(), axis, { mode, value: this.sizeVal(axis) || 100 });
    else this.store.setSize(this.ids(), axis, { mode });
  }

  textSizing(kind: 'autoW' | 'autoH' | 'fixed') {
    const ids = this.ids();
    const w = this.sizeVal('width'), h = this.sizeVal('height');
    this.store.beginGesture();
    if (kind === 'autoW') { this.store.setSize(ids, 'width', { mode: 'hug' }); this.store.setSize(ids, 'height', { mode: 'hug' }); }
    if (kind === 'autoH') { this.store.setSize(ids, 'width', { mode: 'fixed', value: w }); this.store.setSize(ids, 'height', { mode: 'hug' }); }
    if (kind === 'fixed') { this.store.setSize(ids, 'width', { mode: 'fixed', value: w }); this.store.setSize(ids, 'height', { mode: 'fixed', value: h }); }
    this.store.endGesture();
  }

  /** Wrap the selection in a new auto-layout frame with the given direction. */
  wrapAuto(mode: 'vertical' | 'horizontal') {
    this.store.transact(() => { // one undo step
      this.store.addAutoLayout();
      const id = this.store.selectedId();
      if (id && mode === 'horizontal') this.store.setLayout(id, { mode });
    });
  }

  setOpacity(pct: number) {
    const v = Math.max(0, Math.min(100, pct));
    this.style({ opacity: v >= 100 ? undefined : v / 100 });
  }

  setPad(p: (number | null)[]) {
    const cur = [...this.pad()];
    p.forEach((v, i) => { if (v !== null) cur[i] = Math.max(0, v); });
    this.layout({ padding: cur.some(x => x) ? (cur as [number, number, number, number]) : undefined });
  }

  setPadSide(side: number, v: number) {
    const p: (number | null)[] = [null, null, null, null];
    p[side] = v;
    this.setPad(p);
  }

  // 3×3 alignment grid: rows/cols map to align (cross axis) & justify (main axis)
  private grid() {
    const e = this.eff()!;
    const idx = (v?: string) => (v === 'center' ? 1 : v === 'end' ? 2 : 0);
    const main = idx(e.layout?.justify), cross = idx(e.layout?.align);
    return e.layout?.mode === 'horizontal' ? { row: cross, col: main } : { row: main, col: cross };
  }
  gridOn(r: number, c: number) {
    const g = this.grid();
    const between = this.eff()?.layout?.justify === 'between';
    const horizontal = this.eff()?.layout?.mode === 'horizontal';
    if (between) return horizontal ? g.row === r : g.col === c;
    return g.row === r && g.col === c;
  }
  gridSet(r: number, c: number) {
    const v = (i: number) => (['start', 'center', 'end'] as const)[i];
    const horizontal = this.eff()?.layout?.mode === 'horizontal';
    const between = this.eff()?.layout?.justify === 'between';
    const main = horizontal ? c : r, cross = horizontal ? r : c;
    this.layout({ align: cross === 0 ? undefined : v(cross), justify: between ? 'between' : main === 0 ? undefined : v(main) });
  }

  // colors
  parsed(c?: string) { return parseColor(c); }
  toCss(hex: string, a: number) { return toCss(hex, a); }
  private liveStarted = false;
  private liveTimer?: ReturnType<typeof setTimeout>;
  private readonly closeLive = inject(DestroyRef).onDestroy(() => this.commitColor());
  /**
   * Color picker drags update live as one undo step. Some pickers never fire
   * `change` (closed with Esc, focus lost), so the step is also closed on blur
   * and after a short pause — an open gesture would otherwise swallow later edits.
   */
  liveColor(key: 'fill' | 'stroke' | 'color', e: Event, a: number) {
    if (!this.liveStarted) { this.store.beginGesture(); this.liveStarted = true; }
    this.style({ [key]: toCss(this.val(e), a) });
    clearTimeout(this.liveTimer);
    this.liveTimer = setTimeout(() => this.commitColor(), 800);
  }
  commitColor() {
    clearTimeout(this.liveTimer);
    if (this.liveStarted) { this.store.endGesture(); this.liveStarted = false; }
  }

  // effects
  addShadow() {
    const list = [...this.shadows(), { inset: false, x: 0, y: 4, blur: 12, spread: 0, color: 'rgba(0,0,0,0.25)' }];
    this.style({ shadow: formatShadows(list) });
  }
  editShadow(i: number, patch: Partial<Shadow>) {
    const list = this.shadows().map((s, j) => (j === i ? { ...s, ...patch } : s));
    this.style({ shadow: formatShadows(list) });
  }
  removeShadow(i: number) {
    const list = this.shadows().filter((_, j) => j !== i);
    this.style({ shadow: list.length ? formatShadows(list) : undefined });
  }

  insertBinding(n: DNode, e: Event) {
    const token = this.val(e);
    (e.target as HTMLSelectElement).value = '';
    if (!token) return;
    if (n.props?.rich) this.store.setProps(n.id, { text: (n.props?.text ?? '') + token, rich: n.props.rich + token });
    else this.store.setProps(n.id, { text: (n.props?.text ?? '') + token });
  }

  /** Image source as it shows on the canvas ({{site.logo}} etc. resolved). */
  resolvedSrc(src?: string) {
    if (!src) return '';
    return src.includes('{{') ? bind(src, { site: this.store.siteCtx(), post: this.store.pageCtx() }) : src;
  }

  sourceLabel(src?: string) {
    if (src === this.siteLogo) return 'ed.pp.srcLogo';
    if (src === this.postImage) return 'ed.pp.srcPost';
    return src?.includes('{{') ? 'ed.pp.boundImage' : '';
  }

  /** The layer's text-style value for the breakpoint being edited (shown as a placeholder). */
  tsv(n: DNode, key: 'fontSize' | 'fontWeight' | 'lineHeight'): number | undefined {
    const ts = n.style?.textStyle ? this.store.textStyleMap().get(n.style.textStyle) : undefined;
    if (!ts) return undefined;
    const bp = this.store.bp();
    return (bp === 'mobile' ? ts.mobile?.[key] ?? ts.tablet?.[key] : bp === 'tablet' ? ts.tablet?.[key] : undefined) ?? ts.style[key] as number | undefined;
  }

  isFloating(n: DNode) { return this.store.floating().some(x => x.id === n.id); }

  themeColor(k: string) { return this.theme().colors[k as ThemeColorKey] ?? this.theme().palette.find(c => c.id === k)?.value ?? '#888888'; }

  setThemeColor(k: ThemeColorKey, v: string) {
    const t = structuredClone(this.theme());
    t.colors[k] = v;
    this.store.themeDraft.set(t);
  }

  setThemeBp(k: 'tablet' | 'mobile', v: string) {
    const t: Theme = structuredClone(this.theme());
    const def = k === 'mobile' ? 640 : 1024, n = Number(v);
    const bp = { ...t.breakpoints, [k]: n === def ? undefined : n };
    t.breakpoints = bp.mobile || bp.tablet ? bp : undefined;
    this.store.themeDraft.set(t);
  }

  setThemeFont(k: 'fontHeading' | 'fontBody', v: string) {
    const t: Theme = { ...structuredClone(this.theme()), [k]: v };
    this.store.themeDraft.set(t);
  }

  async saveTheme() {
    try { await this.store.saveTheme(); } catch (e) { this.store.notify(String((e as Error)?.message ?? e), true); }
  }

  /** Replaces the post list's card with a ready-made style (asks first: the old card is replaced). */
  async applyCard(n: DNode, c: (typeof CARD_STYLES)[number]) {
    if (!(await Dialog.confirm({ title: this.i18n.t('ed.pp.cardConfirm'), text: this.i18n.t('ed.pp.cardConfirmText'), confirm: this.i18n.t('ed.pp.cardApply') }))) return;
    this.store.applyCardStyle(n.id, c.card(), { ...c.layout, columns: c.layout.columns }, structuredClone(c.responsive));
  }

  async makeComponent(n: DNode) {
    const name = await Dialog.prompt({ title: this.i18n.t('ed.componentPrompt'), value: n.name || 'Component' });
    if (name) await this.store.createComponent(n.id, name);
  }

  blockTitle(n: DNode) {
    const b = this.store.blockInfo(n);
    return b ? localized(b.name) : n.props?.block ?? '';
  }

  picked(m: Media) {
    const s = this.sel();
    if (!s) return;
    if (this.pickFor() === 'file') {
      this.store.refreshFiles();
      this.store.mediaFiles.update(xs => (xs.some(x => x.id === m.id) ? xs : [...xs, m]));
      this.store.setProps(s.node.id, { mediaId: String(m.id) });
    } else if (this.pickFor() === 'src') this.store.setProps(s.node.id, { src: m.url, alt: s.node.props?.alt || m.alt });
    else this.style({ backgroundImage: m.url });
    this.pickFor.set(null);
  }
}

/** Another layer (not `except`) already using the name for code `ref`. */
function findRef(root: DNode, ref: string, except: string): DNode | null {
  if (root.id !== except && root.props?.ref === ref) return root;
  for (const c of root.children ?? []) { const f = findRef(c, ref, except); if (f) return f; }
  return null;
}
