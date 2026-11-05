# Blog Extras (`blog-extras`) — ความคิดเห็นและส่วนเสริมของบทความ

ปลั๊กอินทางการ: ความคิดเห็นใต้บทความ (ตอบกลับได้ 1 ชั้น, รออนุมัติหรือแสดงทันที, กันสแปมด้วย honeypot และจำกัด 3 ครั้ง/นาที/IP,
อีเมลแจ้ง), สารบัญอัตโนมัติ (หัวข้อ h2/h3 ตั้งแต่ 3 ข้อ), เวลาอ่าน (นับอักษรไทยและคำภาษาอังกฤษ), ปุ่มแชร์ Facebook / X / LINE /
คัดลอกลิงก์, บทความที่เกี่ยวข้องจากหมวดเดียวกัน และกล่องผู้เขียน — เปิดปิดแต่ละอย่างได้ในการตั้งค่า

ใส่ให้ทุกบทความ (ประเภท `post`) ผ่าน filter `post.content` จึงใช้กับแม่แบบบทความเริ่มต้นได้ทันที และมีบล็อก **ความคิดเห็น**
**ปุ่มแชร์** **บทความที่เกี่ยวข้อง** สำหรับแม่แบบที่ออกแบบเอง หน้า **ความคิดเห็น** ในหลังบ้านใช้อนุมัติ ซ่อน ทำเครื่องหมายสแปม และลบ

## English

Comments (one reply level, moderation or instant, honeypot + 3/min/IP rate limit, email notice), an automatic table of
contents (3+ h2/h3), reading time (Thai characters and English words), share buttons (Facebook, X, LINE, copy link),
related posts from the same category and an author box — each one a setting. Added to every post through the
`post.content` filter; designer blocks `blog-extras/comments`, `share`, `related` for custom templates.
Events `blog-extras.comment.created`, `blog-extras.comment.approved`; table `p_blog_extras_comments`; capability
`blog-extras.moderate`.
