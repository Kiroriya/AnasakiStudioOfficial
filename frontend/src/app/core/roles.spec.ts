import { CapInfo } from './models';
import { capInfoLabel, capSections, i18nText } from './roles';

// The roles editor (Users → Roles): core groups, then one section per installed plugin (CMS 2.0.6).
describe('roles editor: capability sections', () => {
  const caps: CapInfo[] = [
    { key: 'read', group: 'content' },
    { key: 'edit_pages', group: 'design' },
    { key: 'publish_posts', group: 'content' },
    { key: 'seo.manage', group: 'plugins', plugin: 'seo', pluginName: 'SEO', pluginIcon: '🔍', label: { th: 'จัดการ SEO', en: 'Manage SEO' } },
    { key: 'analytics.view', group: 'plugins', plugin: 'analytics', pluginName: 'Analytics', label: { th: 'ดูสถิติ', en: 'View analytics' }, description: { th: 'เปิดหน้าสถิติ', en: 'Open the page' } },
    { key: 'seo.edit_meta', group: 'plugins', plugin: 'seo', pluginName: 'SEO', label: { en: 'Edit per-page SEO' } },
    { key: 'legacy_grant', group: 'plugins' },
  ];

  it('keeps core groups in catalog order and adds one section per plugin, sorted by name', () => {
    const s = capSections(caps);
    expect(s.map(x => x.id)).toEqual(['content', 'design', 'plugins', 'plugin:analytics', 'plugin:seo']);
    expect(s[0].caps.map(c => c.key)).toEqual(['read', 'publish_posts']);
    const seo = s.find(x => x.id === 'plugin:seo')!;
    expect(seo.plugin).toBe('seo');
    expect(seo.pluginName).toBe('SEO');
    expect(seo.pluginIcon).toBe('🔍');
    expect(seo.caps.map(c => c.key)).toEqual(['seo.manage', 'seo.edit_meta']);
    // a plugin-group capability without plugin info stays a core "plugins" group entry
    expect(s.find(x => x.id === 'plugins')!.caps.map(c => c.key)).toEqual(['legacy_grant']);
  });

  it('no plugins → core groups only', () => {
    expect(capSections(caps.filter(c => !c.plugin)).every(x => !x.plugin)).toBe(true);
    expect(capSections([])).toEqual([]);
  });

  it('labels: the manifest th/en label, falling back to the other language, else the core translation', () => {
    expect(capInfoLabel(caps[3], 'th')).toBe('จัดการ SEO');
    expect(capInfoLabel(caps[3], 'en')).toBe('Manage SEO');
    expect(capInfoLabel(caps[5], 'th')).toBe('Edit per-page SEO');
    expect(i18nText(caps[4].description, 'en')).toBe('Open the page');
    expect(i18nText(undefined, 'en')).toBe('');
    expect(capInfoLabel({ key: 'some_cap', group: 'x' }, 'en')).toBe('some cap');
  });
});
