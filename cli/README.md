# anasaki-code

Command-line tool for **Anasaki Studio Site Code**: keep the site's server-side TypeScript/JavaScript in a
local folder, edit it in VS Code (with `anasaki.d.ts` types), and sync / publish from the terminal.

Node.js ≥ 20, zero dependencies. · [ภาษาไทยด้านล่าง](#ภาษาไทย)

## Install

```sh
npx anasaki-code --help          # once published
# or, from the CMS repo
node cli/bin/anasaki-code.mjs --help
```

## Quick start

1. In the admin: **Profile → API tokens** → create a token with scope `code` (it is shown once).
2. ```sh
   anasaki-code login https://example.com          # paste the token (or --token ast_...)
   mkdir my-site && cd my-site
   anasaki-code init https://example.com           # anasaki.json + tsconfig.json + .anasakiignore, then pull
   code .
   anasaki-code watch                              # pushes on save, prints build problems
   anasaki-code publish -m "new checkout route"
   ```

## Commands

| Command | What it does |
|---|---|
| `login <site> [--token ast_...]` | Verifies the token with `GET /api/v1/auth/me` and saves it in `~/.anasaki/credentials.json` (keyed by site origin, file mode 0600 where supported). Prompts when `--token` is missing. |
| `logout [site]` | Removes the saved token. |
| `init [site] [--dir <dir>]` | Writes `anasaki.json`, `tsconfig.json` (for VS Code), a default `.anasakiignore`, then runs `pull`. |
| `pull [--force]` | Downloads the server draft and `anasaki.d.ts`. Files you changed locally since the last sync are **never overwritten** — they are listed as conflicts (exit 2) unless `--force` (take the server copy). Files deleted on the server are deleted locally only if you did not change them. |
| `push [--dry-run] [--delete]` | Uploads changed files in one atomic request, each with the hash it was based on. A file changed on both sides stops the push (nothing is uploaded). Files deleted locally are deleted on the server **only with `--delete`** (and only files the CLI has synced before). `--dry-run` asks the server to validate without saving. |
| `status` | Three-way comparison of local files, the server draft and the last sync: modified / added / deleted (local), remote modified / added / deleted, conflict, and skipped files. |
| `watch [--debounce ms] [--poll ms]` | Initial push, then a push 300 ms after the last change. Prints build problems. **Never deletes remote files.** Uses recursive `fs.watch` (Windows/macOS, recent Linux), otherwise polls (`--poll` forces polling). |
| `publish [-m <msg>] [--yes]` | Checks the draft (`POST /code/build`), lists pending database migrations and destructive operations, asks for confirmation before destructive ones (`--yes` to skip; required when not interactive), then publishes. Warns when local changes are not pushed yet. |
| `logs [--follow] [--unit <id>] [--level <l>] [--limit n]` | Prints Site Code logs oldest first; `--follow` keeps polling. |
| `types` | Refreshes `anasaki.d.ts`. |
| `--help`, `--version` | |

**Exit codes:** `0` ok · `1` error (network, auth, usage) · `2` conflict, validation error or build errors.

## Files

- `anasaki.json` (project root, no secrets): `{ "site": "https://example.com", "dir": "." }` — `dir` is the
  folder that mirrors the server draft. Commands work from any sub-folder.
- `.anasaki/state.json`: the hashes of the last sync (used to tell your changes from server changes). Do not
  commit it if several people share the folder.
- `.anasakiignore` (project root; paths are relative to `dir`): gitignore-like — globs (`*`, `?`, `**`,
  `[abc]`), `#` comments, `dir/` for folders, a leading `/` to anchor, `!` to re-include. Always ignored:
  dotfiles and dot-folders, `node_modules/`, `anasaki.json`, `.anasakiignore`, and `anasaki.d.ts` /
  `tsconfig.json` / `jsconfig.json` at the top of `dir`. Only `.ts .js .mjs .json .md` are synced; other files
  and names the server does not allow (`A-Z a-z 0-9 . _ -` per segment) are reported as *skipped*.
- Paths always use `/` on the wire; Windows paths are converted locally.

## Environment

| Variable | |
|---|---|
| `ANASAKI_TOKEN` | Token to use (overrides the saved one) — handy in CI. |
| `ANASAKI_LANG` / `LANG` | Messages are English unless the value starts with `th`. |
| `ANASAKI_CONFIG_DIR` | Where `credentials.json` lives (default `~/.anasaki`). |

## VS Code problem matcher

Build problems are printed as `path:line:column: error: message` with paths relative to the project root, and
`watch` prints the stable markers `[anasaki] sync start` / `[anasaki] sync done`. `.vscode/tasks.json`:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "anasaki: watch",
      "type": "shell",
      "command": "npx anasaki-code watch",
      "isBackground": true,
      "problemMatcher": {
        "owner": "anasaki",
        "fileLocation": ["relative", "${workspaceFolder}"],
        "pattern": {
          "regexp": "^(.+?):(\\d+):(\\d+): (error|warning): (.*)$",
          "file": 1, "line": 2, "column": 3, "severity": 4, "message": 5
        },
        "background": {
          "activeBegin": "^\\[anasaki\\] sync start",
          "endsPattern": "^\\[anasaki\\] sync done"
        }
      }
    },
    {
      "label": "anasaki: push",
      "type": "shell",
      "command": "npx anasaki-code push",
      "problemMatcher": {
        "owner": "anasaki",
        "fileLocation": ["relative", "${workspaceFolder}"],
        "pattern": {
          "regexp": "^(.+?):(\\d+):(\\d+): (error|warning): (.*)$",
          "file": 1, "line": 2, "column": 3, "severity": 4, "message": 5
        }
      }
    }
  ]
}
```

## Conflicts

The CLI remembers what it last synced. If the same file changed locally **and** on the server (someone saved
it in the web editor), `push` uploads nothing and exits 2. Then:

1. `anasaki-code pull` — downloads everything else and lists the conflicting files (your copies are kept).
2. Back up your version of each conflicting file, run `anasaki-code pull --force` to take the server copy,
   merge your changes back in.
3. `anasaki-code push`.

## API used

`GET /api/v1/auth/me` · `GET /api/v1/code/tree` · `GET /api/v1/code/files/{path}` · `POST /api/v1/code/sync`
· `GET /api/v1/code/status` · `POST /api/v1/code/build` · `POST /api/v1/code/publish` · `GET /api/v1/code/logs`
· `GET /api/v1/code/types` — all with `Authorization: Bearer ast_...`.

## Development

```sh
node --test cli/test     # from the CMS repo root (uses a fake API server, no network)
```

---

## ภาษาไทย

เครื่องมือบรรทัดคำสั่งสำหรับ **Site Code ของ Anasaki Studio**: เก็บโค้ดฝั่งเซิร์ฟเวอร์ (TypeScript/JavaScript)
ของเว็บไว้ในโฟลเดอร์ในเครื่อง แก้ใน VS Code (มี type จาก `anasaki.d.ts`) แล้วซิงก์/เผยแพร่จากเทอร์มินัล
ต้องใช้ Node.js ≥ 20 ไม่มี dependency เพิ่ม ข้อความจะเป็นภาษาไทยเมื่อ `ANASAKI_LANG` หรือ `LANG` ขึ้นต้นด้วย `th`
(เช่น `ANASAKI_LANG=th`)

### เริ่มต้นใช้งาน

1. ในหน้าแอดมิน: **โปรไฟล์ → โทเค็นสำหรับเครื่องมือ** → สร้างโทเค็น scope `code` (แสดงครั้งเดียว)
2. ```sh
   anasaki-code login https://example.com          # วางโทเค็น (หรือใช้ --token ast_...)
   mkdir my-site && cd my-site
   anasaki-code init https://example.com           # สร้าง anasaki.json + tsconfig.json + .anasakiignore แล้ว pull
   anasaki-code watch                              # push ทุกครั้งที่บันทึก และแสดงข้อผิดพลาดของ build
   anasaki-code publish -m "เพิ่มหน้าชำระเงิน"
   ```

### คำสั่ง

| คำสั่ง | ทำอะไร |
|---|---|
| `login <site> [--token ast_...]` | ตรวจโทเค็นกับ `GET /api/v1/auth/me` แล้วบันทึกที่ `~/.anasaki/credentials.json` (แยกตามเว็บ, สิทธิ์ไฟล์ 0600 เมื่อระบบรองรับ) ถ้าไม่ใส่ `--token` จะถาม |
| `logout [site]` | ลบโทเค็นที่บันทึกไว้ |
| `init [site] [--dir <dir>]` | สร้าง `anasaki.json`, `tsconfig.json` (สำหรับ VS Code), `.anasakiignore` แล้ว `pull` |
| `pull [--force]` | ดาวน์โหลดฉบับร่างจากเซิร์ฟเวอร์และ `anasaki.d.ts` **ไม่เขียนทับ** ไฟล์ที่คุณแก้ในเครื่องหลังซิงก์ครั้งล่าสุด (แจ้งว่าชนกัน, รหัสออก 2) เว้นแต่ใช้ `--force` (ใช้ฉบับบนเซิร์ฟเวอร์) |
| `push [--dry-run] [--delete]` | อัปโหลดไฟล์ที่แก้ในคำขอเดียว (ทั้งหมดหรือไม่มีเลย) พร้อม hash ที่ใช้เป็นฐาน ไฟล์ที่ถูกแก้ทั้งสองฝั่งจะหยุดการ push ไฟล์ที่ลบในเครื่องจะถูกลบบนเซิร์ฟเวอร์ **เฉพาะเมื่อใช้ `--delete`** `--dry-run` ให้เซิร์ฟเวอร์ตรวจโดยไม่บันทึก |
| `status` | เทียบไฟล์ในเครื่อง / ฉบับร่างบนเซิร์ฟเวอร์ / ซิงก์ครั้งล่าสุด: แก้ไข / เพิ่มใหม่ / ลบ, แก้/เพิ่ม/ลบบนเซิร์ฟเวอร์, ชนกัน และไฟล์ที่ข้าม |
| `watch` | push ครั้งแรก แล้ว push หลังไฟล์เปลี่ยน 300 ms แสดงข้อผิดพลาดของ build **ไม่ลบไฟล์บนเซิร์ฟเวอร์เด็ดขาด** |
| `publish [-m <ข้อความ>] [--yes]` | ตรวจฉบับร่าง แสดง migration ที่รอรันและรายการที่ทำให้ข้อมูลหาย ถามยืนยันก่อน (ข้ามด้วย `--yes`; ต้องใช้เมื่อไม่ได้รันแบบโต้ตอบ) แล้วเผยแพร่ |
| `logs [--follow] [--unit <id>] [--level <l>]` | ดูบันทึก (เก่าไปใหม่) `--follow` ดึงต่อเนื่อง |
| `types` | ดาวน์โหลด `anasaki.d.ts` ใหม่ |

**รหัสออก:** `0` สำเร็จ · `1` ผิดพลาด (เครือข่าย, สิทธิ์, ใช้คำสั่งผิด) · `2` ชนกัน / ข้อมูลไม่ถูกต้อง / build มีข้อผิดพลาด

### ไฟล์

- `anasaki.json` (ไม่มีความลับ): `{ "site": "https://example.com", "dir": "." }` — `dir` คือโฟลเดอร์ที่ตรงกับฉบับร่างบนเซิร์ฟเวอร์
- `.anasaki/state.json`: hash ของการซิงก์ครั้งล่าสุด ใช้แยกว่าใครแก้อะไร
- `.anasakiignore`: แบบเดียวกับ `.gitignore` (glob, `#` คอมเมนต์, `โฟลเดอร์/`, `/` นำหน้า, `!` ยกเว้น) ไฟล์ที่ข้ามเสมอ:
  ไฟล์/โฟลเดอร์ที่ขึ้นต้นด้วยจุด, `node_modules/`, `anasaki.json`, `.anasakiignore`, `anasaki.d.ts` / `tsconfig.json`
  ที่ระดับบนสุดของ `dir` ซิงก์เฉพาะ `.ts .js .mjs .json .md` ไฟล์อื่นแสดงเป็น "ข้าม"
