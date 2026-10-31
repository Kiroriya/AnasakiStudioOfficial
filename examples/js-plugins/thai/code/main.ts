// The "thai" unit (code.entry). The library (code.library = code/lib/index.ts) is bundled into importers
// and runs in their context; this file runs in the plugin's own unit and needs no scopes:
//
// 1. Binding formatters (phase 6: plugins may register formatters). In expressions they are
//    namespaced: {{ item.price | thai:baht }}, {{ total | thai:bahtText }},
//    {{ post.publishedAt | thai:thaiDate:'full' }}, {{ n | thai:thaiNumber }}. The administrator may
//    enable unprefixed aliases (| baht) on the Plugins page.
// 2. One export for plugins that use thai only *optionally* (manifest "optionalRequires" +
//    plugins.has / plugins.call, e.g. the shop sample):
//      plugins.call('thai', 'format', 'bahtText', 1250.75)   // same names and arguments as the formatters
//    (It is not named like a library function on purpose: an export shadows a library function of the
//    same name for importers.)
import { formatterFns } from './lib/index';

const fns = formatterFns();
for (const [name, fn] of Object.entries(fns)) site.formatter(name, fn);

site.export('format', (name: string, value: unknown, ...args: unknown[]) => {
  const fn = fns[name];
  if (!fn) throw new Error(`thai.format: unknown formatter "${name}" (${Object.keys(fns).join(', ')})`);
  return fn(value, ...args);
});
