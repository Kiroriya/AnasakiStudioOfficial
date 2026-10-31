import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { Auth } from '../core/auth';
import { TPipe } from '../core/i18n';
import './messages-code';
import './messages-access';
import './messages-code4';
import './messages-code4b';
import './messages-code6';

/** Editor / Versions / Logs / Database / Jobs / Hooks / Formatters / Settings / Access switcher shown on every Site Code screen. */
@Component({
  selector: 'rea-code-tabs',
  imports: [RouterLink, RouterLinkActive, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="code-tabs" [attr.aria-label]="'code.title' | t">
      <a class="code-tab" id="code-tab-editor" routerLink="/code" routerLinkActive="on" [routerLinkActiveOptions]="{ exact: true }">✎ {{ 'code.tab.editor' | t }}</a>
      <a class="code-tab" id="code-tab-versions" routerLink="/code/versions" routerLinkActive="on">🕘 {{ 'code.tab.versions' | t }}</a>
      <a class="code-tab" id="code-tab-logs" routerLink="/code/logs" routerLinkActive="on">📜 {{ 'code.tab.logs' | t }}</a>
      @if (canData()) { <a class="code-tab" id="code-tab-db" routerLink="/code/db" routerLinkActive="on">🗄️ {{ 'code.tab.db' | t }}</a> }
      <a class="code-tab" id="code-tab-jobs" routerLink="/code/jobs" routerLinkActive="on">⏱ {{ 'code.tab.jobs' | t }}</a>
      <a class="code-tab" id="code-tab-hooks" routerLink="/code/hooks" routerLinkActive="on">🪝 {{ 'code.tab.hooks' | t }}</a>
      <a class="code-tab" id="code-tab-formatters" routerLink="/code/formatters" routerLinkActive="on">🧮 {{ 'code.tab.formatters' | t }}</a>
      @if (canAccess()) { <a class="code-tab" id="code-tab-settings" routerLink="/code/settings" routerLinkActive="on">🔑 {{ 'code.tab.settings' | t }}</a> }
      @if (canAccess()) { <a class="code-tab" id="code-tab-access" routerLink="/code/access" routerLinkActive="on">🔐 {{ 'code.tab.access' | t }}</a> }
    </nav>
  `,
})
export class CodeTabs {
  private auth = inject(Auth);
  readonly canData = computed(() => this.auth.can('view_code_data'));
  readonly canAccess = computed(() => this.auth.can('manage_scripts'));
}
