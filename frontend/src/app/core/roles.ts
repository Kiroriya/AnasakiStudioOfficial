import { Locale, tr } from './i18n';
import { CapInfo, RoleInfo } from './models';

/** English names the built-in roles are created with; any other name was set by the site owner. */
export const BUILTIN_ROLE_NAMES: Record<string, string> = {
  administrator: 'Administrator', editor: 'Editor', author: 'Author', contributor: 'Contributor', subscriber: 'Subscriber',
};

/** Display name of a role: a name given by the owner wins, else the translated built-in name. */
export function roleName(slug: string, roles: RoleInfo[] = []): string {
  const own = roles.find(r => r.slug === slug)?.name;
  if (own && own !== BUILTIN_ROLE_NAMES[slug]) return own;
  const key = `role.${slug}`;
  const t = tr(key);
  return t !== key ? t : (own ?? slug);
}

/** Readable label for a capability key. */
export function capLabel(key: string): string {
  const t = tr(`cap.${key}`);
  return t !== `cap.${key}` ? t : key.replace(/_/g, ' ');
}

/** A section of the roles editor: a core group, or one installed plugin's capabilities (CMS 2.0.6). */
export interface CapSection {
  /** "content", "code", … or "plugin:<id>". */
  id: string;
  /** Core group key (translated with capgroup.<group>); "" for a plugin section. */
  group: string;
  plugin?: string;
  pluginName?: string;
  pluginIcon?: string;
  caps: CapInfo[];
}

/**
 * Groups the capability catalog for the roles editor: core groups in catalog order, then one section per
 * plugin (sorted by plugin name). Plugin capabilities without plugin info stay in the "plugins" group.
 */
export function capSections(caps: CapInfo[]): CapSection[] {
  const core: CapSection[] = [];
  const plugins = new Map<string, CapSection>();
  for (const c of caps) {
    if (c.plugin) {
      let s = plugins.get(c.plugin);
      if (!s) plugins.set(c.plugin, (s = { id: `plugin:${c.plugin}`, group: '', plugin: c.plugin, pluginName: c.pluginName || c.plugin, pluginIcon: c.pluginIcon || '', caps: [] }));
      s.caps.push(c);
      continue;
    }
    let g = core.find(x => x.group === c.group);
    if (!g) core.push((g = { id: c.group, group: c.group, caps: [] }));
    g.caps.push(c);
  }
  const ps = [...plugins.values()].sort((a, b) => (a.pluginName ?? '').localeCompare(b.pluginName ?? '') || (a.plugin ?? '').localeCompare(b.plugin ?? ''));
  return [...core, ...ps];
}

/** Text of a th/en manifest label in locale (falls back to the other language). */
export function i18nText(l: { th?: string; en?: string } | undefined, locale: Locale): string {
  if (!l) return '';
  return (locale === 'en' ? l.en || l.th : l.th || l.en) ?? '';
}

/** Label of a catalog entry: the plugin's own label, else the core translation. */
export function capInfoLabel(c: CapInfo, locale: Locale): string {
  return i18nText(c.label, locale) || capLabel(c.key);
}
