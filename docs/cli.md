# anasaki-code — the Site Code CLI

`anasaki-code` keeps a site's **Site Code** (server-side TypeScript / JavaScript, see
[site-code.md](site-code.md)) in a local folder so you can edit it in VS Code with full types, then sync,
watch and publish from the terminal. Source: [`cli/`](../cli) (Node.js ≥ 20, zero dependencies, ESM).
[ภาษาไทย](#ภาษาไทย)

```sh
npx anasaki-code --help          # once published to npm
node cli/bin/anasaki-code.mjs    # from the CMS repo
```

## Quick start

1. Admin → **Profile → API tokens / โทเค็นสำหรับเครื่องมือ** → create a token with scope `code`. It is shown
   once (`ast_` + 40 characters).
2. ```sh
   anasaki-code login https://example.com          # paste the token (or --token ast_...)
   mkdir my-site && cd my-site
   anasaki-code init https://example.com           # anasaki.json + tsconfig.json + .anasakiignore, then pull
   code .
   anasaki-code watch                              # pushes on save, prints build problems
   anasaki-code publish -m "new checkout route"
   ```

What a token may do is decided by its user's role: `view_scripts` (pull, status, logs, types),
`edit_scripts` (push, watch), `publish_scripts` (publish). A token stops working when it is revoked, expires
or its user changes password.

## Commands

| Command | What it does |
|---|---|
| `login <site> [--token ast_...]` | Checks the token with `GET /api/v1/auth/me` and saves it in `~/.anasaki/credentials.json`, keyed by site origin (mode 0600 where supported). Prompts when `--token` is missing. |
| `logout [site]` | Removes the saved token. |
| `init [site] [--dir <dir>]` | Writes `anasaki.json`, `tsconfig.json` (VS Code), a default `.anasakiignore`, then `pull`. |
| `pull [--force]` | Downloads the server draft and `anasaki.d.ts`. Local files changed since the last sync are never overwritten — they are listed as conflicts (exit 2) unless `--force` (take the server copy). Files deleted on the server are deleted locally only when unchanged locally. |
| `push [--dry-run] [--delete]` | Uploads changed files in one atomic request (`POST /code/sync`), each with the hash it was based on. A file changed on both sides stops the push — nothing is uploaded. Locally deleted files are deleted on the server only with `--delete` (and only files synced before). `--dry-run` validates on the server without saving. |
| `status` | Three-way comparison of local files, the server draft and the last sync: modified / added / deleted locally, changed remotely, conflict, skipped. |
| `watch [--debounce ms] [--poll ms]` | Initial push, then a push 300 ms after the last change; prints build problems. **Never deletes remote files.** Recursive `fs.watch` on Windows / macOS / recent Linux, polling otherwise (`--poll` forces it). |
| `publish [-m <msg>] [--yes]` | Builds the draft (`POST /code/build`), lists pending migrations and destructive operations, asks before destructive ones (`--yes` skips the question; required when not interactive), then publishes. Warns about local changes not pushed yet. |
| `logs [--follow] [--unit <id>] [--level <l>] [--limit n]` | Prints Site Code logs oldest first; `--follow` keeps polling. `--unit` = `site` or a JS plugin id. |
| `types` | Refreshes `anasaki.d.ts` (globals, your table row types, modules of active JS plugins). |
| `--help`, `--version` | |

**Exit codes:** `0` ok · `1` error (network, authentication, usage) · `2` conflict, validation error or build
errors.

## Files

- **`anasaki.json`** (project root, no secrets): `{ "site": "https://example.com", "dir": "." }` — `dir` is
  the folder that mirrors the server draft. Commands work from any sub-folder.
- **`.anasaki/state.json`**: hashes of the last sync (tells your changes from server changes). Do not commit it
  when several people share the folder.
- **`.anasakiignore`** (paths relative to `dir`): gitignore-like — `*`, `?`, `**`, `[abc]`, `#` comments,
  `dir/` for folders, a leading `/` to anchor, `!` to re-include. Always ignored: dotfiles and dot-folders,
  `node_modules/`, `anasaki.json`, `.anasakiignore`, and `anasaki.d.ts` / `tsconfig.json` / `jsconfig.json`
  at the top of `dir`. Only `.ts .js .mjs .json .md` are synced; other files and names the server refuses
  (segments of `A-Z a-z 0-9 . _ -`) are reported as *skipped*.
- Paths use `/` on the wire; Windows paths are converted locally.
- Server limits apply: 200 files, 512 KB per file, 2 MB in total.

## Environment

| Variable | |
|---|---|
| `ANASAKI_TOKEN` | Token to use (overrides the saved one) — for CI. |
| `ANASAKI_LANG` / `LANG` | Messages are English unless the value starts with `th`. |
| `ANASAKI_CONFIG_DIR` | Where `credentials.json` lives (default `~/.anasaki`). |

## VS Code

`init` writes a `tsconfig.json` that includes `anasaki.d.ts`, so the globals and `import … from '<plugin>'`
are completed and type-checked. Build problems are printed as `path:line:column: error: message` (paths
relative to the project root) and `watch` prints `[anasaki] sync start` / `[anasaki] sync done`, so a
background task can feed the Problems panel. `.vscode/tasks.json`:

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
    }
  ]
}
```

## Conflicts

The CLI remembers what it last synced. When the same file changed locally **and** on the server (someone
saved it in the web editor), `push` uploads nothing and exits 2. Then:

1. `anasaki-code pull` — downloads everything else and lists the conflicting files (your copies are kept).
2. Keep a copy of your version of each conflicting file, `anasaki-code pull --force` to take the server
   copy, merge your changes back in.
3. `anasaki-code push`.

## API used

All requests send `Authorization: Bearer ast_…` (a personal access token with scope `code`; any other
endpoint answers 403 `{"error":"token scope"}`, a revoked / expired token 401). Details:
[site-code.md](site-code.md#personal-access-tokens-and-the-sync-api-phase-5).

| Request | Used by | Response |
|---|---|---|
| `GET /api/v1/auth/me` | login | the user (`name`, `email`, `role`, `caps`) |
| `GET /api/v1/code/tree` | pull, push, status | `{files:[{path, sha256, size, updatedAt}], liveVersion, draftChanged}` |
| `GET /api/v1/code/files/{path}` | pull | `{path, content, updatedAt}` |
| `POST /api/v1/code/sync` `{put:[{path, content, baseSha256?}], delete:[{path, baseSha256?}], dryRun?}` | push, watch | `{files, liveVersion, draftChanged, ok, problems, warnings, written, deleted, skipped, dryRun}`; 409 `{error, conflicts:[{path, serverSha256}]}` (nothing applied); `baseSha256: ""` = the file must not exist on the server |
| `GET /api/v1/code/status` | publish, status | `{liveVersion, draftChanged, …}` |
| `POST /api/v1/code/build` | publish | `{ok, errors, warnings, migrations:{pending:[{name, sql, destructive}]}}` |
| `POST /api/v1/code/publish` `{message, baseVersion, confirmDestructive}` | publish | `{version:{id, message, …}}`; 409 `{liveVersion}`; 400 `{errors}` or `{destructive}` |
| `GET /api/v1/code/logs?perPage=&unit=&level=&from=<RFC3339>` | logs | `{items:[{id, at, level, source, message, trace, unit}], total}` |
| `GET /api/v1/code/types` | init, pull, types | `anasaki.d.ts` (text/plain) |

## Development

```sh
node --test cli/test     # from the CMS repo root (fake API server, no network)
```

## ภาษาไทย

`anasaki-code` ใช้ดึงโค้ดของเว็บ (Site Code) มาแก้ในเครื่องด้วย VS Code แล้วส่งขึ้น / ดูผล / เผยแพร่จาก
terminal ต้องใช้ Node.js ≥ 20 ไม่มี dependency เพิ่ม ข้อความเป็นภาษาไทยเมื่อ `ANASAKI_LANG` หรือ `LANG`
ขึ้นต้นด้วย `th`

1. ในแอดมิน: **โปรไฟล์ → โทเค็นสำหรับเครื่องมือ** → สร้างโทเค็นขอบเขต `code` (แสดงครั้งเดียว คัดลอกเก็บไว้)
2. `anasaki-code login https://example.com` แล้ววางโทเค็น
3. `anasaki-code init https://example.com` ในโฟลเดอร์ใหม่ → เปิดด้วย VS Code
4. `anasaki-code watch` ส่งไฟล์ขึ้น draft ทุกครั้งที่บันทึก และแสดงข้อผิดพลาดของ build
5. `anasaki-code publish -m "ข้อความ"` เพื่อเผยแพร่ (ถ้ามี migration ที่ทำให้ข้อมูลหายจะถามยืนยันก่อน)

สิทธิ์ของโทเค็นเท่ากับสิทธิ์ของผู้ใช้ (`view_scripts` ดึง, `edit_scripts` ส่งขึ้น, `publish_scripts`
เผยแพร่) โทเค็นใช้ได้เฉพาะ `/api/v1/code/**` กับ `GET /api/v1/auth/me` และจะใช้ไม่ได้ทันทีเมื่อถูกยกเลิก
หมดอายุ หรือเจ้าของเปลี่ยนรหัสผ่าน ถ้าไฟล์เดียวกันถูกแก้ทั้งในเครื่องและบนเว็บ `push` จะไม่ส่งอะไรเลย
(exit 2) — ให้ `pull` แล้วรวมการแก้ไขก่อนส่งใหม่
