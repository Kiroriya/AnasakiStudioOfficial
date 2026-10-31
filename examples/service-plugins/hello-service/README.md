# Hello Service — ตัวอย่างปลั๊กอินที่มีโค้ดฝั่งเซิร์ฟเวอร์

โค้ดชุดเดียว build ได้ 2 แบบ (ดู `docs/plugins.md` §2):

- **WASM (แนะนำ)** — ไฟล์ `plugin.wasm` อยู่ใน zip ติดตั้งแล้วใช้ได้เลย รันในกล่องปิดภายใน CMS
- **โปรแกรมแยก** — รันเป็นอีก container แล้วเชื่อมต่อที่หน้าตั้งค่าปลั๊กอิน

ใช้ทุกส่วนของระบบ:

- หน้าเว็บ `/hello` — สร้างโดยโปรแกรมปลั๊กอิน แต่ใช้ส่วนหัว/ท้ายของเว็บ และอ่านบทความล่าสุดผ่าน API ของ CMS
- บล็อก "คำทักทาย" ในหน้าออกแบบ — โปรแกรมปลั๊กอินเป็นคน render
- แหล่งข้อมูล `quotes` สำหรับบล็อกรายการข้อมูล (`{{item.title}}`, `{{item.text}}`)
- หน้าจัดการ "Hello Service" ในเมนูหลังบ้าน — แสดงเหตุการณ์ที่ได้รับ และปุ่มส่งเหตุการณ์กลับเข้า CMS
- รับเหตุการณ์ `post.published`, `user.created`, `media.uploaded` แล้วเก็บไว้ด้วยที่เก็บข้อมูลของ CMS (scope `storage`)

## ลองแบบ WASM (ติดตั้งแล้วใช้ได้เลย)

```sh
cd service
GOOS=wasip1 GOARCH=wasm go build -trimpath -ldflags="-s -w" -o ../package/plugin.wasm .
cd ../package && zip ../hello-service.zip manifest.json plugin.wasm
```

อัปโหลด `hello-service.zip` ที่ **ปลั๊กอิน → อัปโหลดปลั๊กอิน** → **เปิดใช้งาน** → เปิด `/hello` บนหน้าเว็บ, เมนู **Hello Service**
ในหลังบ้าน และลากบล็อก "คำทักทาย" ในหน้าออกแบบ

## ลองแบบโปรแกรมแยก

ลบ `"runtime"` และ `"module"` ออกจาก `manifest.json` แล้ว:

### โครงสร้าง

```
package/manifest.json   ← zip ไฟล์นี้แล้วอัปโหลดที่ ปลั๊กอิน → อัปโหลด (หรือส่งขึ้น Plugin Store)
service/main.go         ← โปรแกรมปลั๊กอิน (ใช้ SDK: sdk/go/anasaki)
Dockerfile              ← build image ของโปรแกรมปลั๊กอิน
```

### ขั้นตอน

1. zip โฟลเดอร์ `package/` แล้วอัปโหลดที่ **ปลั๊กอิน → อัปโหลดปลั๊กอิน**
2. เปิด **ตั้งค่า** ของปลั๊กอิน แล้วคัดลอก **รหัสลับ (secret)**
3. รันโปรแกรมปลั๊กอิน
   - แบบ Go: `cd service && ANASAKI_PLUGIN_ID=hello-service ANASAKI_PLUGIN_SECRET=<secret> ANASAKI_URL=http://localhost:8090 go run .`
   - แบบ Docker (รันจากโฟลเดอร์ `cms/`):
     ```
     docker build -f examples/service-plugins/hello-service/Dockerfile -t hello-service .
     ```
     แล้วเพิ่มเข้า docker-compose.yml ของ CMS ตามตัวอย่างใน `docs/plugins.md` §2.2
4. ใส่ที่อยู่ เช่น `http://localhost:9300` (หรือ `http://hello-service:9300` ใน compose) → กด **ทดสอบการเชื่อมต่อ** → **เปิดใช้งาน**
5. เปิด `/hello` บนหน้าเว็บ, เมนู **Hello Service** ในหลังบ้าน และลากบล็อก "คำทักทาย" ในหน้าออกแบบ
