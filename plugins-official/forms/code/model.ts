// Forms: model, validation and storage — ported 1:1 from the built-in Go plugin (internal/plugins/forms).

export interface Field {
  key: string;
  label: string;
  type: string; // text|email|tel|textarea|select|checkbox|number|date, 2.1: radio|checkboxes, 2.2: file
  required: boolean;
  options: string[];
  placeholder: string;
  /** 2.1: shown (and checked) only when an earlier field has this value. */
  showIf?: { field: string; value: string };
}

/** 2.1: a mail to the address in the form's first email field after a submission (empty subject = off). */
export interface AutoReply {
  subject: string;
  message: string;
}

export interface Form {
  id: number;
  name: string;
  fields: Field[];
  submitLabel: string;
  successMessage: string;
  redirectUrl: string;
  webhookUrl: string;
  autoReply: AutoReply;
  createdAt: string;
  submissionCount: number;
  unreadCount: number;
}

export interface Submission {
  id: number;
  formId: number;
  data: Record<string, string>;
  read: boolean;
  createdAt: string;
}

export const DEFAULT_SUCCESS = 'ขอบคุณ! เราได้รับข้อความของคุณแล้ว';
export const DEFAULT_SUBMIT = 'ส่งข้อความ';

const FIELD_TYPES = ['text', 'email', 'tel', 'textarea', 'select', 'checkbox', 'number', 'date', 'radio', 'checkboxes', 'file'];
/** Types with a list of options. */
export const WITH_OPTIONS = ['select', 'radio', 'checkboxes'];
const keyRe = /^[a-zA-Z][a-zA-Z0-9_]{0,39}$/;

/** UTF-8 byte length (the Go plugin's limits count bytes). */
export function byteLen(s: string): number {
  let n = 0;
  for (const ch of s) {
    const c = ch.codePointAt(0) as number;
    n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4;
  }
  return n;
}

/** Go's %q for the messages. */
const q = (s: string) => JSON.stringify(s);

const str = (v: unknown) => (v === undefined || v === null ? '' : String(v));

/**
 * An address on this site ("/thank-you", "/a?b=1#c"): one leading "/", not "//host" or "/\host", no control
 * characters — sending visitors elsewhere after a form would make the site an open redirect.
 */
export function localPath(s: string): boolean {
  if (!s.startsWith('/') || s.startsWith('//') || s.includes('\\')) return false;
  for (const ch of s) {
    const c = ch.codePointAt(0) as number;
    if (c < 0x20 || c === 0x7f) return false;
  }
  return true;
}

