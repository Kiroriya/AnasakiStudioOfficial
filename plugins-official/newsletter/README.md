# Newsletter (`newsletter`) — จดหมายข่าว

ปลั๊กอินทางการ: บล็อก **ฟอร์มรับข่าวสาร** (และ `POST /newsletter/subscribe`), ยืนยันอีเมล (double opt-in), ลิงก์ยกเลิกในทุกฉบับ
(หน้ายืนยันก่อนยกเลิก), หน้า **จดหมายข่าว** ในหลังบ้าน (ผู้รับข่าว: ค้นหา เพิ่ม ลบ ส่งออก CSV — แคมเปญ: เขียน ส่งทดสอบ ส่งทั้งหมด
ดูความคืบหน้า) ส่งทยอยด้วยงานเบื้องหลังตามค่า **ส่งไม่เกินกี่ฉบับต่อชั่วโมง** เพื่อไม่ใช้โควตาอีเมลของเว็บจนหมด

คู่มือ: [/docs/store/plugin-newsletter](https://anasaki.live/docs/store/plugin-newsletter)

## English

A sign-up block with double opt-in, an unsubscribe link in every mail, subscriber import / CSV export, and plain-text
campaigns sent by a background job paced by the `perHour` setting (the site's mail quota is shared). Events
`newsletter.subscribed`, `newsletter.unsubscribed`, `newsletter.sent`; export `subscribe(email, name)`; tables
`p_newsletter_subscribers`, `p_newsletter_campaigns`; capability `newsletter.send`.