- path ที่ส่งไปเซิร์ฟเวอร์ใช้ `/` เสมอ (แปลงจาก path ของ Windows ให้อัตโนมัติ)

### ตัวแปรสภาพแวดล้อม

`ANASAKI_TOKEN` (ใช้แทนโทเค็นที่บันทึก เหมาะกับ CI), `ANASAKI_LANG=th` (ข้อความภาษาไทย),
`ANASAKI_CONFIG_DIR` (ที่เก็บ `credentials.json`, ค่าเริ่มต้น `~/.anasaki`)

### VS Code

ข้อผิดพลาดของ build แสดงเป็น `path:line:column: error: message` (path นับจากโฟลเดอร์โปรเจกต์) ใช้
`.vscode/tasks.json` ตามตัวอย่างด้านบน ("VS Code problem matcher") ได้เลย — ข้อความ `[anasaki] sync start` /
`[anasaki] sync done` เป็นภาษาอังกฤษเสมอเพื่อให้ problem matcher ทำงานทุกภาษา

### เมื่อไฟล์ชนกัน

ถ้าไฟล์เดียวกันถูกแก้ทั้งในเครื่องและบนเซิร์ฟเวอร์ (เช่นมีคนบันทึกในตัวแก้ไขบนเว็บ) `push` จะไม่อัปโหลดอะไรเลย
และออกด้วยรหัส 2 ให้ทำดังนี้:

1. `anasaki-code pull` — ดึงไฟล์อื่น ๆ และแสดงรายชื่อไฟล์ที่ชนกัน (ฉบับของคุณยังอยู่)
2. สำรองฉบับของคุณ แล้ว `anasaki-code pull --force` เพื่อใช้ฉบับบนเซิร์ฟเวอร์ จากนั้นรวมการแก้ของคุณกลับเข้าไป
3. `anasaki-code push`

## License

MIT