/** Normalises a form from admin input and returns the first problem (Thai message), or "". */
export function validateForm(input: any): { form: Form; error: string } {
  const fm: Form = {
    id: Number(input?.id) || 0,
    name: str(input?.name).trim(),
    fields: [],
    submitLabel: str(input?.submitLabel),
    successMessage: str(input?.successMessage),
    redirectUrl: str(input?.redirectUrl).trim(),
    webhookUrl: str(input?.webhookUrl).trim(),
    autoReply: { subject: str(input?.autoReply?.subject).trim(), message: str(input?.autoReply?.message).trim() },
    createdAt: '',
    submissionCount: 0,
    unreadCount: 0,
  };
  const bad = (error: string) => ({ form: fm, error });
  if (fm.name === '' || byteLen(fm.name) > 191) return bad('กรุณาตั้งชื่อฟอร์ม');
  const raw = Array.isArray(input?.fields) ? input.fields : [];
  if (raw.length === 0 || raw.length > 50) return bad('ฟอร์มต้องมีช่องกรอกอย่างน้อย 1 ช่อง (ไม่เกิน 50 ช่อง)');
  const seen: Record<string, boolean> = {};
  for (const r of raw) {
    const fd: Field = {
      key: str(r?.key).trim(),
      label: str(r?.label).trim(),
      type: str(r?.type) || 'text',
      required: r?.required === true,
      options: Array.isArray(r?.options) ? r.options.map(str) : [],
      placeholder: str(r?.placeholder),
    };
    if (r?.showIf && typeof r.showIf === 'object' && str(r.showIf.field) !== '') {
      fd.showIf = { field: str(r.showIf.field).trim(), value: str(r.showIf.value).trim() };
      if (!seen[fd.showIf.field]) return bad(`เงื่อนไขของช่อง ${q(fd.key)} ต้องอ้างถึงช่องที่อยู่ก่อนหน้า`);
      if (byteLen(fd.showIf.value) > 200) return bad(`เงื่อนไขของช่อง ${q(fd.key)} ยาวเกินไป`);
    }
    if (!keyRe.test(fd.key) || fd.key === '_hp' || seen[fd.key]) return bad(`คีย์ของช่อง ${q(fd.key)} ไม่ถูกต้องหรือซ้ำ (ใช้ a-z, 0-9, _)`);
    seen[fd.key] = true;
    if (FIELD_TYPES.indexOf(fd.type) < 0) return bad(`ชนิดของช่อง ${q(fd.key)} ไม่ถูกต้อง`);
    if (fd.label === '') fd.label = fd.key;
    if (byteLen(fd.label) > 300 || byteLen(fd.placeholder) > 300) return bad(`ข้อความของช่อง ${q(fd.key)} ยาวเกินไป`);
    if (WITH_OPTIONS.indexOf(fd.type) >= 0 && fd.options.length === 0) return bad(`ช่อง ${q(fd.key)} ต้องมีตัวเลือกอย่างน้อย 1 รายการ`);
    if (fd.options.length > 100) return bad(`ช่อง ${q(fd.key)} มีตัวเลือกมากเกินไป`);
    fm.fields.push(fd);
  }
  if (byteLen(fm.submitLabel) > 100 || byteLen(fm.successMessage) > 1000) return bad('ข้อความปุ่มหรือข้อความสำเร็จยาวเกินไป');
  if (byteLen(fm.autoReply.subject) > 200 || byteLen(fm.autoReply.message) > 5000) return bad('อีเมลตอบกลับอัตโนมัติยาวเกินไป');
  if (fm.autoReply.subject !== '' && !fm.fields.some((f) => f.type === 'email')) return bad('อีเมลตอบกลับอัตโนมัติต้องมีช่องอีเมลในฟอร์ม');
  if (fm.redirectUrl !== '' && (!localPath(fm.redirectUrl) || byteLen(fm.redirectUrl) > 512)) {
    return bad('ลิงก์หลังส่งฟอร์มต้องเป็นหน้าในเว็บนี้ที่ขึ้นต้นด้วย / เช่น /thank-you');
  }
  if (fm.webhookUrl !== '' && (byteLen(fm.webhookUrl) > 512 || !(fm.webhookUrl.startsWith('https://') || fm.webhookUrl.startsWith('http://')))) {
    return bad('Webhook URL ต้องขึ้นต้นด้วย http:// หรือ https://');
  }
  return { form: fm, error: '' };
}

// ---------------------------------------------------------------- submitted values

const atext = /^[A-Za-z0-9!#$%&'*+/=?^_`{|}~\-\u0080-\uffff]+$/;
const dotAtom = (s: string) => s !== '' && s.split('.').every((p) => atext.test(p));

/** Like Go's mail.ParseAddress(v) with a.Address == v: a bare addr-spec (no display name, no quoting). */
export function plainEmail(v: string): boolean {
  const at = v.lastIndexOf('@');
  if (at <= 0 || at === v.length - 1) return false;
  return dotAtom(v.slice(0, at)) && dotAtom(v.slice(at + 1));
}

/** Like Go's strconv.ParseFloat (decimal forms, exponents, inf / nan). */
export function goNumber(v: string): boolean {
  return /^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/.test(v) || /^[+-]?(inf|infinity|nan)$/i.test(v);
}

/** YYYY-MM-DD with a real calendar day (Go's time.Parse("2006-01-02")). */
export function isoDate(v: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v);
  if (!m) return false;
  const y = Number(m[1]), mo = Number(m[2]), d = Number(m[3]);
  if (mo < 1 || mo > 12 || d < 1) return false;
  const dim = [31, y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][mo - 1];
  return d <= dim;
}

