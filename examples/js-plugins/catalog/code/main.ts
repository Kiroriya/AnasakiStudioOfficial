// Catalog (example JS plugin, CMS 2.1.0) — proves the 2.1.0 joints in one small plugin:
//  - site.postType with the new field types: gallery, relation, repeater, color
//  - site.taxonomy: Product categories (hierarchical) and Brands, with admin screens and archive pages
//  - db.terms + PostInput.terms: a JSON import that creates brands / categories and products
//  - a secret setting (feedKey) guarding a headless product feed
const TYPE = 'catalog_product';
const CATS = 'catalog_category';
const BRANDS = 'catalog_brand';
const ANY = { access: ['*'] };

site.postType(TYPE, {
  label: { th: 'สินค้า', en: 'Products' },
  singular: { th: 'สินค้า', en: 'Product' },
  icon: '🛍️',
  slug: 'products',
  supports: ['title', 'content', 'excerpt', 'image'],
  fields: {
    price: { type: 'number', min: 0, required: true, label: { th: 'ราคา', en: 'Price' } },
    sku: { type: 'text', max: 40, label: { th: 'รหัสสินค้า (SKU)', en: 'SKU' } },
    gallery: { type: 'gallery', max: 12, label: { th: 'แกลเลอรี', en: 'Gallery' } },
    related: { type: 'relation', postType: TYPE, multiple: true, max: 8, label: { th: 'สินค้าที่เกี่ยวข้อง', en: 'Related products' } },
    specs: {
      type: 'repeater', max: 30, label: { th: 'สเปก', en: 'Specifications' },
      fields: { name: { type: 'text', required: true, label: { th: 'หัวข้อ', en: 'Name' } }, value: { type: 'text', label: { th: 'ค่า', en: 'Value' } } },
    },
    accent: { type: 'color', label: { th: 'สีประจำสินค้า', en: 'Accent colour' } },
  },
});

site.taxonomy(CATS, {
  label: { th: 'หมวดสินค้า', en: 'Product categories' },
  singular: { th: 'หมวดสินค้า', en: 'Product category' },
  postTypes: [TYPE],
  hierarchical: true,
  slug: 'product-category',
});

site.taxonomy(BRANDS, {
  label: { th: 'แบรนด์', en: 'Brands' },
  singular: { th: 'แบรนด์', en: 'Brand' },
  postTypes: [TYPE],
  slug: 'brands',
});

const slugs = (p: Post, tax: string) => (p.terms[tax] ?? []).map((t) => t.slug);

function product(p: Post) {
  return {
    id: p.id, title: p.title, slug: p.slug, url: p.url, excerpt: p.excerpt, image: p.image,
    price: p.meta.price ?? null, currency: site.settings('currency') || 'THB', sku: p.meta.sku ?? '',
    gallery: p.meta.gallery ?? [], related: p.meta.related ?? [], specs: p.meta.specs ?? [],
    categories: slugs(p, CATS), brands: slugs(p, BRANDS), updatedAt: p.updatedAt,
  };
}

// headless feed: GET /catalog/feed.json with X-Api-Key (the secret setting)
site.route('GET /catalog/feed.json', ANY, ({ request }) => {
  const key = String(site.settings('feedKey') || '');
  if (!key) return { status: 404, body: 'feed is off' };
  const given = String(request.headers['x-api-key'] || '');
  if (!given || !crypto.timingSafeEqual(given, key)) return { status: 401, body: 'bad key' };
  const page = Math.max(1, Number(request.query.page) || 1);
  const res = db.posts.list({ type: TYPE, perPage: 50, page });
  return { total: res.total, page, items: res.items.map(product) };
});

// brands with their counts (public)
site.route('GET /catalog/brands', ANY, () =>
  db.terms.list({ taxonomy: BRANDS }).map((t) => ({ slug: t.slug, name: t.name, count: t.count })));

// one product by slug (public)
site.route('GET /catalog/product/{slug}', ANY, ({ request }) => {
  const p = db.posts.get(String(request.params.slug), { type: TYPE });
  return p ? product(p) : { status: 404, body: 'not found' };
});

/** A term of tax by name, created when missing (parent: the name of a term of the same taxonomy). */
function ensureTerm(tax: string, name: string, parent?: string): number {
  const slug = name.toLowerCase().trim().replace(/[^a-z0-9ก-๙]+/g, '-').replace(/^-+|-+$/g, '') || 'item';
  const found = db.terms.get(slug, { taxonomy: tax });
  if (found) return found.id;
  const parentId = parent ? ensureTerm(tax, parent) : 0;
  return db.terms.create({ taxonomy: tax, name, slug, parent: parentId || null }).id;
}

// import: POST /catalog/import {products: [{title, price, sku?, brand?, category?, parentCategory?, specs?}]}
site.route('POST /catalog/import', { access: ['administrator'] }, ({ request }) => {
  const body = request.json() as { products?: any[] };
  const ids: number[] = [];
  for (const it of (body.products ?? []).slice(0, 200)) {
    const terms: Record<string, number[]> = {};
    if (it.brand) terms[BRANDS] = [ensureTerm(BRANDS, String(it.brand))];
    if (it.category) terms[CATS] = [ensureTerm(CATS, String(it.category), it.parentCategory ? String(it.parentCategory) : undefined)];
    const p = db.posts.create({
      type: TYPE, title: String(it.title || 'Product'), status: 'publish', terms,
      meta: { price: Number(it.price) || 0, sku: it.sku ?? '', specs: Array.isArray(it.specs) ? it.specs : [] },
    });
    ids.push(p.id);
  }
  return { imported: ids.length, ids };
});
