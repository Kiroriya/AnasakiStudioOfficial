// Messages: English by default, Thai when ANASAKI_LANG / LANG (or LC_ALL /
// LC_MESSAGES) starts with "th".

export function detectLang(env = {}) {
  const v = env.ANASAKI_LANG || env.LC_ALL || env.LC_MESSAGES || env.LANG || '';
  return /^th/i.test(v) ? 'th' : 'en';
}

const en = {
  // generic
  'err.generic': 'error: {msg}',
  'err.network': 'cannot reach {site}: {msg}',
  'err.unauthorized': 'the server refused the token (401): {msg}\n  Run: anasaki-code login {site}',
  'err.forbidden': 'not allowed (403): {msg}\n  The token or your role lacks the capability for this command.',
  'err.http': 'server answered {status}: {msg}',
  'err.noProject': 'no anasaki.json found in {cwd} or its parents.\n  Run: anasaki-code init <site>',
  'err.badConfig': 'anasaki.json is invalid: {msg}',
  'err.noToken': 'not logged in to {site}.\n  Run: anasaki-code login {site}   (or set ANASAKI_TOKEN)',
  'err.unknownCommand': 'unknown command: {cmd}\n  Run: anasaki-code --help',
  'err.unknownOption': 'unknown option: {opt}',
  'err.missingValue': 'option {opt} needs a value',
  'err.needSite': 'which site? Usage: anasaki-code {cmd} <site>',
  'err.badSite': 'not a valid site URL: {site}',
  'err.emptyToken': 'no token given',
  'err.tokenFormat': 'warning: tokens normally start with "ast_"',
  'err.notTTY': 'confirmation needed but stdin is not interactive — re-run with --yes to confirm',

  // login
  'login.prompt': 'Paste the API token for {site} (Profile → API tokens): ',
  'login.ok': 'Logged in to {site} as {user}.',
  'login.saved': 'Token saved in {file}',
  'login.envNote': 'note: ANASAKI_TOKEN is set and takes precedence over the saved token.',
  'logout.ok': 'Removed the saved token for {site}.',
  'logout.none': 'No saved token for {site}.',

  // init
  'init.exists': 'anasaki.json already exists in {dir} — keeping it.',
  'init.wrote': 'Wrote {file}',
  'init.kept': 'Kept existing {file}',
  'init.done': 'Ready. Edit files, then: anasaki-code push   (or: anasaki-code watch)',

  // status
  'status.server': 'Server {site}: live version {live}{draft}',
  'status.noLive': 'none',
  'status.draftChanged': ', draft has unpublished changes',
  'status.clean': 'Everything in sync ({n} files).',
  'status.kind.modified': 'modified',
  'status.kind.added': 'added',
  'status.kind.deleted': 'deleted',
  'status.kind.remote-modified': 'remote modified',
  'status.kind.remote-added': 'remote added',
  'status.kind.remote-deleted': 'remote deleted',
  'status.kind.conflict': 'conflict',
  'status.local': 'Local changes (push to upload):',
  'status.remote': 'Server changes (pull to download):',
  'status.conflicts': 'Conflicts (changed on both sides):',
  'status.skipped': 'Skipped (not synced):',
  'status.skip.ext': 'unsupported file type',
  'status.skip.name': 'name not allowed (use A-Z a-z 0-9 . _ - and / only)',

  // pull
  'pull.wrote': '  ↓ {path}',
  'pull.removed': '  x {path} (deleted on server)',
  'pull.kept': '  = {path} (local change kept)',
  'pull.conflict': '  ! {path} — changed locally and on the server',
  'pull.refused': 'Refused to overwrite {n} locally changed file(s). Keep your copy, merge by hand, or re-run with --force to take the server version.',
  'pull.done': 'Pulled: {w} downloaded, {d} deleted, {k} local changes kept.',
  'pull.upToDate': 'Already up to date.',
  'types.wrote': 'Updated {file}',
  'types.failed': 'warning: could not download anasaki.d.ts: {msg}',

  // push
  'push.put': '  ↑ {path}',
  'push.del': '  x {path} (delete on server)',
  'push.wouldPut': '  would upload {path}',
  'push.wouldDel': '  would delete {path} on server',
  'push.keptRemote': '  - {path} deleted locally; still on the server (use --delete to remove it there)',
  'push.remoteNews': 'note: {n} file(s) changed on the server since your last sync — run "anasaki-code pull" to get them.',
  'push.nothing': 'Nothing to push — server draft already matches.',
  'push.done': 'Pushed: {p} uploaded, {d} deleted.',
  'push.dryDone': 'Dry run: {p} to upload, {d} to delete. Nothing was changed.',
  'push.buildOk': 'Build check: OK',
  'push.buildErrors': 'Build check: {n} error(s) in the draft (fix them before publishing).',
  'push.conflictLocal': 'Conflict — these files changed on the server AND locally since your last sync:',
  'push.conflict409': 'Conflict — the server copy changed since your last sync (nothing was uploaded):',
  'push.conflictNext': 'Nothing was uploaded. Next steps:\n  1. anasaki-code pull            keeps your edits, downloads the rest, lists the conflicting files\n  2. merge the conflicting files by hand (back up your version first), then anasaki-code pull --force to take the server copy\n  3. re-apply your edits and run anasaki-code push',
  'push.rejected': 'The server rejected the upload: {msg}',

  // watch
  'watch.start': 'Watching {dir} — Ctrl+C to stop. Remote files are never deleted in watch mode.',
  'watch.poll': '(recursive fs.watch unavailable — polling every {ms} ms)',
  'watch.deletedLocal': 'note: {path} was deleted locally; watch never deletes on the server (use: anasaki-code push --delete)',
  'watch.stopped': 'Stopped watching.',

  // publish
  'publish.localPending': 'warning: {n} local change(s) not pushed — publish uses the server draft. Run "anasaki-code push" first if needed.',
  'publish.noChanges': 'The server draft matches the live version (#{live}) — nothing to publish.',
  'publish.buildFailed': 'Cannot publish: the draft has build errors.',
  'publish.migrations': 'Pending database migrations ({n}):',
  'publish.destructiveHead': 'DESTRUCTIVE changes (data may be lost; the tables are backed up first):',
  'publish.destructiveItem': '  ! {migration}: {op} {target}',
  'publish.confirm': 'Publish with these destructive changes? Type "yes" to continue: ',
  'publish.aborted': 'Publish cancelled.',
  'publish.ok': 'Published version #{id}{msg}.',
  'publish.conflict': 'Someone published meanwhile (live is now #{live}). Run "anasaki-code status", then publish again.',
  'publish.failed': 'Publish refused: {msg}',

  // logs
  'logs.none': 'No log entries.',

  'state.corrupt': 'warning: .anasaki/state.json is unreadable; treating everything as never synced.',
};