/** Validates submitted values against the form: the stored data, or a user-facing (Thai) message. */
export function clean(fm: Form, input: Record<string, string>): { data: Record<string, string> | null; problem: string } {
  const data: Record<string, string> = {};
  for (const fd of fm.fields) {
    // 2.1: a field whose condition is not met is neither required nor kept
    if (fd.showIf && !conditionMet(fd.showIf, data)) {
      data[fd.key] = '';
      continue;
    }
    let v = str(input[fd.key]).trim();
    if (fd.type === 'checkboxes') v = multiValues(v).join(', ');
    if (byteLen(v) > 5000) return { data: null, problem: 'ข้อความในช่อง "' + fd.label + '" ยาวเกินไป' };
    if (fd.type === 'checkbox') v = v !== '' && v !== 'false' && v !== '0' ? 'yes' : '';
    if (v === '') {
      if (fd.required) return { data: null, problem: 'กรุณากรอก "' + fd.label + '"' };
      data[fd.key] = '';
      continue;
    }
    switch (fd.type) {
      case 'file': // 2.2: the sent file's name (submit.ts checked and keeps the file)
        break;
      case 'email':
        if (!plainEmail(v)) return { data: null, problem: 'อีเมลในช่อง "' + fd.label + '" ไม่ถูกต้อง' };
        break;
      case 'number':
        if (!goNumber(v)) return { data: null, problem: 'ช่อง "' + fd.label + '" ต้องเป็นตัวเลข' };
        break;
      case 'date':
        if (!isoDate(v)) return { data: null, problem: 'วันที่ในช่อง "' + fd.label + '" ไม่ถูกต้อง' };
        break;
      case 'select':
      case 'radio':
        if (fd.options.indexOf(v) < 0) return { data: null, problem: 'ตัวเลือกในช่อง "' + fd.label + '" ไม่ถูกต้อง' };
        break;
      case 'checkboxes':
        if (multiValues(str(input[fd.key])).some((x) => fd.options.indexOf(x) < 0)) return { data: null, problem: 'ตัวเลือกในช่อง "' + fd.label + '" ไม่ถูกต้อง' };
        break;
    }
    data[fd.key] = v;
  }
  return { data, problem: '' };
}

/** The values of a checkboxes field: one per line (form posts), a JSON array (JSON posts) or "a, b" (stored). */
export function multiValues(v: string): string[] {
  const s = str(v).trim();
  if (s === '') return [];
  if (s.startsWith('[')) {
    try {
      const a = JSON.parse(s);
      if (Array.isArray(a)) return a.map((x) => str(x).trim()).filter(Boolean);
    } catch {
      /* not JSON */
    }
  }
  return s.split(s.includes('\n') ? /\r?\n/ : /\s*,\s*/).map((x) => x.trim()).filter(Boolean);
}

/** Whether a field's condition holds for the values cleaned so far (checkbox: "yes"; checkboxes: one of them). */
export function conditionMet(c: { field: string; value: string }, data: Record<string, string>): boolean {
  const v = data[c.field] ?? '';
  if (c.value === '') return v !== '';
  return v === c.value || v.split(', ').indexOf(c.value) >= 0;
}

// ---------------------------------------------------------------- storage

