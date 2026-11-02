# Catalog (`catalog`) — แคตตาล็อกสินค้า (example JS plugin, CMS 2.1.0)

## ภาษาไทย

ปลั๊กอินตัวอย่างที่พิสูจน์ joint ของ CMS 2.1.0 ในตัวเดียว:

- ประเภทเนื้อหา **สินค้า** (`catalog_product`, `/products/<slug>`) ฟิลด์ ราคา, SKU, **แกลเลอรี** (`gallery`),
  **สินค้าที่เกี่ยวข้อง** (`relation`), **สเปก** (`repeater`: หัวข้อ/ค่า) และสีประจำสินค้า (`color`)
- **หมวดสินค้า** (`catalog_category`, มีลำดับชั้น, `/product-category/<slug>`) และ **แบรนด์** (`catalog_brand`,
  `/brands/<slug>`) ด้วย `site.taxonomy` หลังบ้านมีเมนูจัดการ และมีช่องเลือกในหน้าแก้ไขสินค้า
- `POST /catalog/import` (ผู้ดูแลระบบ) นำเข้าสินค้าจาก JSON สร้างแบรนด์/หมวดที่ยังไม่มีด้วย `db.terms`
  แล้วผูกกับสินค้าผ่าน `terms`
- ฟีดสำหรับ headless `GET /catalog/feed.json` ต้องส่ง header `X-Api-Key` ให้ตรงกับการตั้งค่า **feedKey** ซึ่งเป็นชนิด
  `secret` (เก็บแบบเข้ารหัส หลังบ้านไม่แสดงค่าอีก)
- `GET /catalog/brands`, `GET /catalog/product/<slug>`

**สิทธิ์:** `content`, `content-write`, `routes`, `post-types`, `taxonomies`, `terms-write`

## English

Proves the CMS 2.1.0 joints: a **Products** type with gallery / relation / repeater / color fields, **Product
categories** (hierarchical) and **Brands** taxonomies with admin screens, editor panels and archive pages, a JSON
import that creates terms with `db.terms` and assigns them with `PostInput.terms`, and a headless feed
(`GET /catalog/feed.json`) guarded by the **feedKey** setting of type `secret`.

```bash
curl -H "X-Api-Key: $KEY" https://example.com/catalog/feed.json
curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"products":[{"title":"Anvil","price":990,"brand":"Acme","category":"Tools","parentCategory":"Hardware"}]}' \
  https://example.com/catalog/import
```
