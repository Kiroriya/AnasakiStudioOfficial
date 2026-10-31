import { Injectable } from '@angular/core';
import type * as Monaco from 'monaco-editor';

export type MonacoApi = typeof Monaco;

interface AmdRequire {
  (deps: string[], ok: (...mods: unknown[]) => void, fail?: (e: unknown) => void): void;
  config(c: Record<string, unknown>): void;
}
type MonacoWindow = Window & { monaco?: MonacoApi; require?: AmdRequire };

/**
 * Loads the self-hosted Monaco editor (copied from node_modules/monaco-editor/min/vs to
 * assets/monaco/vs by angular.json) with its AMD loader, on first use only — the Site
 * Code screens call this; nothing of Monaco is in the app bundle (types only).
 */
@Injectable({ providedIn: 'root' })
export class MonacoLoader {
  private loading: Promise<MonacoApi> | null = null;
  private tsReady = false;

  /** Base URL of the Monaco files (follows the admin base href, e.g. /admin/). */
  static base(): string {
    return new URL('assets/monaco/vs', document.baseURI).href;
  }

  load(): Promise<MonacoApi> {
    return (this.loading ??= new Promise<MonacoApi>((resolve, reject) => {
      const w = window as MonacoWindow;
      if (w.monaco?.editor) return resolve(w.monaco);
      const base = MonacoLoader.base();
      const fail = (e: unknown) => { this.loading = null; reject(e instanceof Error ? e : new Error('Monaco failed to load')); };
      const start = () => {
        const req = w.require;
        if (!req?.config) return fail(new Error('Monaco loader missing'));
        req.config({ paths: { vs: base } });
        req(['vs/editor/editor.main'], (mod: unknown) => {
          const m = (mod as MonacoApi | undefined)?.editor ? (mod as MonacoApi) : w.monaco;
          if (m) resolve(m); else fail(new Error('Monaco missing'));
        }, fail);
      };
      if (w.require?.config) return start();
      const s = document.createElement('script');
      s.src = base + '/loader.js';
      s.async = true;
      s.onload = start;
      s.onerror = () => { s.remove(); fail(new Error('Monaco loader failed')); };
      document.head.appendChild(s);
    }));
  }

  /**
   * TypeScript/JavaScript language settings for the project (once): ES modules with Node-style
   * resolution so `import { x } from './lib/x'` finds the `file:///` models of other project
   * files; JS files are type-checked against anasaki.d.ts too.
   */
  configure(m: MonacoApi) {
    if (this.tsReady) return;
    this.tsReady = true;
    const ts = tsOf(m);
    const opts: Monaco.typescript.CompilerOptions = {
      target: ts.ScriptTarget.ESNext,
      module: ts.ModuleKind.ESNext,
      moduleResolution: ts.ModuleResolutionKind.NodeJs,
      allowJs: true,
      checkJs: false,
      allowNonTsExtensions: true,
      resolveJsonModule: true,
      esModuleInterop: true,
      noEmit: true,
      strict: false,
      baseUrl: 'file:///',
      lib: ['es2022'],
    };
    ts.typescriptDefaults.setCompilerOptions(opts);
    ts.javascriptDefaults.setCompilerOptions(opts);
    ts.typescriptDefaults.setEagerModelSync(true);
    ts.javascriptDefaults.setEagerModelSync(true);
  }

  private libs: Monaco.IDisposable[] = [];

  /** (Re)loads anasaki.d.ts as an extra lib for TS and JS (autocomplete of site/db/log/…). */
  setTypes(m: MonacoApi, dts: string) {
    // lib is es2022 only (no DOM: fetch/setTimeout do not exist in phase 1); console does.
    if (!dts.includes('console')) {
      dts += '\ndeclare const console: { log(...a: any[]): void; info(...a: any[]): void; warn(...a: any[]): void; error(...a: any[]): void; debug(...a: any[]): void };\n';
    }
    this.libs.forEach(l => l.dispose());
    const uri = 'file:///node_modules/@types/anasaki/index.d.ts';
    this.libs = [
      tsOf(m).typescriptDefaults.addExtraLib(dts, uri),
      tsOf(m).javascriptDefaults.addExtraLib(dts, uri),
    ];
  }
}

/** The TypeScript language service API (exported at the top level; older builds hung it on languages). */
function tsOf(m: MonacoApi): MonacoApi['typescript'] {
  return m.typescript ?? (m.languages as unknown as { typescript: MonacoApi['typescript'] }).typescript;
}
