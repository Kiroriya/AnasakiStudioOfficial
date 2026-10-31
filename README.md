# Anasaki Studio

[![Docker image](https://img.shields.io/docker/image-size/chichat/anasaki-studio/latest?label=docker%20image&logo=docker)](https://hub.docker.com/r/chichat/anasaki-studio)
[![Docker pulls](https://img.shields.io/docker/pulls/chichat/anasaki-studio?logo=docker)](https://hub.docker.com/r/chichat/anasaki-studio)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**[ภาษาไทย](#ภาษาไทย) · [English](#english)**

> [!IMPORTANT]
> **Repository นี้เป็นซอร์สโค้ดสำหรับศึกษา — build/รันไม่ได้โดยตรง**
> โค้ดส่วนที่ทำหน้าที่ด้านความปลอดภัยถูกนำออก: การยืนยันตัวตนและสิทธิ์ (`backend/internal/auth`), โทเค็น API
> (`api/tokens.go`, `store/tokens.go`), ตัวกรอง HTML (`backend/internal/kses`, `design/strip.go`), ตัวกันการเรียก
> เครือข่ายภายใน (`plugin/netguard.go`), การเข้ารหัสค่าลับของ Site Code (`sitecode/secrets.go`) รวมถึงเทสต์ที่เกี่ยวข้อง
> และ CI — ส่วนอื่น (ตัวออกแบบหน้าเว็บ, ตัวเรนเดอร์, Site Code engine, ระบบปลั๊กอิน, ปลั๊กอินตัวอย่างและปลั๊กอินทางการ,
> CLI, SDK, เอกสาร) อยู่ครบสำหรับอ่านและเรียนรู้ ถ้าต้องการใช้งานจริง ให้ใช้ Docker image ทางการตามหัวข้อ "ติดตั้ง" ด้านล่าง
>
> **This repository is source code for learning — it does not build or run as is.** The security code was removed:
> authentication and permissions (`backend/internal/auth`), API tokens (`api/tokens.go`, `store/tokens.go`), the HTML
> sanitiser (`backend/internal/kses`, `design/strip.go`), the internal-network guard (`plugin/netguard.go`), Site Code
> secret encryption (`sitecode/secrets.go`), their tests and CI. Everything else (designer, renderer, Site Code engine,
> plugin system, sample and official plugins, CLI, SDK, docs) is here to read and learn from. To run Anasaki Studio, use
> the official Docker image described under "Install" below.

---

## ภาษาไทย

**ระบบสร้างและจัดการเว็บไซต์แบบโอเพนซอร์ส (MIT)** — จัดการเนื้อหา ออกแบบหน้าเว็บได้อิสระ และเพิ่มความสามารถด้วยปลั๊กอิน ในที่เดียว
สร้างด้วย **Angular 22**, **Go** และ **MySQL 8**

- 🎨 **ออกแบบได้อิสระแบบ Figma** — ลากวาง, Auto layout, คอลัมน์, ส่วนที่ใช้ซ้ำ, ปรับแยกคอมพิวเตอร์/แท็บเล็ต/มือถือ; หน้าเว็บจริงแสดงผลตรงกับที่ออกแบบทุกพิกเซล
- 🖌 **ธีมทั้งเว็บ** — ตั้งสีและฟอนต์ครั้งเดียว ทุกหน้าเปลี่ยนตาม · ข้อความหลายสี/ตัวหนา/ลิงก์ในบรรทัดเดียว · ประวัติเวอร์ชันของทุกหน้า
- 📝 **จัดการเนื้อหา** — บทความพร้อม rich-text editor (หัวข้อ สี ตาราง วิดีโอ YouTube รูปพร้อมคำบรรยาย), ปักหมุดและจัดลำดับเอง, หมวดหมู่, เมนู, คลังไฟล์ (ดึงรูปจากลิงก์ได้)
- 👥 **ผู้ใช้และสิทธิ์ละเอียด** — ตำแหน่งมาตรฐาน + สร้างเอง, ระงับบัญชี, ลืมรหัสผ่าน, อีเมลต้อนรับ
- 🧩 **ปลั๊กอินในตัว** — สถิติผู้เข้าชม (ไม่ใช้คุกกี้), SEO + sitemap, แบบฟอร์ม, Redirects + 404 log, Cookie consent (PDPA), โหมดปิดปรับปรุง, Custom code, สำรองข้อมูล
- 🛍 **Anasaki Plugin Store** — ติดตั้งปลั๊กอินเพิ่มได้คลิกเดียว; ปลั๊กอินที่มีโค้ดฝั่งเซิร์ฟเวอร์ (เช่น ร้านค้า) รันในกล่องปิด (WebAssembly) ภายในระบบ
- ✉️ **ระบบอีเมลในตัว** (SMTP: Gmail, Microsoft 365, SendGrid ฯลฯ) · 🌏 หน้าจัดการภาษาไทย / English

### ติดตั้ง (Docker)

ต้องมี Docker และ Docker Compose — ใช้แค่ 2 ไฟล์ [`docker-compose.yml`](docker-compose.yml) และ [`.env.example`](.env.example) ไม่ต้องดาวน์โหลดซอร์สโค้ด

```bash
mkdir anasaki-studio && cd anasaki-studio
curl -fsSLO https://raw.githubusercontent.com/chichat/AnasakiStudio/main/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/chichat/AnasakiStudio/main/.env.example -o .env
mkdir -p docker/mysql-init && curl -fsSL https://raw.githubusercontent.com/chichat/AnasakiStudio/main/docker/mysql-init/10-site-code.sh -o docker/mysql-init/10-site-code.sh
nano .env              # ใส่ JWT_SECRET, MYSQL_PASSWORD, MYSQL_ROOT_PASSWORD, CODE_DB_PASSWORD (สุ่มด้วย: openssl rand -hex 32)
docker compose up -d
```

เปิด `http://localhost:8090/admin/` → ระบบพาเข้า **หน้าตั้งค่าเริ่มต้น** (เลือกภาษา → ชื่อเว็บ → สร้างบัญชีผู้ดูแล → ติดตั้ง) ใช้เวลาไม่ถึง 1 นาที
เว็บไซต์จริงอยู่ที่ `http://localhost:8090/`

- image สำเร็จรูป [`chichat/anasaki-studio`](https://hub.docker.com/r/chichat/anasaki-studio) มีทั้ง `linux/amd64` และ `linux/arm64` (VPS ทั่วไป, Raspberry Pi 4/5, Mac M-series); แท็ก `latest`, `1.2.3`, `1.2`, `1`
- อัปเดต: `docker compose pull && docker compose up -d` · ล็อกเวอร์ชัน: `STUDIO_IMAGE=chichat/anasaki-studio:1.0.0`
- ใช้งานจริงบนอินเทอร์เน็ต: วาง reverse proxy ที่ทำ HTTPS (Caddy, Nginx, Cloudflare) ไว้หน้าพอร์ต 8090

#### ตัวแปรในไฟล์ `.env`

| ตัวแปร | ค่าเริ่มต้น | หน้าที่ |
|---|---|---|
| `JWT_SECRET` | — (**ต้องตั้ง**) | กุญแจเซ็น session ของหน้าจัดการ (เปลี่ยนภายหลัง = ทุกคนต้อง login ใหม่) |
| `MYSQL_PASSWORD` / `MYSQL_ROOT_PASSWORD` | `change-me` | รหัสผ่านฐานข้อมูล (ตั้งก่อนเริ่มครั้งแรก — เปลี่ยนภายหลังไม่มีผล) |
| `CODE_DB_PASSWORD` | — (**ต้องตั้ง**) | รหัสผ่านของผู้ใช้ MySQL `anasaki_code` — ฐานข้อมูลของ Site Code ที่แตะตารางของ CMS ไม่ได้ (สร้างโดย `docker/mysql-init/10-site-code.sh` ตอนเริ่มครั้งแรก; เว็บที่ติดตั้งก่อนหน้านี้ดู [docs/site-code.md](docs/site-code.md)) |
| `SITE_CODE_SECRET_KEY` | — (ว่าง = สร้างไฟล์ `site-code.key` ใน `PLUGINS_DIR`) | กุญแจเข้ารหัสค่าลับของ Site Code (AES-256-GCM) — เก็บไว้ให้ดี ถ้าหายค่าลับที่บันทึกไว้จะอ่านไม่ได้ |
| `COMPOSE_PROJECT_NAME` | `anasaki-studio` | ชื่อของการติดตั้งนี้ ใช้ตั้งชื่อ container และ volume ข้อมูล |
| `PORT` | `8090` | พอร์ตที่เปิดบนเครื่อง |
| `STUDIO_IMAGE` | `chichat/anasaki-studio:latest` | image ที่ใช้ (ล็อกเวอร์ชัน หรือใช้ image จาก GHCR) |
| `ADMIN_PATH` | (ว่าง) | กู้คืนที่อยู่หน้าจัดการ ถ้าลืม path ที่ตั้งไว้ใน ตั้งค่า → ความปลอดภัย |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD` | (ว่าง) | ตั้งทั้งคู่ = สร้างผู้ดูแลให้และข้ามหน้าตั้งค่าเริ่มต้น |
| `SITE_URL` | (ว่าง) | ที่อยู่จริงของเว็บ เช่น `https://example.com` ใช้สร้างลิงก์ในอีเมล (ตั้งรหัสผ่านใหม่ / ต้อนรับ) — ควรตั้งเสมอ |
| `TRUST_PROXY` | `false` | ตั้ง `true` เมื่อมี reverse proxy อยู่หน้าพอร์ต 8090 เพื่อใช้ IP ผู้ใช้จริงจาก `X-Real-IP` / `X-Forwarded-For` (ห้ามเปิดถ้าพอร์ตเปิดตรงสู่อินเทอร์เน็ต) |

ข้อมูลทั้งหมดอยู่ใน Docker volumes (`mysql-data`, `uploads`, `plugins`) — สำรองจาก volume หรือจากปลั๊กอิน "สำรองข้อมูล" ในหน้าจัดการ

#### เปิดหลายเว็บบนเครื่องเดียว (เช่น เว็บจริง + เว็บ demo)

แยกโฟลเดอร์ละเว็บ และใน `.env` ของแต่ละโฟลเดอร์ต้องตั้ง **`COMPOSE_PROJECT_NAME` และ `PORT` ไม่ซ้ำกัน**

```bash
# โฟลเดอร์ demo/.env
COMPOSE_PROJECT_NAME=anasaki-demo
PORT=8091
```

> ⚠️ ถ้าชื่อซ้ำกัน `docker compose up` ในโฟลเดอร์ที่สองจะไปแทนที่ container ของเว็บแรกและใช้ฐานข้อมูลเดียวกัน
> อย่าเปลี่ยน `COMPOSE_PROJECT_NAME` ของเว็บที่ใช้งานอยู่ — ข้อมูลอยู่ใน volume ที่ตั้งชื่อตามค่านี้

#### เว็บทดลอง (demo) ให้คนทั่วไปเข้ามาลองเล่น

ตั้งใน `.env` ของเว็บทดลอง: `DEMO_MODE=true`, `DEMO_EMAIL`, `DEMO_PASSWORD`, `DEMO_RESET_MINUTES=60`
- หน้า login แสดงบัญชีทดลองพร้อมปุ่มเข้าสู่ระบบ — ทุกคนใช้บัญชีเดียวกัน
- ผู้ทดลองแก้เนื้อหา หน้าเว็บ ดีไซน์ เมนู หมวดหมู่ ธีม และอัปโหลดรูปเล็ก ๆ ได้ — แต่จัดการผู้ใช้ การตั้งค่าระบบ ปลั๊กอิน โค้ด HTML ดิบ อีเมลและรหัสผ่านของบัญชีไม่ได้
- ทุก `DEMO_RESET_MINUTES` นาที ทุกอย่างกลับเป็นค่าเริ่มต้น (สถานะตอนเปิดโหมดครั้งแรก — ผู้ดูแลบันทึกใหม่ได้จากแถบด้านบนของหน้าจัดการ)
- ใช้บัญชีผู้ดูแลของคุณเองแยกต่างหาก และอย่าเปิดโหมดนี้กับเว็บจริง

### ปลั๊กอิน

| แบบ | คืออะไร | ติดตั้ง |
|---|---|---|
| **แพ็กเกจ** | `.zip` ที่มี `manifest.json`, template, CSS/JS — เพิ่มบล็อก, หน้าตั้งค่า, webhook ไม่มีโค้ดฝั่งเซิร์ฟเวอร์ | คลิกเดียว |
| **มีโค้ดฝั่งเซิร์ฟเวอร์ (WASM)** | แพ็กเกจที่มี `plugin.wasm` — หน้าเว็บของตัวเอง, หน้าจัดการ, บล็อก, ข้อมูล, เหตุการณ์ รันในกล่องปิดภายใน CMS | คลิกเดียว |
| **โปรแกรมแยก (service)** | โค้ดเดียวกันแต่รันเป็นอีก container — สำหรับงานหนักหรือใช้ฐานข้อมูลของตัวเอง | รัน container เองแล้วเชื่อมต่อในหน้าตั้งค่าปลั๊กอิน |

คู่มือ: [docs/plugins.md](docs/plugins.md) · SDK (Go): [sdk/go](sdk/go) · ตัวอย่าง: [examples/plugins](examples/plugins), [examples/service-plugins/hello-service](examples/service-plugins/hello-service)

### พัฒนา (development)

```bash
git clone https://github.com/chichat/AnasakiStudio.git && cd AnasakiStudio

# MySQL อย่างเดียว เปิดที่ 127.0.0.1:3306 (ตั้ง MYSQL_PORT ได้) user/รหัสผ่าน anasaki/anasaki
MYSQL_PASSWORD=anasaki MYSQL_ROOT_PASSWORD=root CODE_DB_PASSWORD=anasaki_code JWT_SECRET=dev docker compose -f docker-compose.yml -f docker-compose.build.yml up -d mysql
cd backend && JWT_SECRET=$(openssl rand -hex 32) go run ./cmd/server   # API + เว็บไซต์ ที่ :8090
cd frontend && npm install && npm start          # หน้าจัดการ (hot reload) ที่ :4200

# หรือ build image จากซอร์สโค้ดแล้วรันทั้งระบบ
JWT_SECRET=dev docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

ทดสอบ: `cd backend && go test ./...` (ต่อฐานข้อมูลจริงด้วย `TEST_DB_DSN`) · `cd frontend && npm test && npx ng build` (unit test รันด้วย Vitest + jsdom ไม่ต้องใช้เบราว์เซอร์)

> `backend/internal/design/css.go` (`Decls`) และ `frontend/src/app/editor/design-css.ts` (`nodeCss`) ต้องแก้ให้ตรงกันเสมอ เพื่อให้หน้าออกแบบแสดงผลตรงกับเว็บจริง

### เวอร์ชัน

เลขเวอร์ชันอยู่ในไฟล์ [`VERSION`](VERSION) — Docker image ทางการ (`chichat/anasaki-studio:<เวอร์ชัน>`) build และปล่อยจาก
repository หลักของทีม (repository นี้ไม่มี CI เพราะเป็นซอร์สสำหรับศึกษา) · เลขนี้แสดงในหน้าจัดการและใช้เทียบ `minCore` ของปลั๊กอิน

---

## English

**Open-source website builder and CMS (MIT)** — content management, a free-form visual designer and plugins in one place.
Built with **Angular 22**, **Go** and **MySQL 8**.

- 🎨 **Figma-like designer** — drag and drop, auto layout, grid columns, components, desktop/tablet/mobile overrides; the live site matches the canvas pixel for pixel
- 🖌 **Site-wide theme** — set colors and fonts once, every page follows · mixed colors/bold/links inside one text · version history for every page
- 📝 **Content** — rich-text posts (headings, colors, tables, YouTube, captioned images), pinning and manual order, categories, menus, media library (import images by URL)
- 👥 **Users and fine-grained roles** — built-in and custom roles, suspension, password reset, welcome emails
- 🧩 **Official plugins** from the Plugin Store — cookie-less analytics, SEO + sitemap, forms, redirects + 404 log, cookie consent (PDPA/GDPR), maintenance mode, custom code (offered by the setup wizard; upgraded sites are migrated automatically, see [docs/plugins.md §5](docs/plugins.md))
- 💾 **Export / import site** — the whole site (content, files, installed plugins list, Site Code draft) as one zip
- 🛍 **Anasaki Plugin Store** — one-click installs; plugins with server code (e.g. a shop) run sandboxed (WebAssembly) inside the CMS
- ✉️ **Built-in email** (SMTP: Gmail, Microsoft 365, SendGrid…) · 🌏 Thai / English admin per account

### Install (Docker)

Requires Docker and Docker Compose — you only need [`docker-compose.yml`](docker-compose.yml) and [`.env.example`](.env.example), no source code.

```bash
mkdir anasaki-studio && cd anasaki-studio
curl -fsSLO https://raw.githubusercontent.com/chichat/AnasakiStudio/main/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/chichat/AnasakiStudio/main/.env.example -o .env
mkdir -p docker/mysql-init && curl -fsSL https://raw.githubusercontent.com/chichat/AnasakiStudio/main/docker/mysql-init/10-site-code.sh -o docker/mysql-init/10-site-code.sh
nano .env              # set JWT_SECRET, MYSQL_PASSWORD, MYSQL_ROOT_PASSWORD, CODE_DB_PASSWORD (generate: openssl rand -hex 32)
docker compose up -d
```

Open `http://localhost:8090/admin/` → the **first-run setup** (language → site name → admin account → install) takes under a minute.
The website is at `http://localhost:8090/`.

- Prebuilt image [`chichat/anasaki-studio`](https://hub.docker.com/r/chichat/anasaki-studio) for `linux/amd64` and `linux/arm64` (VPS, Raspberry Pi 4/5, Apple silicon); tags `latest`, `1.2.3`, `1.2`, `1`
- Update: `docker compose pull && docker compose up -d` · pin a version: `STUDIO_IMAGE=chichat/anasaki-studio:1.0.0`
- In production put an HTTPS reverse proxy (Caddy, Nginx, Cloudflare) in front of port 8090

#### `.env` settings

| Variable | Default | Purpose |
|---|---|---|
| `JWT_SECRET` | — (**required**) | signs admin sessions (changing it signs everyone out) |
| `MYSQL_PASSWORD` / `MYSQL_ROOT_PASSWORD` | `change-me` | database passwords (set before the first start; later changes have no effect) |
| `CODE_DB_PASSWORD` | — (**required**) | password of the `anasaki_code` MySQL user: Site Code's own database, with no access to the CMS tables (created by `docker/mysql-init/10-site-code.sh` on the first start; existing sites: see [docs/site-code.md](docs/site-code.md)) |
| `SITE_CODE_SECRET_KEY` | — (empty = a `site-code.key` file in `PLUGINS_DIR`) | key that encrypts Site Code secrets (AES-256-GCM) — keep it: without it stored secrets cannot be read |
| `COMPOSE_PROJECT_NAME` | `anasaki-studio` | name of this installation; containers and data volumes are named after it |
| `PORT` | `8090` | port opened on the host |
| `STUDIO_IMAGE` | `chichat/anasaki-studio:latest` | image to run (pin a version, or use GHCR) |
| `ADMIN_PATH` | (empty) | recover the admin URL if you forgot the path set in Settings → Security |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD` | (empty) | both set = create the admin automatically and skip the setup screen |
| `SITE_URL` | (empty) | public address of the site, e.g. `https://example.com`, used for links in emails (password reset / welcome) — always set it |
| `TRUST_PROXY` | `false` | `true` when a reverse proxy sits in front of port 8090: the client IP is taken from `X-Real-IP` / `X-Forwarded-For` (never enable it when the port is reachable directly) |

All data lives in Docker volumes (`mysql-data`, `uploads`, `plugins`) — back up the volumes or use **Settings → Export / import site**.

#### Several sites on one server (e.g. production + demo)

One folder per site, and in each folder's `.env` a **different `COMPOSE_PROJECT_NAME` and `PORT`**:

```bash
# demo/.env
COMPOSE_PROJECT_NAME=anasaki-demo
PORT=8091
```

> ⚠️ With the same name, `docker compose up` in the second folder replaces the first site's containers and shares its database.
> Never change `COMPOSE_PROJECT_NAME` of a running site — its data is in volumes named after it.

#### Public demo site

In the demo site's `.env`: `DEMO_MODE=true`, `DEMO_EMAIL`, `DEMO_PASSWORD`, `DEMO_RESET_MINUTES=60`
- the login page shows the shared visitor account with a sign-in button
- visitors can edit content, pages, designs, menus, categories and the theme, and upload small images — not users, settings, plugins, raw HTML, or the account's e-mail/password
- every `DEMO_RESET_MINUTES` everything goes back to the baseline (the state when demo mode was first enabled; an administrator can save a new one from the admin banner)
- keep your own administrator account private, and never enable this on a real site

### Plugins

| Kind | What | Install |
|---|---|---|
| **Package** | a `.zip` with `manifest.json`, templates, CSS/JS — designer blocks, settings, webhooks; no server code | one click |
| **Server code (WASM)** | a package with `plugin.wasm` — its own pages, admin screens, blocks, data and events, sandboxed inside the CMS | one click |
| **Separate service** | the same code running as its own container — for heavy work or its own database | run the container and connect it in the plugin's settings |

Guide: [docs/plugins.md](docs/plugins.md) · Go SDK: [sdk/go](sdk/go) · Examples: [examples/plugins](examples/plugins), [examples/service-plugins/hello-service](examples/service-plugins/hello-service)

### Development

```bash
git clone https://github.com/chichat/AnasakiStudio.git && cd AnasakiStudio

# MySQL only, on 127.0.0.1:3306 (MYSQL_PORT to change) — user/password anasaki/anasaki
MYSQL_PASSWORD=anasaki MYSQL_ROOT_PASSWORD=root CODE_DB_PASSWORD=anasaki_code JWT_SECRET=dev docker compose -f docker-compose.yml -f docker-compose.build.yml up -d mysql
cd backend && JWT_SECRET=$(openssl rand -hex 32) go run ./cmd/server   # API + website on :8090
cd frontend && npm install && npm start          # admin (hot reload) on :4200

# or build the image from source and run everything
JWT_SECRET=dev docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

| Backend variable | Default |
|---|---|
| `ADDR` | `:8090` |
| `DB_DSN` | `anasaki:anasaki@tcp(127.0.0.1:3306)/anasaki?parseTime=true&charset=utf8mb4&loc=UTC` |
| `CODE_DB_DSN` | (empty = Site Code fallback mode) — e.g. `anasaki_code:anasaki_code@tcp(127.0.0.1:3306)/anasaki_code?parseTime=true&charset=utf8mb4&loc=UTC` |
| `UPLOAD_DIR` / `PLUGINS_DIR` | `./uploads` / `./plugins-data` |
| `ADMIN_DIR` | `../frontend/dist/anasaki-studio/browser` |
| `STORE_URL` | (empty = the official Anasaki Plugin Store) — for testing against a local store |

Tests: `cd backend && go test ./...` (database tests need `TEST_DB_DSN`) · `cd frontend && npm test && npx ng build` (unit tests run in Vitest + jsdom, no browser needed)

> Keep `backend/internal/design/css.go` (`Decls`) and `frontend/src/app/editor/design-css.ts` (`nodeCss`) in sync so the designer canvas matches the live site.

### Version

The version is in the [`VERSION`](VERSION) file. The official Docker image (`chichat/anasaki-studio:<version>`) is built and
released from the team's main repository (this repository has no CI — it is source for learning). The version shows in
the admin and is what plugins' `minCore` is checked against.

### Project layout

```
backend/
  cmd/server          entrypoint (routes, admin SPA, uploads, admin path)
  internal/design     node model, CSS generator (Decls), HTML renderer, rich text
  internal/site       public site: templates, bindings, theme, favicon
  internal/api        REST API (setup, posts, media, users/roles, settings, mail, plugins, service API)
  internal/plugin     plugin runtime: built-ins, packages, service + WASM plugins, blocks, hooks, store client
  plugins-official/   the official JS plugins (seo, forms, redirects, analytics, cookie-consent, maintenance, custom-code)
  internal/legacy     readers of the former built-ins' data + the upgrade migration task
  internal/backup     Export / import site (core tool)
  internal/mail       SMTP + HTML email template
  internal/store      MySQL repository · internal/auth  bcrypt, JWT, capabilities
frontend/src/app/
  admin/  editor/  plugins/  core/  shared/
sdk/go/               Go SDK for plugins with server code
examples/             example plugins (packages and a service/WASM plugin)
docs/                 plugins.md, API.md
```

### License

[MIT](LICENSE) © 2026 Anasaki — free to use, modify and use commercially (ใช้ แก้ไข และนำไปใช้เชิงพาณิชย์ได้อิสระ).
The Anasaki Plugin Store service and the Anasaki name/logo are not covered by this license.
