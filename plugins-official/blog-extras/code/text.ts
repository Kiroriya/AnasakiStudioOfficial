// Visitor-facing texts in the site's language (th, else English).

const T = {
  toc: ['สารบัญ', 'Contents'],
  minutes: ['อ่าน {n} นาที', '{n} min read'],
  share: ['แชร์', 'Share'],
  copy: ['คัดลอกลิงก์', 'Copy link'],
  copied: ['คัดลอกแล้ว', 'Copied'],
  related: ['บทความที่เกี่ยวข้อง', 'Related posts'],
  author: ['เขียนโดย', 'Written by'],
  comments: ['ความคิดเห็น', 'Comments'],
  count: ['{n} ความคิดเห็น', '{n} comments'],
  none: ['ยังไม่มีความคิดเห็น เป็นคนแรกที่แสดงความคิดเห็น', 'No comments yet — be the first'],
  leave: ['แสดงความคิดเห็น', 'Leave a comment'],
  reply: ['ตอบกลับ', 'Reply'],
  replyTo: ['ตอบกลับ {name}', 'Reply to {name}'],
  cancelReply: ['ยกเลิกการตอบกลับ', 'Cancel reply'],
  name: ['ชื่อ', 'Name'],
  email: ['อีเมล (ไม่แสดงบนเว็บ)', 'Email (not shown)'],
  body: ['ความคิดเห็น', 'Comment'],
  send: ['ส่งความคิดเห็น', 'Post comment'],
  pending: ['ได้รับความคิดเห็นแล้ว จะแสดงหลังผู้ดูแลอนุมัติ', 'Thanks — your comment will appear once approved'],
  posted: ['แสดงความคิดเห็นแล้ว', 'Your comment is posted'],
  e_name: ['กรุณากรอกชื่อ (ไม่เกิน 100 ตัวอักษร)', 'Enter your name (up to 100 characters)'],
  e_email: ['อีเมลไม่ถูกต้อง', 'Invalid email address'],
  e_body: ['กรุณาเขียนความคิดเห็น (2–3000 ตัวอักษร)', 'Write a comment (2–3000 characters)'],
  e_links: ['มีลิงก์มากเกินไป', 'Too many links'],
  e_busy: ['ส่งถี่เกินไป รอสักครู่แล้วลองใหม่', 'Too many comments — wait a moment'],
  e_closed: ['ปิดความคิดเห็นแล้ว', 'Comments are closed'],
  e_failed: ['ส่งความคิดเห็นไม่สำเร็จ', 'Could not post the comment'],
  back: ['กลับไปที่บทความ', 'Back to the post'],
} as const;

export type Key = keyof typeof T;

export function lang(): 0 | 1 {
  try {
    return String(site.info().language || 'th').startsWith('th') ? 0 : 1;
  } catch {
    return 0;
  }
}

export function t(key: Key, params?: Record<string, string | number>): string {
  let s: string = T[key][lang()];
  if (params) for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);
