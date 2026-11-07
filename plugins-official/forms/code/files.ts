// forms 2.2: files sent with a form (file fields) — the types taken, checked by name and first bytes, at most
// 4 MB each, kept as private files of the plugin (forms/<form id>/<random>-<name>).

export const MAX_FILE = 4 << 20;

/** Extension → content type; nothing that a browser would run (no HTML, SVG or scripts). */
const TYPES: Record<string, string> = {
  jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png', gif: 'image/gif', webp: 'image/webp', heic: 'image/heic',
  pdf: 'application/pdf', txt: 'text/plain; charset=utf-8', csv: 'text/csv; charset=utf-8', zip: 'application/zip',
  doc: 'application/msword', xls: 'application/vnd.ms-excel', ppt: 'application/vnd.ms-powerpoint',
  docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
};

/** The file input's accept attribute. */
export const ACCEPT = Object.keys(TYPES).map((x) => '.' + x).join(',');
export const FILE_HINT = 'รูป PDF Word Excel หรือ ZIP ไม่เกิน 4 MB / Images, PDF, Office or ZIP up to 4 MB';

export const extOf = (name: string) => (/\.([a-z0-9]{1,5})$/i.exec(name)?.[1] ?? '').toLowerCase();

export const contentType = (name: string) => TYPES[extOf(name)] ?? 'application/octet-stream';

const starts = (b: Uint8Array, sig: number[]) => sig.every((x, i) => b[i] === x);

/** Why a sent file is refused ("" = fine). */
export function checkFile(name: string, bytes: Uint8Array): string {
  const ext = extOf(name);
  if (!TYPES[ext]) return 'รับเฉพาะรูป PDF เอกสาร Office ข้อความ หรือ ZIP / This file type is not accepted';
  if (bytes.length > MAX_FILE) return 'ไฟล์ใหญ่เกิน 4 MB / The file is over 4 MB';
  if (bytes.length === 0) return 'ไฟล์ว่าง / The file is empty';
  const ok = (() => {
    switch (ext) {
      case 'jpg': case 'jpeg': return starts(bytes, [0xff, 0xd8, 0xff]);
      case 'png': return starts(bytes, [0x89, 0x50, 0x4e, 0x47]);
      case 'gif': return starts(bytes, [0x47, 0x49, 0x46, 0x38]);
      case 'webp': return starts(bytes, [0x52, 0x49, 0x46, 0x46]) && bytes[8] === 0x57 && bytes[9] === 0x45 && bytes[10] === 0x42 && bytes[11] === 0x50;
      case 'heic': return bytes[4] === 0x66 && bytes[5] === 0x74 && bytes[6] === 0x79 && bytes[7] === 0x70; // ftyp
      case 'pdf': return starts(bytes, [0x25, 0x50, 0x44, 0x46]); // %PDF
      case 'zip': case 'docx': case 'xlsx': case 'pptx': return starts(bytes, [0x50, 0x4b, 0x03, 0x04]);
      case 'doc': case 'xls': case 'ppt': return starts(bytes, [0xd0, 0xcf, 0x11, 0xe0]);
      case 'txt': case 'csv': return !bytes.subarray(0, 4096).includes(0); // text: no NUL bytes
    }
    return false;
  })();
  return ok ? '' : 'ไฟล์ไม่ตรงกับนามสกุล / The file does not match its type';
}

/** A file name safe on disk and in headers: letters (Thai too), digits, . _ - ; at most 80 characters. */
export function safeName(name: string): string {
  const base = String(name ?? '').split(/[\\/]/).pop() ?? '';
  const ext = extOf(base);
  let stem = base.slice(0, ext ? base.length - ext.length - 1 : base.length).replace(/[^\p{L}\p{M}\p{N}._-]+/gu, '-').replace(/^[-.]+|[-.]+$/g, '');
  stem = Array.from(stem).slice(0, 70).join('') || 'file';
  return ext ? `${stem}.${ext}` : stem;
}
