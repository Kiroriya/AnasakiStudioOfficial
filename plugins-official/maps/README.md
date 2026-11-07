# Maps (`maps`) — แผนที่

ปลั๊กอินทางการ: บล็อก **แผนที่** สำหรับหน้าออกแบบ ใส่ที่อยู่หรือพิกัด (ละติจูด, ลองจิจูด) หรืออ่านจากช่องข้อมูลของแต่ละหน้า
(เช่น ช่อง `address` ของประกาศขายบ้านหรืออีเวนต์ — วางในแม่แบบครั้งเดียว ทุกหน้ามีแผนที่ของตัวเอง) ใช้ Google Maps โดยไม่ต้องมี API key
หรือ OpenStreetMap พร้อมปุ่มนำทาง

## English

The designer block `maps/map`: an address or "lat, lng", a post's field (props `field`), or the business address in the settings;
Google Maps (no API key) or OpenStreetMap, height, zoom and a directions link. Needs CMS 2.5.0 (iframes of trusted map hosts in
plugin blocks).