const th = {
  'err.generic': 'ผิดพลาด: {msg}',
  'err.network': 'ติดต่อ {site} ไม่ได้: {msg}',
  'err.unauthorized': 'เซิร์ฟเวอร์ไม่รับโทเค็นนี้ (401): {msg}\n  ให้รัน: anasaki-code login {site}',
  'err.forbidden': 'ไม่มีสิทธิ์ (403): {msg}\n  โทเค็นหรือบทบาทของคุณไม่มีสิทธิ์สำหรับคำสั่งนี้',
  'err.http': 'เซิร์ฟเวอร์ตอบ {status}: {msg}',
  'err.noProject': 'ไม่พบ anasaki.json ใน {cwd} หรือโฟลเดอร์แม่\n  ให้รัน: anasaki-code init <site>',
  'err.badConfig': 'anasaki.json ไม่ถูกต้อง: {msg}',
  'err.noToken': 'ยังไม่ได้ล็อกอิน {site}\n  ให้รัน: anasaki-code login {site}   (หรือตั้ง ANASAKI_TOKEN)',
  'err.unknownCommand': 'ไม่รู้จักคำสั่ง: {cmd}\n  ดูวิธีใช้: anasaki-code --help',
  'err.unknownOption': 'ไม่รู้จักตัวเลือก: {opt}',
  'err.missingValue': 'ตัวเลือก {opt} ต้องมีค่า',
  'err.needSite': 'เว็บไหน? วิธีใช้: anasaki-code {cmd} <site>',
  'err.badSite': 'URL ของเว็บไม่ถูกต้อง: {site}',
  'err.emptyToken': 'ไม่ได้ใส่โทเค็น',
  'err.tokenFormat': 'คำเตือน: โทเค็นปกติขึ้นต้นด้วย "ast_"',
  'err.notTTY': 'ต้องยืนยันแต่ไม่ได้รันแบบโต้ตอบ — รันใหม่พร้อม --yes เพื่อยืนยัน',

  'login.prompt': 'วางโทเค็น API ของ {site} (โปรไฟล์ → โทเค็นสำหรับเครื่องมือ): ',
  'login.ok': 'ล็อกอิน {site} แล้ว ในชื่อ {user}',
  'login.saved': 'บันทึกโทเค็นไว้ที่ {file}',
  'login.envNote': 'หมายเหตุ: ตั้ง ANASAKI_TOKEN ไว้ ซึ่งจะถูกใช้แทนโทเค็นที่บันทึก',
  'logout.ok': 'ลบโทเค็นที่บันทึกไว้ของ {site} แล้ว',
  'logout.none': 'ไม่มีโทเค็นที่บันทึกไว้ของ {site}',

  'init.exists': 'มี anasaki.json อยู่แล้วใน {dir} — ใช้ไฟล์เดิม',
  'init.wrote': 'สร้าง {file} แล้ว',
  'init.kept': 'ใช้ {file} เดิม',
  'init.done': 'พร้อมแล้ว แก้ไฟล์แล้วรัน: anasaki-code push   (หรือ: anasaki-code watch)',

  'status.server': 'เซิร์ฟเวอร์ {site}: เวอร์ชันที่ใช้งาน {live}{draft}',
  'status.noLive': 'ยังไม่มี',
  'status.draftChanged': ', ฉบับร่างมีการแก้ที่ยังไม่เผยแพร่',
  'status.clean': 'ตรงกันทั้งหมด ({n} ไฟล์)',
  'status.kind.modified': 'แก้ไข',
  'status.kind.added': 'เพิ่มใหม่',
  'status.kind.deleted': 'ลบ',
  'status.kind.remote-modified': 'แก้บนเซิร์ฟเวอร์',
  'status.kind.remote-added': 'เพิ่มบนเซิร์ฟเวอร์',
  'status.kind.remote-deleted': 'ลบบนเซิร์ฟเวอร์',
  'status.kind.conflict': 'ชนกัน',
  'status.local': 'การแก้ในเครื่อง (push เพื่ออัปโหลด):',
  'status.remote': 'การแก้บนเซิร์ฟเวอร์ (pull เพื่อดาวน์โหลด):',
  'status.conflicts': 'ไฟล์ที่ชนกัน (ถูกแก้ทั้งสองฝั่ง):',
  'status.skipped': 'ข้าม (ไม่ซิงก์):',
  'status.skip.ext': 'ชนิดไฟล์ที่ไม่รองรับ',
  'status.skip.name': 'ชื่อไฟล์ไม่อนุญาต (ใช้ได้เฉพาะ A-Z a-z 0-9 . _ - และ /)',

  'pull.wrote': '  ↓ {path}',
  'pull.removed': '  x {path} (ถูกลบบนเซิร์ฟเวอร์)',
  'pull.kept': '  = {path} (เก็บการแก้ในเครื่องไว้)',
  'pull.conflict': '  ! {path} — ถูกแก้ทั้งในเครื่องและบนเซิร์ฟเวอร์',
  'pull.refused': 'ไม่เขียนทับไฟล์ที่แก้ในเครื่อง {n} ไฟล์ เก็บฉบับของคุณไว้ รวมการแก้เอง หรือรันใหม่พร้อม --force เพื่อใช้ฉบับบนเซิร์ฟเวอร์',
  'pull.done': 'ดึงแล้ว: ดาวน์โหลด {w}, ลบ {d}, เก็บการแก้ในเครื่อง {k}',
  'pull.upToDate': 'เป็นปัจจุบันแล้ว',
  'types.wrote': 'อัปเดต {file} แล้ว',
  'types.failed': 'คำเตือน: ดาวน์โหลด anasaki.d.ts ไม่ได้: {msg}',

  'push.put': '  ↑ {path}',
  'push.del': '  x {path} (ลบบนเซิร์ฟเวอร์)',
  'push.wouldPut': '  จะอัปโหลด {path}',
  'push.wouldDel': '  จะลบ {path} บนเซิร์ฟเวอร์',
  'push.keptRemote': '  - {path} ถูกลบในเครื่อง แต่ยังอยู่บนเซิร์ฟเวอร์ (ใช้ --delete เพื่อลบบนเซิร์ฟเวอร์)',
  'push.remoteNews': 'หมายเหตุ: มี {n} ไฟล์ที่ถูกแก้บนเซิร์ฟเวอร์หลังซิงก์ครั้งล่าสุด — รัน "anasaki-code pull" เพื่อดึงมา',
  'push.nothing': 'ไม่มีอะไรต้อง push — ฉบับร่างบนเซิร์ฟเวอร์ตรงกันแล้ว',
  'push.done': 'push แล้ว: อัปโหลด {p}, ลบ {d}',
  'push.dryDone': 'ทดลอง (dry run): จะอัปโหลด {p}, จะลบ {d} ยังไม่มีการเปลี่ยนแปลงใด ๆ',
  'push.buildOk': 'ตรวจ build: ผ่าน',
  'push.buildErrors': 'ตรวจ build: ฉบับร่างมีข้อผิดพลาด {n} จุด (แก้ก่อนเผยแพร่)',
  'push.conflictLocal': 'ชนกัน — ไฟล์เหล่านี้ถูกแก้ทั้งบนเซิร์ฟเวอร์และในเครื่องหลังซิงก์ครั้งล่าสุด:',
  'push.conflict409': 'ชนกัน — ไฟล์บนเซิร์ฟเวอร์เปลี่ยนไปหลังซิงก์ครั้งล่าสุด (ยังไม่ได้อัปโหลดอะไร):',
  'push.conflictNext': 'ยังไม่ได้อัปโหลดอะไร ขั้นต่อไป:\n  1. anasaki-code pull            เก็บการแก้ของคุณ ดึงไฟล์อื่น และแสดงไฟล์ที่ชนกัน\n  2. รวมไฟล์ที่ชนกันเอง (สำรองฉบับของคุณก่อน) แล้วรัน anasaki-code pull --force เพื่อใช้ฉบับบนเซิร์ฟเวอร์\n  3. ใส่การแก้ของคุณกลับ แล้วรัน anasaki-code push',
  'push.rejected': 'เซิร์ฟเวอร์ไม่รับการอัปโหลด: {msg}',

  'watch.start': 'กำลังเฝ้าดู {dir} — กด Ctrl+C เพื่อหยุด (โหมด watch ไม่ลบไฟล์บนเซิร์ฟเวอร์)',
  'watch.poll': '(ใช้ fs.watch แบบ recursive ไม่ได้ — ตรวจทุก {ms} ms)',
  'watch.deletedLocal': 'หมายเหตุ: {path} ถูกลบในเครื่อง; watch ไม่ลบบนเซิร์ฟเวอร์ (ใช้: anasaki-code push --delete)',
  'watch.stopped': 'หยุดเฝ้าดูแล้ว',

  'publish.localPending': 'คำเตือน: มีการแก้ในเครื่อง {n} ไฟล์ที่ยังไม่ push — publish ใช้ฉบับร่างบนเซิร์ฟเวอร์ รัน "anasaki-code push" ก่อนถ้าต้องการ',
  'publish.noChanges': 'ฉบับร่างบนเซิร์ฟเวอร์ตรงกับเวอร์ชันที่ใช้งาน (#{live}) — ไม่มีอะไรต้องเผยแพร่',
  'publish.buildFailed': 'เผยแพร่ไม่ได้: ฉบับร่างมีข้อผิดพลาด',
  'publish.migrations': 'migration ฐานข้อมูลที่รอรัน ({n}):',
  'publish.destructiveHead': 'การเปลี่ยนแปลงที่ทำให้ข้อมูลหาย (ระบบจะสำรองตารางก่อน):',
  'publish.destructiveItem': '  ! {migration}: {op} {target}',
  'publish.confirm': 'เผยแพร่พร้อมการเปลี่ยนแปลงที่ทำให้ข้อมูลหาย? พิมพ์ "yes" เพื่อดำเนินการ: ',
  'publish.aborted': 'ยกเลิกการเผยแพร่',
  'publish.ok': 'เผยแพร่เวอร์ชัน #{id}{msg} แล้ว',
  'publish.conflict': 'มีคนเผยแพร่ไปก่อนแล้ว (ตอนนี้ใช้งาน #{live}) รัน "anasaki-code status" แล้วเผยแพร่อีกครั้ง',
  'publish.failed': 'เผยแพร่ไม่สำเร็จ: {msg}',

  'logs.none': 'ไม่มีบันทึก',

  'state.corrupt': 'คำเตือน: อ่าน .anasaki/state.json ไม่ได้ จะถือว่ายังไม่เคยซิงก์',
};

