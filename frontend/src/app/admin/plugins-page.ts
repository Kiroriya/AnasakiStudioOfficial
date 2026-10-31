import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { InstalledPackage, PluginInfo, Reconsent, StoreCategory, StoreDetail, StoreReview, StoreSummary } from '../core/models';
import { PluginConsentDialog, PluginDepsDialog, PluginUninstallDialog, UninstallChoice, UninstallState } from './plugin-dialogs';
import {
  ConsentState, DepStatus, OptionalDepStatus, consentFor, consentNeeded, dependencyStatus, dependentsFromError, dependentsOf, destructiveFromError, isJsPlugin,
  isSensitiveScope, missingDeps, missingFromError, namesOf, normalizeRequires, optionalIcon, optionalStatus, scopeLabel, sortScopes,
} from './plugin-deps';
import './messages-plugins4b';
import { Plugins } from '../core/plugins';
import { SiteInfo } from '../core/site';
import { markdown } from '../shared/markdown';
import './messages-admin';
import './messages-platform';
import './messages-phase7';
import { SelectValue } from '../shared/select-value';
import { LegacyItem, LegacyMigrations, legacyErrorKind, legacyName } from '../core/legacy';

type Tab = 'builtin' | 'installed' | 'store' | 'upload';
type Rankings = { trending: StoreSummary[]; topRated: StoreSummary[]; topDownloads: StoreSummary[] };

/**
 * Plugins, WordPress-style: built-in features, installed packages, the Anasaki Store
 * (browse, rankings, reviews, one-click install/update) and zip upload.
 */
import { Pager } from '../shared/pager';

