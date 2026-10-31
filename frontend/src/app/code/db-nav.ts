import { ChangeDetectionStrategy, Component } from '@angular/core';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { TPipe } from '../core/i18n';
import { CodeTabs } from './code-tabs';
import './messages-db';

/** Header + Site Code tabs + Database sub-navigation shown on every /code/db screen. */
@Component({
  selector: 'rea-db-nav',
  imports: [RouterLink, RouterLinkActive, TPipe, CodeTabs],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🗄️</span>
      <div class="page-head-text"><h1>{{ 'db.title' | t }}</h1><p>{{ 'db.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    <nav class="db-nav" [attr.aria-label]="'db.title' | t">
      <a class="db-nav-link" id="db-nav-overview" routerLink="/code/db" routerLinkActive="on" [routerLinkActiveOptions]="{ exact: true }">{{ 'db.nav.overview' | t }}</a>
      <a class="db-nav-link" id="db-nav-er" routerLink="/code/db/er" routerLinkActive="on">{{ 'db.nav.er' | t }}</a>
      <a class="db-nav-link" id="db-nav-sql" routerLink="/code/db/sql" routerLinkActive="on">{{ 'db.nav.sql' | t }}</a>
      <a class="db-nav-link" id="db-nav-migrations" routerLink="/code/db/migrations" routerLinkActive="on">{{ 'db.nav.migrations' | t }}</a>
    </nav>
  `,
})
export class DbNav {}
