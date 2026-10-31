import { tr } from './i18n';
import { RoleInfo } from './models';

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