@Component({
  selector: 'rea-plugins-page',
  imports: [SelectValue, RouterLink, TPipe, Pager, PluginConsentDialog, PluginDepsDialog, PluginUninstallDialog],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🧩</span>
      <div class="page-head-text"><h1>{{ 'nav.plugins' | t }}</h1><p>{{ 'plugins.desc' | t }}</p></div>
      <button class="btn btn-primary" (click)="setTab('store')">🛍 {{ 'store.browse' | t }}</button>
    </header>

    <div class="tabs-row">
      @if (plugins.builtins().length) {
        <button class="tab" [class.on]="view() === 'builtin'" (click)="setTab('builtin')">⚙️ {{ 'plugins.tab.builtin' | t }} <span class="tab-count">{{ plugins.builtins().length }}</span></button>
      }
      <button class="tab" [class.on]="view() === 'installed'" (click)="setTab('installed')">📦 {{ 'plugins.tab.installed' | t }} <span class="tab-count">{{ packages().length }}</span></button>
      <button class="tab" [class.on]="view() === 'store'" (click)="setTab('store')">🛍 {{ 'plugins.tab.store' | t }}</button>
      <button class="tab" [class.on]="view() === 'upload'" (click)="setTab('upload')">⬆️ {{ 'plugins.tab.upload' | t }}</button>
    </div>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    <!-- phase 7: the former built-in plugins moved to official Store plugins -->
    @if (view() === 'installed' && legacy.summary().items.length) {
      <section id="legacy" class="card pad legacy-section">
        <h3>📦 {{ 'legacy.title' | t }}</h3>
        <p class="hint">{{ 'legacy.desc' | t }}</p>
        <ul class="legacy-list">
          @for (it of legacy.summary().items; track it.id) {
            <li class="legacy-row" [attr.data-id]="it.id" [attr.data-status]="it.status">
              <span class="legacy-name">{{ it.icon }} {{ lname(it) }}</span>
              <span class="legacy-status" [attr.data-status]="it.status">{{ 'legacy.status.' + it.status | t }}</span>
              <span class="legacy-meta">
                @if (it.version) { v{{ it.version }} · {{ 'legacy.source.' + (it.source || 'store') | t }} }
                @if (it.imported) { · {{ 'legacy.imported' | t: { n: n(it.imported) } }} }
                @if (it.status === 'done' && !it.wasActive) { · {{ 'legacy.wasOff' | t }} }
                @if (it.approvedAt) { · {{ 'legacy.approved' | t: { time: i18n.date(it.approvedAt, true) } }} }
                @if (it.status === 'failed' && it.nextAttemptAt) { · {{ 'legacy.nextAttempt' | t: { time: i18n.date(it.nextAttemptAt, true) } }} }
              </span>
              @if (it.status !== 'done') {
                <button class="btn btn-sm legacy-retry-one" [attr.data-id]="it.id" [disabled]="legacy.busy() || legacy.summary().running" (click)="legacyRetry(it)">↻ {{ 'legacy.retry' | t }}</button>
              }
              @if (it.cleanable) {
                <button class="btn btn-sm btn-danger plugin-cleanup" [attr.data-id]="it.id" [disabled]="legacy.busy()" (click)="legacyCleanup(it)">🧹 {{ 'legacy.cleanup' | t }}</button>
              } @else if (it.cleanedAt) {
                <span class="muted small">✓ {{ 'legacy.cleaned' | t }}</span>
              }
              @if (errKind(it) === 'store') {
                <span class="legacy-err">{{ 'legacy.err.store' | t }}</span>
              } @else if (errKind(it) === 'other') {
                <span class="legacy-err">{{ it.error }}</span>
              }
            </li>
          }
        </ul>
        @if (legacy.waiting().length) {
          <label class="btn btn-sm" [class.disabled]="legacy.busy()" [title]="'legacy.uploadHint' | t">⬆ {{ 'legacy.upload' | t }}
            <input type="file" accept=".zip,application/zip" hidden (change)="legacyUpload($event)" />
          </label>
        }
      </section>
    }

    @switch (view()) {
      <!-- ============================================================ built-in -->
      @case ('builtin') {
        <p class="hint section-hint">{{ 'plugins.builtinHint' | t }}</p>
        <div class="plugin-grid">
          @for (p of plugins.builtins(); track p.id) {
            <article class="card plugin-card" [class.active]="p.active">
              <div class="plugin-top">
                <span class="plugin-icon">{{ p.icon }}</span>
                <div>
                  <h3>{{ pname(p) }}</h3>
                  <p class="muted small">v{{ p.version }} · {{ p.author }}</p>
                </div>
                <span class="badge" [class.badge-live]="p.active">{{ (p.active ? 'plugins.on' : 'plugins.off') | t }}</span>
              </div>
              <p class="plugin-desc">{{ pdesc(p) }}</p>
              @if (p.active && p.menu.length) {
                <div class="plugin-links">
                  @for (m of p.menu; track m.path) {
                    @if (!m.cap || auth.can(m.cap)) { <a class="chip-link" [routerLink]="m.path">{{ m.icon }} {{ i18n.locale() === 'en' ? m.labelEn || m.label : m.label }}</a> }
                  }
                </div>
              }
              <div class="plugin-foot">
                @if (p.active && p.hasSettings) { <a class="btn" [routerLink]="['/p', p.id]">⚙️ {{ 'common.settings' | t }}</a> }
                @if (p.active) {
                  <button class="btn" [disabled]="busy() === p.id" (click)="toggle(p)">{{ 'plugins.deactivate' | t }}</button>
                } @else {
                  <button class="btn btn-primary" [disabled]="busy() === p.id" (click)="toggle(p)">{{ (busy() === p.id ? 'plugins.activating' : 'plugins.activate') | t }}</button>
                }
              </div>
            </article>
          } @empty {
            <div class="card empty-state">{{ 'plugins.empty' | t }}</div>
          }
        </div>
      }

      <!-- ============================================================ installed packages -->
      @case ('installed') {
        <p class="hint section-hint">{{ 'plugins.installedHint' | t }}</p>
        <div class="plugin-grid">
          @for (p of packages(); track p.id) {
            <article class="card plugin-card" [class.active]="p.active">
              <div class="plugin-top">
                <span class="plugin-icon">@if (p.iconUrl) { <img [src]="p.iconUrl" alt="" /> } @else { {{ p.icon || '📦' }} }</span>
                <div>
                  <h3>{{ p.name }} @if (officialIds()[p.id]) { <span class="badge badge-info plugin-official" [attr.data-id]="p.id" [title]="'store.officialHint' | t">★ {{ 'store.official' | t }}</span> }</h3>
                  <p class="muted small">v{{ p.version }} · {{ p.author }} · {{ (p.source === 'store' ? 'plugins.fromStore' : 'plugins.fromUpload') | t }}</p>
                </div>
                @if (isJs(p)) { <span class="badge badge-info plugin-runtime-badge" data-runtime="js" [title]="'pjs.runtimeHint' | t">JS</span> }
                <span class="badge" [class.badge-live]="p.active">{{ (p.active ? 'plugins.on' : 'plugins.off') | t }}</span>
              </div>
              <p class="plugin-desc">{{ p.description }}</p>
              @if (p.activationError) {
                <p class="alert plugin-activation-error" [attr.data-id]="p.id">⛔ <b>{{ 'pjs.activationError' | t }}:&nbsp;</b><span class="mono small">{{ p.activationError }}</span></p>
              }
              @if (depMap()[p.id]; as ds) {
                <div class="plugin-requires" [attr.data-id]="p.id">
                  <span class="small"><b>🔗 {{ 'pjs.requires' | t }}</b></span>
                  @for (d of ds; track d.id) {
                    <span class="plugin-require" [attr.data-dep]="d.id" [attr.data-state]="d.state">
                      <span class="chip" [class.chip-warn]="d.state !== 'ok'" [title]="depText(d)">{{ depIcon(d) }} {{ d.name || d.id }} <span class="muted">{{ d.range }}</span></span>
                      @if (d.state === 'missing') {
                        <button class="btn btn-sm plugin-require-install" [disabled]="!!busy()" (click)="fixDep(d)">⬇ {{ 'pjs.dep.install' | t }}</button>
                      } @else if (d.state === 'incompatible') {
                        <button class="btn btn-sm plugin-require-update" [disabled]="!!busy()" (click)="fixDep(d)">⬆ {{ 'pjs.dep.update' | t }}</button>
                      } @else if (d.state === 'inactive') {
                        <button class="btn btn-sm plugin-require-activate" [disabled]="!!busy()" (click)="fixDep(d)">▶ {{ 'pjs.dep.activate' | t }}</button>
                      }
                    </span>
                  }
                </div>
              }
              @if (optMap()[p.id]; as os) {
                <div class="plugin-works-with" [attr.data-id]="p.id" [title]="'pjs.worksWithHint' | t">
                  <span class="small"><b>🤝 {{ 'pjs.worksWith' | t }}</b></span>
                  @for (d of os; track d.id) {
                    <span class="chip plugin-optional" [class.chip-warn]="d.state === 'incompatible'" [attr.data-dep]="d.id" [attr.data-state]="d.state" [title]="optText(d)">{{ optIcon(d) }} {{ d.name || d.id }} <span class="muted">{{ d.range }}</span></span>
                  }
                </div>
              }
              @if (p.dependents?.length) {
                <p class="small plugin-dependents" [attr.data-id]="p.id">↩ <b>{{ 'pjs.dependents' | t }}:</b>
                  @for (x of p.dependents; track x) { <span class="chip plugin-dependent" [attr.data-dep]="x">{{ pkgName(x) }}</span> }</p>
              }
              @if (p.reconsent; as rc) {
                <p class="alert">⚠ {{ 'plugins.reconsent' | t: { v: rc.fromVersion } }}<br /><span class="small">{{ reconsentItems(rc).join(' · ') }}</span></p>
              }
              <div class="plugin-links">
                @if (p.service?.runtime === 'wasm') {
                  <span class="chip" [title]="'svc.wasmHint' | t">⚙️ {{ 'svc.chipWasm' | t }}</span>
                } @else if (p.service) {
                  <span class="chip" [class.chip-warn]="!p.service.url" [title]="'svc.chipHint' | t">🛰 {{ (p.service.url ? 'svc.chip' : 'svc.chipNotConnected') | t }}</span>
                }
                @if (p.blocks.length) { <span class="chip">🧱 {{ 'plugins.blocks' | t: { n: p.blocks.length } }}</span> }
                @for (perm of p.permissions; track perm) { <span class="chip" [title]="'perm.' + perm | t">🔐 {{ 'perm.' + perm | t }}</span> }
                @for (sc of scopes(p.code?.scopes); track sc) {
                  <span class="chip plugin-scope-chip" [attr.data-scope]="sc" [class.chip-warn]="sensitive(sc)" [title]="sc">⌨️ {{ scopeName(sc) }}</span>
                }
                @if (updates()[p.id]; as v) { <span class="badge badge-warn">⬆ {{ 'store.updateTo' | t: { v: v } }}</span> }
              </div>
              <div class="plugin-foot">
                @if (updates()[p.id]) { <button class="btn btn-primary" [disabled]="busy() === p.id" (click)="install(p.id, true)">⬆ {{ 'common.update' | t }}</button> }
                @if (p.settingsSchema.length || p.service) { <a class="btn" [routerLink]="['/packages', p.id]">⚙️ {{ 'common.settings' | t }}</a> }
                @if (p.active) {
                  <button class="btn" [disabled]="busy() === p.id" (click)="togglePkg(p)">{{ 'plugins.deactivate' | t }}</button>
                } @else {
                  <button class="btn btn-primary" [disabled]="busy() === p.id" (click)="togglePkg(p)">{{ (p.reconsent ? 'plugins.reconsentApprove' : 'plugins.activate') | t }}</button>
                  <button class="btn btn-danger" [disabled]="busy() === p.id" (click)="removePkg(p)">🗑</button>
                }
              </div>
            </article>
          } @empty {
            <div class="card empty-state plugin-empty">
              <div class="empty-icon">📦</div>
              <p>{{ 'plugins.noPackages' | t }}</p>
              <div class="row-actions">
                <button class="btn btn-primary" (click)="setTab('store')">🛍 {{ 'store.browse' | t }}</button>
                <button class="btn" (click)="setTab('upload')">⬆️ {{ 'plugins.tab.upload' | t }}</button>
              </div>
            </div>
          }
        </div>
      }

      <!-- ============================================================ store -->
      @case ('store') {
        @if (storeError()) {
          <div class="info-box warn">
            🛍 {{ 'store.offline' | t }} <span class="mono small">{{ storeError() }}</span>
            <div class="row-actions" style="margin-top: 8px">
              <button class="btn btn-sm" (click)="loadStore()">{{ 'common.refresh' | t }}</button>
            </div>
          </div>
        } @else {
          <div class="store-layout">
            <nav class="card store-cats">
              <button class="store-cat" [class.on]="!category()" (click)="setCategory('')"><span>✨</span><span class="grow">{{ 'common.all' | t }}</span></button>
              @for (c of categories(); track c.slug) {
                <button class="store-cat" [class.on]="category() === c.slug" (click)="setCategory(c.slug)">
                  <span>{{ c.icon }}</span><span class="grow">{{ i18n.locale() === 'en' ? c.nameEn || c.name : c.name }}</span><span class="muted small">{{ c.count }}</span>
                </button>
              }
            </nav>
            <div class="store-main">
              <div class="store-toolbar card">
                <input class="search" [placeholder]="'store.searchPh' | t" [value]="query()" (input)="query.set(val($event))" (keyup.enter)="search()" />
                <select [value]="sort()" (change)="sort.set(val($event)); search()">
                  @for (s of sorts; track s) { <option [value]="s">{{ 'store.sort.' + s | t }}</option> }
                </select>
                <button class="btn" (click)="search()">🔎 {{ 'common.search' | t }}</button>
              </div>

              @if (showRankings() && rankings(); as r) {
                @for (row of rankingRows; track row.key) {
                  @if (r[row.key].length) {
                    <section class="store-row">
                      <h3>{{ row.icon }} {{ 'store.rank.' + row.key | t }}</h3>
                      <div class="store-strip">
                        @for (p of r[row.key].slice(0, 6); track p.id; let i = $index) {
                          <button class="store-mini card" (click)="open(p.id)">
                            <span class="rank" [class.top]="i < 3">{{ i + 1 }}</span>
                            <span class="plugin-icon sm">@if (p.iconUrl) { <img [src]="p.iconUrl" alt="" /> } @else { {{ p.icon || '📦' }} }</span>
                            <span class="store-mini-text"><b>{{ p.name }}</b><small>{{ stars(p.rating) }} · ⬇ {{ n(p.downloads) }}</small></span>
                          </button>
                        }
                      </div>
                    </section>
                  }
                }
                <h3 class="store-all">{{ 'store.allPlugins' | t }}</h3>
              }

              <div class="plugin-grid">
                @for (p of results(); track p.id) {
                  <article class="card plugin-card store-card" (click)="open(p.id)">
                    <div class="plugin-top">
                      <span class="plugin-icon">@if (p.iconUrl) { <img [src]="p.iconUrl" alt="" /> } @else { {{ p.icon || '📦' }} }</span>
                      <div>
                        <h3>{{ p.name }} @if (p.verified) { <span class="verified" [title]="'store.verified' | t">✔</span> } @if (p.official) { <span class="badge badge-info plugin-official" [title]="'store.officialHint' | t">★ {{ 'store.official' | t }}</span> }</h3>
                        <p class="muted small">{{ 'store.by' | t: { name: p.author } }} · v{{ p.version }}</p>
                      </div>
                    </div>
                    <p class="plugin-desc">{{ p.description }}</p>
                    <div class="store-stats">
                      <span class="stars" [title]="p.rating.toFixed(1)">{{ stars(p.rating) }}</span><span class="muted small">({{ p.ratingCount }})</span>
                      <span class="grow"></span>
                      <span class="muted small" [title]="'store.downloads' | t">⬇ {{ n(p.downloads) }}</span>
                      <span class="muted small" [title]="'store.views' | t">👁 {{ n(p.views) }}</span>
                    </div>
                    <div class="plugin-foot" (click)="$event.stopPropagation()">
                      @if (p.updateAvailable) {
                        <button class="btn btn-primary" [disabled]="busy() === p.id" (click)="install(p.id, true)">⬆ {{ 'common.update' | t }}</button>
                      } @else if (p.installed) {
                        <span class="badge badge-live">✓ {{ 'store.installed' | t }}</span>
                      } @else {
                        <button class="btn btn-primary" [disabled]="busy() === p.id" (click)="install(p.id)">{{ (busy() === p.id ? 'store.installing' : 'common.install') | t }}</button>
                      }
                    </div>
                  </article>
                } @empty {
                  <div class="card empty-state">{{ 'store.noResults' | t }}</div>
                }
              </div>
              <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="go($event)" />
            </div>
          </div>
        }
      }

      <!-- ============================================================ upload -->
      @case ('upload') {
        <section class="card upload-card">
          <div class="dropzone" [class.over]="over()" (dragover)="$event.preventDefault(); over.set(true)" (dragleave)="over.set(false)" (drop)="drop($event)">
            <span class="dropzone-icon">📦</span>
            <b>{{ 'plugins.dropZip' | t }}</b>
            <span class="muted small">{{ 'plugins.dropZipHint' | t }}</span>
            <label class="btn btn-primary" style="margin-top: 12px">
              {{ (busy() === 'upload' ? 'store.installing' : 'plugins.chooseZip') | t }}
              <input type="file" accept=".zip,application/zip" hidden (change)="uploadFile($event)" />
            </label>
          </div>
          <div class="info-box">🛡 {{ 'plugins.safeInfo' | t }}</div>
          <p class="hint">{{ 'plugins.devHint' | t }} <code>docs/plugins.md</code></p>
        </section>
      }
    }

    <!-- ============================================================ store detail drawer -->
    @if (detail(); as d) {
      <div class="drawer-backdrop" (click)="detail.set(null)"></div>
      <aside class="drawer drawer-xl">
        <header class="drawer-head">
          <div class="plugin-top">
            <span class="plugin-icon">@if (d.iconUrl) { <img [src]="d.iconUrl" alt="" /> } @else { {{ d.icon || '📦' }} }</span>
            <div>
              <h3>{{ d.name }} @if (d.verified) { <span class="verified">✔</span> } @if (d.official) { <span class="badge badge-info plugin-official" id="store-detail-official" [title]="'store.officialHint' | t">★ {{ 'store.official' | t }}</span> } @if (isJs(d)) { <span class="badge badge-info plugin-runtime-badge" data-runtime="js" [title]="'pjs.runtimeHint' | t">JS</span> }</h3>
              <p class="muted small">{{ 'store.by' | t: { name: d.author } }} · v{{ d.version }} · {{ d.license }}</p>
            </div>
          </div>
          <button class="icon-btn" (click)="detail.set(null)">✕</button>
        </header>
        <div class="drawer-body">
          <div class="store-kpis">
            <div><b>{{ d.rating.toFixed(1) }}</b><small><span class="stars">{{ stars(d.rating) }}</span> {{ 'store.ratings' | t: { n: d.ratingCount } }}</small></div>
            <div><b>{{ n(d.downloads) }}</b><small>{{ 'store.downloads' | t }}</small></div>
            <div><b>{{ n(d.views) }}</b><small>{{ 'store.views' | t }}</small></div>
            <div><b>{{ catName(d.category) }}</b><small>{{ 'store.category' | t }}</small></div>
          </div>
          <div class="row-actions detail-actions">
            @if (d.updateAvailable) {
              <button class="btn btn-primary" [disabled]="busy() === d.id" (click)="install(d.id, true)">⬆ {{ 'store.updateTo' | t: { v: d.version } }}</button>
            } @else if (d.installed) {
              <span class="badge badge-live">✓ {{ 'store.installed' | t }} v{{ d.installedVersion }}</span>
            } @else {
              <button class="btn btn-primary" [disabled]="busy() === d.id" (click)="install(d.id)">{{ (busy() === d.id ? 'store.installing' : 'common.install') | t }}</button>
            }
            @if (d.homepage) { <a class="btn" [href]="d.homepage" target="_blank" rel="noopener">↗ {{ 'store.homepage' | t }}</a> }
          </div>
          @if (d.permissions.length) {
            <h4>🔐 {{ 'store.permissions' | t }}</h4>
            <ul class="perm-list">@for (p of d.permissions; track p) { <li>{{ 'perm.' + p | t }}</li> }</ul>
          }
          @if (scopes(d.code?.scopes); as scs) {
            @if (scs.length) {
              <h4>⌨️ {{ 'pjs.scopes' | t }}</h4>
              <ul class="perm-list" id="store-detail-scopes">@for (sc of scs; track sc) { <li class="store-scope" [attr.data-scope]="sc">{{ scopeName(sc) }}@if (sensitive(sc)) { <span class="muted small"> ⚠</span> }</li> }</ul>
            }
          }
          @if (detailDeps().length) {
            <h4>🔗 {{ 'pjs.requires' | t }}</h4>
            <ul class="perm-list" id="store-detail-requires">
              @for (dep of detailDeps(); track dep.id) { <li class="store-require" [attr.data-dep]="dep.id" [attr.data-state]="dep.state">{{ depIcon(dep) }} <b class="mono">{{ dep.id }}</b> <span class="muted small">{{ dep.range }} · {{ depText(dep) }}</span></li> }
            </ul>
          }
          @if (detailOpt().length) {
            <h4>🤝 {{ 'pjs.worksWith' | t }}</h4>
            <p class="hint">{{ 'pjs.worksWithHint' | t }}</p>
            <ul class="perm-list" id="store-detail-works-with">
              @for (dep of detailOpt(); track dep.id) { <li class="store-optional" [attr.data-dep]="dep.id" [attr.data-state]="dep.state">{{ optIcon(dep) }} <b class="mono">{{ dep.name || dep.id }}</b> <span class="muted small">{{ dep.range }} · {{ optText(dep) }}</span></li> }
            </ul>
          }
          @if (d.tags.length) { <div class="plugin-links">@for (tg of d.tags; track tg) { <span class="chip">#{{ tg }}</span> }</div> }

          <div class="seg detail-tabs">
            <button [class.on]="dtab() === 'about'" (click)="dtab.set('about')">{{ 'store.about' | t }}</button>
            <button [class.on]="dtab() === 'versions'" (click)="dtab.set('versions')">{{ 'store.versions' | t }} ({{ d.versions.length }})</button>
            <button [class.on]="dtab() === 'reviews'" (click)="dtab.set('reviews')">{{ 'store.reviews' | t }} ({{ d.ratingCount }})</button>
          </div>
          @switch (dtab()) {
            @case ('about') { <div class="readme rea-content" [innerHTML]="readme()"></div> }
            @case ('versions') {
              <ul class="list">
                @for (v of d.versions; track v.version) {
                  <li><span><b>v{{ v.version }}</b> <span class="muted small">· {{ i18n.date(v.createdAt) }} · {{ (v.size / 1024).toFixed(0) }} KB</span>@if (v.changelog) { <br /><span class="small">{{ v.changelog }}</span> }</span>
                    @if (v.minCore) { <span class="muted small">{{ 'store.minCore' | t: { v: v.minCore } }}</span> }</li>
                }
              </ul>
            }
            @case ('reviews') {
              <form class="review-form" (submit)="$event.preventDefault(); review()">
                <div class="star-input">
                  @for (s of [1, 2, 3, 4, 5]; track s) { <button type="button" [class.on]="myStars() >= s" (click)="myStars.set(s)" [attr.aria-label]="s">★</button> }
                </div>
                <textarea rows="2" [placeholder]="'store.reviewPh' | t" [value]="myComment()" (input)="myComment.set(val($event))"></textarea>
                <button class="btn btn-primary btn-sm" [disabled]="!myStars()">{{ 'store.sendReview' | t }}</button>
              </form>
              <ul class="review-list">
                @for (r of reviews(); track $index) {
                  <li><div><span class="stars">{{ stars(r.stars) }}</span> <b>{{ r.author }}</b> <span class="muted small">· {{ i18n.date(r.createdAt) }}</span></div><p>{{ r.comment }}</p></li>
                } @empty { <li class="muted">{{ 'store.noReviews' | t }}</li> }
              </ul>
            }
          }
        </div>
      </aside>
    }

    @for (c of consentList(); track c) { <rea-plugin-consent [state]="c" (decide)="onConsent($event)" /> }
    @for (x of depsList(); track x) { <rea-plugin-deps [name]="x.name" [deps]="x.deps" (decide)="onDeps($event)" /> }
    @for (u of uninstallList(); track u) { <rea-plugin-uninstall [state]="u" (decide)="onUninstall($event)" /> }
  `,
  styles: `
    .plugin-requires { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; margin: 6px 0; }
    .plugin-require { display: inline-flex; align-items: center; gap: 4px; }
    .plugin-works-with { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; margin: 4px 0; }
    .plugin-dependents { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; margin: 4px 0; }
    .plugin-activation-error { word-break: break-word; }
  `,
})
export class PluginsPage implements OnInit {
  readonly plugins = inject(Plugins);
  readonly auth = inject(Auth);
  private site = inject(SiteInfo);
  readonly i18n = inject(I18n);
  private api = inject(Api);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  /** Phase 7: migration of the former built-in plugins. */
  readonly legacy = inject(LegacyMigrations);

  readonly perPage = 12;
  readonly sorts = ['popular', 'trending', 'downloads', 'rating', 'new'];
  readonly rankingRows: { key: keyof Rankings; icon: string }[] = [
    { key: 'trending', icon: '🔥' }, { key: 'topRated', icon: '⭐' }, { key: 'topDownloads', icon: '⬇' },
  ];

  readonly tab = signal<Tab>('installed');
  /** The tab shown: "built-in" falls back to "installed" when this build ships no Go plugins (phase 7 moved them to the Store). */
  readonly view = computed<Tab>(() => (this.tab() === 'builtin' && !this.plugins.builtins().length ? 'installed' : this.tab()));
  readonly packages = signal<InstalledPackage[]>([]);
  readonly busy = signal<string | null>(null);
  readonly error = signal('');
  readonly over = signal(false);

  readonly categories = signal<StoreCategory[]>([]);
  readonly category = signal('');
  readonly query = signal('');
  readonly sort = signal('popular');
  readonly page = signal(1);
  readonly results = signal<StoreSummary[]>([]);
  readonly total = signal(0);
  readonly rankings = signal<Rankings | null>(null);
  readonly storeError = signal('');
  readonly updates = signal<Record<string, string>>({});
  /** Installed store packages the store marks as official (★). */
  readonly officialIds = signal<Record<string, boolean>>({});
  readonly showRankings = computed(() => !this.category() && !this.query() && this.page() === 1);

  readonly detail = signal<StoreDetail | null>(null);
  readonly dtab = signal<'about' | 'versions' | 'reviews'>('about');
  readonly reviews = signal<StoreReview[]>([]);
  readonly myStars = signal(0);
  readonly myComment = signal('');
  readonly readme = computed(() => markdown(this.detail()?.readme || this.detail()?.description || ''));

  /** Phase 4b dialogs (one at a time each; they resolve the promise of the flow that opened them). */
  readonly consent = signal<ConsentState | null>(null);
  readonly depsAsk = signal<{ name: string; deps: DepStatus[] } | null>(null);
  readonly uninstall = signal<UninstallState | null>(null);
  readonly consentList = computed(() => { const c = this.consent(); return c ? [c] : []; });
  readonly depsList = computed(() => { const c = this.depsAsk(); return c ? [c] : []; });
  readonly uninstallList = computed(() => { const c = this.uninstall(); return c ? [c] : []; });
  private consentDone?: (ok: boolean) => void;
  private depsDone?: (ok: boolean) => void;
  private uninstallDone?: (c: UninstallChoice | null) => void;

  /** Status of each plugin's `requires` against the installed packages. */
  readonly depMap = computed(() => {
    const all = this.packages(), out: Record<string, DepStatus[]> = {};
    for (const p of all) { const r = normalizeRequires(p.requires); if (r.length) out[p.id] = dependencyStatus(r, all); }
    return out;
  });
  readonly detailDeps = computed(() => dependencyStatus(normalizeRequires(this.detail()?.requires), this.packages()));
  /** Phase 6: `optionalRequires` ("works with") of each plugin — informational, never blocking. */
  readonly optMap = computed(() => {
    const all = this.packages(), out: Record<string, OptionalDepStatus[]> = {};
    for (const p of all) { const o = optionalStatus(p.optionalRequires, all); if (o.length) out[p.id] = o; }
    return out;
  });
  readonly detailOpt = computed(() => optionalStatus(this.detail()?.optionalRequires, this.packages()));

  private storeLoaded = false;
  /** Resolves once the installed-package list has been (re)loaded. */
  private packagesReady: Promise<void> = Promise.resolve();

  ngOnInit() {
    const t = this.route.snapshot.queryParamMap.get('tab') as Tab | null;
    this.plugins.load();
    this.loadPackages();
    void this.computeUpdates(); // update badges and ★ official on the Installed tab (store unreachable → none)
    void this.legacy.load();
    this.setTab(t && ['builtin', 'installed', 'store', 'upload'].includes(t) ? t : 'installed', false);
  }

  lname(it: LegacyItem) { return legacyName(it, this.i18n.locale()); }
  errKind(it: LegacyItem) { return legacyErrorKind(it); }

  async legacyRetry(it: LegacyItem) {
    try { await this.legacy.retry(it.id); this.loadPackages(); } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async legacyUpload(e: Event) {
    const input = e.target as HTMLInputElement;
    const f = input.files?.[0];
    input.value = '';
    if (!f) return;
    try { await this.legacy.upload(f); this.loadPackages(); Dialog.toast(this.i18n.t('legacy.uploaded')); } catch (err) { Dialog.error(errorMessage(err)); }
  }

  /** "Clean up old data": drops the built-in's old tables / options after a confirmation. */
  async legacyCleanup(it: LegacyItem) {
    const ok = await Dialog.confirm({ title: this.i18n.t('legacy.cleanupConfirm', { name: this.lname(it) }), text: this.i18n.t('legacy.cleanupText'), danger: true });
    if (!ok) return;
    try { await this.legacy.cleanup(it.id); Dialog.toast(this.i18n.t('legacy.cleanupDone')); } catch (e) { Dialog.error(errorMessage(e)); }
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  n(v: number) { return (v ?? 0).toLocaleString(this.i18n.locale() === 'th' ? 'th-TH' : 'en-US'); }
  stars(r: number) { const f = Math.round(r || 0); return '★'.repeat(f) + '☆'.repeat(5 - f); }
  pname(p: PluginInfo) { return this.english(p)[0] ?? p.name; }
  pdesc(p: PluginInfo) { return this.english(p)[1] ?? p.description; }
  /** English name/description in EN mode (from the server, or our own translation of built-ins). */
  private english(p: PluginInfo): [string?, string?] {
    if (this.i18n.locale() !== 'en') return [];
    if (p.nameEn) return [p.nameEn, p.descriptionEn];
    const key = `plugin.${p.id}.en`;
    const t = this.i18n.t(key);
    return t && t !== key ? (t.split('|') as [string, string]) : [];
  }
  isJs(p: { runtime?: string }) { return isJsPlugin(p); }
  scopes(s: string[] | undefined) { return sortScopes(s); }
  scopeName(s: string) { return scopeLabel(s); }
  sensitive(s: string) { return isSensitiveScope(s); }
  pkgName(id: string) { return namesOf([id], this.packages())[0]; }
  depIcon(d: DepStatus) { return d.state === 'ok' ? '✓' : d.state === 'inactive' ? '⏸' : d.state === 'incompatible' ? '⚠' : '✗'; }
  optIcon(d: OptionalDepStatus) { return optionalIcon(d); }
  optText(d: OptionalDepStatus) { return d.state === 'incompatible' ? this.i18n.t('pjs.opt.incompatible', { v: d.installedVersion ?? '?' }) : this.i18n.t('pjs.opt.' + d.state); }
  depText(d: DepStatus) { return d.state === 'incompatible' ? this.i18n.t('pjs.dep.incompatible', { v: d.installedVersion ?? '?' }) : this.i18n.t('pjs.dep.' + d.state); }

  // ---------------------------------------------------------------- dialogs (promise-based)

  askConsent(c: ConsentState): Promise<boolean> {
    this.consentDone?.(false);
    this.consent.set(c);
    return new Promise(r => (this.consentDone = r));
  }
  onConsent(ok: boolean) { const r = this.consentDone; this.consentDone = undefined; this.consent.set(null); r?.(ok); }
  askDeps(name: string, deps: DepStatus[]): Promise<boolean> {
    this.depsDone?.(false);
    this.depsAsk.set({ name, deps });
    return new Promise(r => (this.depsDone = r));
  }
  onDeps(ok: boolean) { const r = this.depsDone; this.depsDone = undefined; this.depsAsk.set(null); r?.(ok); }
  askUninstall(u: UninstallState): Promise<UninstallChoice | null> {
    this.uninstallDone?.(null);
    this.uninstall.set(u);
    return new Promise(r => (this.uninstallDone = r));
  }
  onUninstall(c: UninstallChoice | null) { const r = this.uninstallDone; this.uninstallDone = undefined; this.uninstall.set(null); r?.(c); }

  catName(slug: string) {
    const c = this.categories().find(x => x.slug === slug);
    return c ? (this.i18n.locale() === 'en' ? c.nameEn || c.name : c.name) : slug;
  }

  setTab(t: Tab, nav = true) {
    this.tab.set(t);
    this.error.set('');
    if (nav) this.router.navigate([], { queryParams: { tab: t }, replaceUrl: true });
    if ((t === 'store' || t === 'installed') && !this.storeLoaded) this.loadStore();
  }

  // ---------------------------------------------------------------- built-in

  async toggle(p: PluginInfo) {
    if (p.active && !(await Dialog.confirm({ title: this.i18n.t('plugins.confirmOff', { name: this.pname(p) }), confirm: this.i18n.t('plugins.deactivate') }))) return;
    this.busy.set(p.id);
    try {
      await this.plugins.setActive(p.id, !p.active);
      await this.auth.refresh(); // capabilities may change
      Dialog.toast(this.i18n.t(p.active ? 'plugins.offDone' : 'plugins.onDone', { name: this.pname(p) }));
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(null);
    }
  }

  // ---------------------------------------------------------------- packages

  loadPackages(): Promise<void> {
    this.packagesReady = (async () => {
      try { this.packages.set(await this.api.packages()); } catch { this.packages.set([]); }
    })();
    return this.packagesReady;
  }

  /** Human-readable list of what an upgrade asks for beyond the last approval. */
  reconsentItems(rc: Reconsent): string[] {
    return [
      ...(rc.permissions ?? []).map(x => this.i18n.t('perm.' + x)),
      ...(rc.api ?? []).map(x => this.i18n.t('svc.api.' + x)),
      ...(rc.hosts ?? []).map(h => this.i18n.t('plugins.reconsentHost', { host: h })),
      ...(rc.scopes ?? []).map(s => scopeLabel(s)),
    ];
  }

  async togglePkg(p: InstalledPackage) {
    if (p.active) await this.deactivatePkg(p);
    else await this.activatePkg(p);
  }

  /** Turning off a plugin others need turns them off too (asked first; the server answers 409 otherwise). */
  private async deactivatePkg(p: InstalledPackage) {
    const all = this.packages();
    let deps = dependentsOf(p.id, all).filter(id => all.find(x => x.id === id)?.active);
    let force = false;
    this.busy.set(p.id);
    try {
      for (let attempt = 0; attempt < 2; attempt++) {
        if (deps.length && !force) {
          this.busy.set(null);
          if (!(await Dialog.confirm({
            title: this.i18n.t('pjs.off.dependentsTitle', { name: p.name }), text: namesOf(deps, all).map(x => '• ' + x).join('\n'),
            confirm: this.i18n.t('pjs.off.force'), danger: true,
          }))) return;
          force = true;
          this.busy.set(p.id);
        }
        try {
          const u = await this.api.setPackageActive(p.id, false, force ? { force: true } : {});
          if (force) await this.loadPackages();
          else this.packages.update(xs => xs.map(x => (x.id === p.id ? u : x)));
          Dialog.toast(this.i18n.t('plugins.offDone', { name: p.name }));
          return;
        } catch (e) {
          const d = dependentsFromError(e);
          if (!d.length || force) throw e;
          deps = d;
        }
      }
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(null);
    }
  }

  /**
   * Turning a package on: re-consent when an upgrade asks for more (code scopes
   * included), then missing dependencies and destructive migrations reported by
   * the server are resolved / confirmed and the call is retried.
   */
  async activatePkg(p: InstalledPackage, depth = 0): Promise<boolean> {
    if (p.reconsent && !(await this.askConsent(consentFor('reconsent', p, p.reconsent)))) return false;
    let confirmDestructive = false;
    this.busy.set(p.id);
    try {
      for (let attempt = 0; attempt < 4; attempt++) {
        try {
          const u = await this.api.setPackageActive(p.id, true, confirmDestructive ? { confirmDestructive } : {});
          if (u && u.id === p.id) this.packages.update(xs => xs.map(x => (x.id === p.id ? u : x)));
          else await this.loadPackages();
          const now = this.packages().find(x => x.id === p.id);
          if (now?.activationError) { Dialog.error(now.activationError, this.i18n.t('pjs.activationError')); return false; }
          Dialog.toast(this.i18n.t('plugins.onDone', { name: p.name }));
          return true;
        } catch (e) {
          const miss = missingFromError(e);
          if (miss.length) {
            this.busy.set(null);
            if (!(await this.resolveDeps(p.name, dependencyStatus(miss, this.packages()), depth))) return false;
            this.busy.set(p.id);
            continue;
          }
          const destr = destructiveFromError(e);
          if (destr.length && !confirmDestructive) {
            this.busy.set(null);
            if (!(await this.askConsent(consentFor('activate', p, null, destr)))) return false;
            confirmDestructive = true;
            this.busy.set(p.id);
            continue;
          }
          throw e;
        }
      }
      return false;
    } catch (e) {
      Dialog.error(errorMessage(e));
      return false;
    } finally {
      this.busy.set(null);
    }
  }

  /** Uninstall dialog: dependents warning (they get turned off) and "delete plugin data". */
  async removePkg(p: InstalledPackage) {
    let deps = dependentsOf(p.id, this.packages());
    for (let attempt = 0; attempt < 2; attempt++) {
      const choice = await this.askUninstall({ id: p.id, name: p.name, runtime: isJsPlugin(p) ? 'js' : p.runtime, dependents: deps, dependentNames: namesOf(deps, this.packages()) });
      if (!choice) return;
      try {
        await this.api.deletePackage(p.id, { deleteData: choice.deleteData, force: deps.length > 0 });
        await this.loadPackages();
        this.refreshInstalledFlags();
        Dialog.toast(this.i18n.t(choice.deleteData ? 'pjs.del.doneData' : 'pjs.del.done'));
        return;
      } catch (e) {
        const d = dependentsFromError(e);
        if (d.length && !deps.length) { deps = d; continue; } // show who needs it and ask again
        Dialog.error(errorMessage(e));
        return;
      }
    }
  }

  /** The install / update / activate button next to a requirement on a card. */
  async fixDep(d: DepStatus) {
    const ok = await this.resolveOne(d, 0);
    await this.loadPackages();
    this.refreshInstalledFlags();
    return ok;
  }

  private resolveOne(d: DepStatus, depth: number): Promise<boolean> {
    if (d.state === 'missing') return this.install(d.id, false, depth + 1);
    if (d.state === 'incompatible') return this.install(d.id, true, depth + 1);
    if (d.state === 'inactive') {
      const p = this.packages().find(x => x.id === d.id);
      return p ? this.activatePkg(p, depth + 1) : this.install(d.id, false, depth + 1);
    }
    return Promise.resolve(true);
  }

  /** "Install all": dependencies one after another, each with its own consent; stops at the first failure. */
  async resolveDeps(name: string, deps: DepStatus[], depth: number): Promise<boolean> {
    if (depth > 5) { Dialog.error(this.i18n.t('pjs.deps.tooDeep')); return false; }
    const todo = deps.filter(d => d.state !== 'ok');
    if (!todo.length) return true;
    if (!(await this.askDeps(name, todo))) return false;
    for (const d of todo) {
      if (!(await this.resolveOne(d, depth))) {
        Dialog.toast(this.i18n.t('pjs.deps.failed', { id: d.id }), 'warning');
        await this.loadPackages();
        return false;
      }
    }
    await this.loadPackages();
    return true;
  }

  // ---------------------------------------------------------------- store

  async loadStore() {
    this.storeError.set('');
    try {
      const [cats, ranks] = await Promise.all([this.api.storeCategories(), this.api.storeRankings()]);
      this.categories.set(cats);
      this.rankings.set(ranks);
      await this.search();
      this.storeLoaded = true;
      this.computeUpdates();
    } catch (e) {
      this.storeError.set(errorMessage(e));
    }
  }

  setCategory(c: string) { this.category.set(c); this.page.set(1); this.search(); }
  go(p: number) { this.page.set(p); this.search(false); }

  async search(reset = true) {
    if (reset) this.page.set(1);
    try {
      const r = await this.api.storePlugins({ category: this.category(), search: this.query(), sort: this.sort(), page: this.page(), perPage: this.perPage });
      this.results.set(r.items);
      this.total.set(r.total);
    } catch (e) { this.storeError.set(errorMessage(e)); }
  }

  /** Update badges for installed packages: ask the store about each one. */
  private async computeUpdates() {
    await this.packagesReady; // the installed list must be known before we can compare
    const out: Record<string, string> = {};
    const official: Record<string, boolean> = {};
    await Promise.all(this.packages().filter(p => p.source === 'store').map(async p => {
      try {
        const d = await this.api.storePlugin(p.id);
        if (d.updateAvailable) out[p.id] = d.version;
        if (d.official) official[p.id] = true;
      } catch { /* removed from store */ }
    }));
    this.updates.set(out);
    this.officialIds.set(official);
  }

  private refreshInstalledFlags() {
    const map = new Map(this.packages().map(p => [p.id, p.version]));
    const mark = (s: StoreSummary): StoreSummary => ({ ...s, installed: map.has(s.id), installedVersion: map.get(s.id), updateAvailable: map.has(s.id) && map.get(s.id) !== s.version });
    this.results.update(xs => xs.map(mark));
    const r = this.rankings();
    if (r) this.rankings.set({ trending: r.trending.map(mark), topRated: r.topRated.map(mark), topDownloads: r.topDownloads.map(mark) });
    const d = this.detail();
    if (d) this.detail.set({ ...d, ...mark(d) });
  }

  async open(id: string) {
    try {
      const d = await this.api.storePlugin(id);
      this.detail.set(d);
      this.dtab.set('about');
      this.myStars.set(0);
      this.myComment.set('');
      this.api.storeReviews(id).then(r => this.reviews.set(r)).catch(() => this.reviews.set([]));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  /**
   * Store install / update. New installs show the permission + code-scope
   * consent; dependencies (from the store detail, or the server's 400
   * `missing`) are offered with "Install all"; destructive migrations (400
   * `destructive`) are confirmed in the consent dialog and the call retried.
   */
  async install(id: string, update = false, depth = 0): Promise<boolean> {
    const d = this.detail()?.id === id ? this.detail() : await this.api.storePlugin(id).catch(() => null);
    // Never install without showing the permissions the plugin asks for.
    if (!update && !d) { Dialog.error(this.i18n.t('store.detailFailed')); return false; }
    if (d) {
      const miss = missingDeps(normalizeRequires(d.requires), this.packages());
      if (miss.length && !(await this.resolveDeps(d.name, miss, depth))) return false;
    }
    let base: ConsentState | null = null;
    if (!update && d) {
      base = consentFor('install', d);
      if (consentNeeded(base) && !(await this.askConsent(base))) return false;
    }
    let confirmDestructive = false;
    this.busy.set(id);
    try {
      for (let attempt = 0; attempt < 4; attempt++) {
        try {
          const p = await this.api.installPackage(id, undefined, confirmDestructive ? { confirmDestructive } : {});
          await this.loadPackages();
          this.refreshInstalledFlags();
          this.updates.update(u => { const c = { ...u }; delete c[id]; return c; });
          this.busy.set(null);
          if (p.reconsent) await this.afterReconsentInstall(p);
          else if (p.activationError) Dialog.error(p.activationError, this.i18n.t('pjs.activationError'));
          else Dialog.toast(this.i18n.t(update ? 'store.updated' : 'store.installedMsg', { name: p.name }));
          return true;
        } catch (e) {
          const miss = missingFromError(e);
          if (miss.length) {
            this.busy.set(null);
            if (!(await this.resolveDeps(d?.name ?? id, dependencyStatus(miss, this.packages()), depth))) return false;
            this.busy.set(id);
            continue;
          }
          const destr = destructiveFromError(e);
          if (destr.length && !confirmDestructive) {
            this.busy.set(null);
            const c = base ?? consentFor(update ? 'upgrade' : 'install', d ?? { name: id });
            if (!(await this.askConsent({ ...c, mode: update ? 'upgrade' : 'install', destructive: destr }))) return false;
            confirmDestructive = true;
            this.busy.set(id);
            continue;
          }
          throw e;
        }
      }
      return false;
    } catch (e) {
      Dialog.error(errorMessage(e));
      return false;
    } finally {
      this.busy.set(null);
    }
  }

  /** An upgrade that asks for more is installed but off: offer the re-consent dialog right away. */
  private async afterReconsentInstall(p: InstalledPackage) {
    if (!(await this.activatePkg(p))) this.notifyReconsent(p);
  }

  async review() {
    const d = this.detail();
    if (!d || !this.myStars()) return;
    try {
      // Reviews are signed with the site's name, not the person's: their name stays on this server.
      // (Sent explicitly: the server would fill in the account name when it is empty.)
      const r = await this.api.storeReview(d.id, { stars: this.myStars(), comment: this.myComment(), author: this.site.title() || location.hostname });
      this.detail.set({ ...d, rating: r.rating, ratingCount: r.ratingCount });
      this.reviews.set(await this.api.storeReviews(d.id).catch(() => this.reviews()));
      this.myStars.set(0);
      this.myComment.set('');
      Dialog.toast(this.i18n.t('store.thanks'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  // ---------------------------------------------------------------- upload

  uploadFile(e: Event) {
    const f = (e.target as HTMLInputElement).files?.[0];
    (e.target as HTMLInputElement).value = '';
    if (f) this.upload(f);
  }

  drop(e: DragEvent) {
    e.preventDefault();
    this.over.set(false);
    const f = e.dataTransfer?.files?.[0];
    if (f) this.upload(f);
  }

  /** An upgrade that asks for more was installed but switched off until approved. */
  private notifyReconsent(p: InstalledPackage) {
    Dialog.alert(this.i18n.t('plugins.reconsentOff', { name: p.name, v: p.version }),
      this.reconsentItems(p.reconsent!).map(x => '• ' + x).join('\n'), 'warning');
  }

  private async upload(f: File) {
    if (!f.name.toLowerCase().endsWith('.zip')) { Dialog.error(this.i18n.t('plugins.zipOnly')); return; }
    const label = f.name.replace(/\.zip$/i, '');
    let confirmDestructive = false;
    this.busy.set('upload');
    try {
      for (let attempt = 0; attempt < 4; attempt++) {
        try {
          const p = await this.api.uploadPackage(f, confirmDestructive ? { confirmDestructive } : {});
          await this.loadPackages();
          this.busy.set(null);
          this.setTab('installed');
          if (p.reconsent) await this.afterReconsentInstall(p);
          else if (p.activationError) Dialog.error(p.activationError, this.i18n.t('pjs.activationError'));
          else Dialog.toast(this.i18n.t('store.installedMsg', { name: p.name }));
          return;
        } catch (e) {
          const miss = missingFromError(e);
          if (miss.length) {
            this.busy.set(null);
            if (!(await this.resolveDeps(label, dependencyStatus(miss, this.packages()), 0))) return;
            this.busy.set('upload');
            continue;
          }
          const destr = destructiveFromError(e);
          if (destr.length && !confirmDestructive) {
            this.busy.set(null);
            if (!(await this.askConsent(consentFor('install', { name: label }, null, destr)))) return;
            confirmDestructive = true;
            this.busy.set('upload');
            continue;
          }
          throw e;
        }
      }
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(null);
    }
  }
}