export const messages = { en, th };

export function translator(lang) {
  const dict = messages[lang] || en;
  return (key, vars = {}) => {
    const s = dict[key] ?? en[key] ?? key;
    return s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
  };
}

export const HELP = {
  en: `anasaki-code — sync Anasaki Studio Site Code with a local folder

Usage: anasaki-code <command> [options]

Commands:
  login <site> [--token ast_...]   save an API token for a site (verified with /auth/me)
  logout [site]                    forget the saved token
  init [site] [--dir <dir>]        write anasaki.json + tsconfig.json, then pull
  pull [--force]                   download the server draft + anasaki.d.ts
                                   (never overwrites local changes without --force)
  push [--dry-run] [--delete]      upload changed files (deletes on the server only with --delete)
  status                           compare local / server / last sync
  watch                            push on every change (300 ms debounce); never deletes on the server
  publish [-m <msg>] [--yes]       publish the server draft (asks before destructive migrations)
  logs [--follow] [--unit <id>] [--level <l>]
                                   show Site Code logs
  types                            refresh anasaki.d.ts

Options: --help, --version
Environment: ANASAKI_TOKEN (overrides saved token), ANASAKI_LANG=th (Thai messages),
             ANASAKI_CONFIG_DIR (default ~/.anasaki)
Exit codes: 0 ok, 1 error, 2 conflict / validation / build errors
`,
  th: `anasaki-code — ซิงก์ Site Code ของ Anasaki Studio กับโฟลเดอร์ในเครื่อง

วิธีใช้: anasaki-code <คำสั่ง> [ตัวเลือก]

คำสั่ง:
  login <site> [--token ast_...]   บันทึกโทเค็น API ของเว็บ (ตรวจกับ /auth/me)
  logout [site]                    ลบโทเค็นที่บันทึกไว้
  init [site] [--dir <dir>]        สร้าง anasaki.json + tsconfig.json แล้ว pull
  pull [--force]                   ดาวน์โหลดฉบับร่างจากเซิร์ฟเวอร์ + anasaki.d.ts
                                   (ไม่เขียนทับไฟล์ที่แก้ในเครื่อง เว้นแต่ใช้ --force)
  push [--dry-run] [--delete]      อัปโหลดไฟล์ที่แก้ (ลบบนเซิร์ฟเวอร์เฉพาะเมื่อใช้ --delete)
  status                           เทียบในเครื่อง / เซิร์ฟเวอร์ / ซิงก์ครั้งล่าสุด
  watch                            push ทุกครั้งที่แก้ไฟล์ (หน่วง 300 ms) ไม่ลบไฟล์บนเซิร์ฟเวอร์
  publish [-m <ข้อความ>] [--yes]   เผยแพร่ฉบับร่างบนเซิร์ฟเวอร์ (ถามก่อนถ้า migration ทำให้ข้อมูลหาย)
  logs [--follow] [--unit <id>] [--level <l>]
                                   ดูบันทึกของ Site Code
  types                            ดาวน์โหลด anasaki.d.ts ใหม่

ตัวเลือก: --help, --version
ตัวแปรสภาพแวดล้อม: ANASAKI_TOKEN (ใช้แทนโทเค็นที่บันทึก), ANASAKI_LANG=th (ข้อความภาษาไทย),
                   ANASAKI_CONFIG_DIR (ค่าเริ่มต้น ~/.anasaki)
รหัสออก: 0 สำเร็จ, 1 ผิดพลาด, 2 ชนกัน / ข้อมูลไม่ถูกต้อง / build มีข้อผิดพลาด
`,
};
