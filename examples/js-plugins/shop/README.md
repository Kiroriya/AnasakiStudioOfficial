# Shop (`shop`) — ร้านค้าตัวอย่าง (example JS plugin)

## ภาษาไทย

ปลั๊กอินตัวอย่างที่ใช้ปลั๊กอิน **money** (`requires`) สร้างตาราง `p_shop_orders` (migration 001, คอลัมน์ `status`
จาก migration 002), route `GET /shop` และ `POST /shop/order` (ชำระแล้ว), collection `code:shop:products`, action
`/_a/shop:order` (สร้างออเดอร์ "รอชำระ"), filter `shop.price`, เหตุการณ์ `shop.order.paid` / `shop.order.cancelled`,
export `cartTotal()` และ **cron** `*/5 * * * *` (งาน `shop:cancelStale`) ที่ยกเลิกออเดอร์ `pending` ที่เก่ากว่า
`cancelAfterMinutes` นาที

export `listOrders`, `getOrder`, `saveOrder` ให้หน้าจัดการข้อมูลทั่วไป (ปลั๊กอิน **admin-crud**, resource `orders`)

**ใช้ร่วมกับ (`optionalRequires`): `thai` และ `promptpay`** — ไม่ต้องติดตั้งก็ใช้ shop ได้ ถ้าเปิดใช้อยู่ shop จะเรียกผ่าน
`plugins.has` / `plugins.call`: `GET /shop?order=<id>` (และคำตอบของ `POST /shop/order`) มี `words` = ยอดเป็นตัวหนังสือ
(thai) และ `promptpay` = `{ payload, svg, url }` คิวอาร์พร้อมเพย์ของร้าน (promptpay ที่ตั้ง `promptpayId` แล้ว) — ไม่มีก็เป็น
`null`. `GET /shop` บอกสถานะใน `worksWith`

**สิทธิ์:** `db` (ตาราง orders), `routes` (`/shop`), `jobs` (งานเบื้องหลังและ cron)

**การตั้งค่า (unit `shop`):** `cancelAfterMinutes` (ค่าเริ่มต้น 30)

## English

Needs the **money** plugin. Creates `p_shop_orders` (+ a `status` column in migration 002), serves `GET /shop` and
`POST /shop/order`, a `code:shop:products` collection, the `/_a/shop:order` action (a pending order), the
`shop.price` filter, the `shop.order.paid` / `shop.order.cancelled` events, exports `cartTotal()`, and runs a
**cron** every 5 minutes (job `shop:cancelStale`) that cancels pending orders older than `cancelAfterMinutes`.
It also exports `listOrders`, `getOrder`, `saveOrder` for the generic **admin-crud** screen (resource `orders`).

**Works with (`optionalRequires`): `thai` and `promptpay`** — not needed to install; when active, shop reaches them
with `plugins.has` / `plugins.call`: `GET /shop?order=<id>` (and the `POST /shop/order` answer) carry `words` (the
amount in Thai words, thai) and `promptpay` = `{ payload, svg, url }` (a PromptPay QR for the shop's `promptpayId`,
promptpay) — `null` without them. `GET /shop` reports `worksWith`.

**Scopes:** `db`, `routes`, `jobs`. **Settings:** `cancelAfterMinutes` (default 30).

## Site Code example / ตัวอย่าง

```ts
import { cartTotal } from 'shop';
site.filter('shop.price', (price) => price * 0.9);             // 10% off everything
site.on('shop.order.paid', (o) => log.info('order', o.id));
site.on('shop.order.cancelled', ({ count }) => log.warn('cancelled unpaid orders', count));
site.route('GET /total', { access: ['*'] }, () => ({ total: cartTotal() }));
```

### Combining with `thai` and `promptpay` / ใช้ร่วมกับ thai และ promptpay

Inside shop the integration is optional (`optionalRequires` + `plugins.has` / `plugins.call`, see `code/main.ts`):

```ts
const words = plugins.has('thai') ? plugins.call('thai', 'format', 'bahtText', total) : null;
const qr = plugins.has('promptpay') ? plugins.call('promptpay', 'qrFor', total, { size: 200 }) : null;
```

Site Code — which may import any active plugin — can glue them too:

```ts
import { bahtText } from 'thai';
import { payload, qrSvg } from 'promptpay';
import { cartTotal } from 'shop';

// in the designer: {{ item.price | thai:baht }} on code:shop:products, {{ page.total | thai:bahtText }}

site.page('/checkout', {
  load: () => {
    const total = cartTotal();
    return { total, words: bahtText(total), qr: qrSvg(payload({ id: '0812345678', amount: total }), { size: 220 }) };
  },
});
```
