/**
 * Which components are the site header and footer.
 *
 * The site role comes first: a `siteRole` field on the component as the server
 * sends it, or the `site_header_component` / `site_footer_component` settings.
 * Older data has neither — there the component's name decides, the way it
 * always did ("ส่วนหัวเว็บ" / "header", "ส่วนท้ายเว็บ" / "footer").
 */
export type SiteRole = 'header' | 'footer';

/** Names that meant "site header/footer" before components had a role. */
export const SITE_ROLE_NAMES: Record<SiteRole, string[]> = { header: ['ส่วนหัวเว็บ', 'header'], footer: ['ส่วนท้ายเว็บ', 'footer'] };
/** Settings that may hold the header/footer component id. */
export const SITE_ROLE_SETTINGS: Record<SiteRole, string> = { header: 'site_header_component', footer: 'site_footer_component' };

interface RoleComponent { id: number; name: string; siteRole?: string | null }

export function siteRoleIds(components: readonly RoleComponent[], settings: Record<string, string | undefined> = {}): Record<SiteRole, number | null> {
  const pick = (role: SiteRole): number | null => {
    const set = Number(settings[SITE_ROLE_SETTINGS[role]]);
    if (set > 0 && components.some(c => c.id === set)) return set;
    const tagged = components.find(c => c.siteRole === role);
    if (tagged) return tagged.id;
    return components.find(c => SITE_ROLE_NAMES[role].includes(c.name.trim().toLowerCase()))?.id ?? null;
  };
  return { header: pick('header'), footer: pick('footer') };
}

export function siteRoleOf(c: RoleComponent, components: readonly RoleComponent[], settings?: Record<string, string | undefined>): SiteRole | null {
  const r = siteRoleIds(components, settings);
  return c.id === r.header ? 'header' : c.id === r.footer ? 'footer' : null;
}