/** RFC 3339 without fractions ("2026-10-31T08:00:00Z"), like Go's time.Time JSON / Format(RFC3339) in UTC. */
export function rfc3339(v: unknown): string {
  const d = v instanceof Date ? v : new Date(String(v ?? ''));
  return isNaN(d.getTime()) ? '' : d.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

const parseJSON = (v: unknown, def: any) => {
  if (v === null || v === undefined) return def;
  if (typeof v !== 'string') return v;
  try {
    return JSON.parse(v);
  } catch {
    return def;
  }
};

function fieldsOf(v: unknown): Field[] {
  const list = parseJSON(v, []);
  return (Array.isArray(list) ? list : []).map((r: any) => ({
    key: str(r?.key), label: str(r?.label), type: str(r?.type) || 'text', required: r?.required === true,
    options: Array.isArray(r?.options) ? r.options.map(str) : [], placeholder: str(r?.placeholder),
    ...(r?.showIf && str(r.showIf.field) ? { showIf: { field: str(r.showIf.field), value: str(r.showIf.value) } } : {}),
  }));
}

function toForm(r: any, counts = true): Form {
  const id = Number(r.id);
  return {
    id, name: str(r.name), fields: fieldsOf(r.fields), submitLabel: str(r.submit_label), successMessage: str(r.success_message),
    redirectUrl: str(r.redirect_url), webhookUrl: str(r.webhook_url),
    autoReply: { subject: str(r.auto_reply_subject), message: str(r.auto_reply_message) }, createdAt: rfc3339(r.created_at),
    submissionCount: counts ? db.table('submissions').where('form_id', id).count() : 0,
    unreadCount: counts ? db.table('submissions').where('form_id', id).where('is_read', false).count() : 0,
  };
}

export function getForm(id: number): Form | null {
  if (!(id > 0)) return null;
  const r = db.table('forms').where('id', id).first();
  return r ? toForm(r) : null;
}

/** Newest first. */
export function listForms(): Form[] {
  return db.table('forms').orderBy('id', 'desc').get().map((r: any) => toForm(r));
}

const row = (fm: Form) => ({
  name: fm.name, fields: JSON.stringify(fm.fields), submit_label: fm.submitLabel, success_message: fm.successMessage,
  redirect_url: fm.redirectUrl, webhook_url: fm.webhookUrl,
  auto_reply_subject: fm.autoReply?.subject ?? '', auto_reply_message: fm.autoReply?.message ?? '',
});

export function createForm(fm: Form): Form {
  const id = db.table('forms').insert({ ...row(fm), created_at: new Date() });
  return getForm(id) as Form;
}

export function updateForm(id: number, fm: Form): Form | null {
  if (!getForm(id)) return null;
  db.table('forms').where('id', id).update(row(fm));
  return getForm(id);
}

/** Deletes a form and its submissions (and their files). */
export function deleteForm(id: number): void {
  for (const f of files.list(`forms/${id}/`)) files.delete(f.path);
  db.table('submissions').where('form_id', id).delete();
  db.table('forms').where('id', id).delete();
}

/** 2.2: the private files of a submission, {field key: path} (kept in its data as "_files"). */
export function filesOf(s: Submission): Record<string, string> {
  try {
    const m = JSON.parse(s.data._files ?? '{}');
    return m && typeof m === 'object' ? m : {};
  } catch {
    return {};
  }
}

function toSubmission(r: any): Submission {
  const data = parseJSON(r.data, {});
  const out: Record<string, string> = {};
  if (data && typeof data === 'object') for (const k of Object.keys(data)) out[k] = str(data[k]);
  return { id: Number(r.id), formId: Number(r.form_id), data: out, read: r.is_read === true || r.is_read === 1, createdAt: rfc3339(r.created_at) };
}

/** Newest first, at most 5000 (like the Go plugin). */
export function listSubmissions(formId: number): Submission[] {
  return db.table('submissions').where('form_id', formId).orderBy('id', 'desc').limit(5000).get().map(toSubmission);
}

export function getSubmission(id: number): Submission | null {
  const r = db.table('submissions').where('id', id).first();
  return r ? toSubmission(r) : null;
}

export function addSubmission(formId: number, data: Record<string, string>, at: Date): number {
  return db.table('submissions').insert({ form_id: formId, data: JSON.stringify(data), is_read: false, created_at: at });
}

export function markRead(id: number, read: boolean): Submission | null {
  db.table('submissions').where('id', id).update({ is_read: read });
  return getSubmission(id);
}

export function deleteSubmission(id: number): void {
  const s = getSubmission(id);
  if (s) for (const p of Object.values(filesOf(s))) files.delete(p);
  db.table('submissions').where('id', id).delete();
}
