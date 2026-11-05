# Shop (`eshop`) — ร้านค้าออนไลน์พร้อมเพย์

ปลั๊กอินทางการ: สินค้า (ประเภทเนื้อหา `eshop_product` ที่ `/product/<slug>`), ตะกร้าในคุกกี้, `/cart`, `/checkout`,
หน้าคำสั่งซื้อ `/shop/order/<รหัส>` พร้อม QR พร้อมเพย์ตามยอด (EMVCo + CRC16, ตัวเข้ารหัส QR ในตัว), โอนเงิน หรือเก็บเงินปลายทาง,
แนบสลิป (ไฟล์ส่วนตัว), หน้า **คำสั่งซื้อ** ในหลังบ้าน, การ์ดแดชบอร์ด, อีเมลถึงลูกค้าและร้าน, ยกเลิกคำสั่งซื้อที่ไม่ชำระอัตโนมัติ (คืนสต็อก)

คู่มือ: [/docs/store/plugin-shop](https://anasaki.live/docs/store/plugin-shop)

## English

Official plugin: products (content type `eshop_product` at `/product/<slug>`), a cookie cart, `/cart`, `/checkout`, the order page
`/shop/order/<code>` with a PromptPay QR for the exact amount (built-in EMVCo payload and QR encoder), bank transfer or cash on
delivery, slip upload (private files), the **Orders** admin page, a dashboard card, mails to the customer and the shop, and an
hourly job that cancels unpaid orders and returns their stock.

- Designer blocks: `eshop/add-to-cart`, `eshop/cart-link`, `eshop/price`; collection `code:eshop:products`
- Events: `eshop.order.created`, `eshop.order.paid`, `eshop.order.status` · export `orderTotal(lines)`
- Tables: `p_eshop_orders`, `p_eshop_items` · capabilities `eshop.orders`, `eshop.products`
- Scopes: db, routes, mail, admin-pages, post-types, content, content-write (stock), files (slips), jobs, filters-core
  (the price and button under a product's description)
