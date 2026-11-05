// Visitor texts in the site's language, the site's address and the mail of a campaign (shared by main and admin).
import { bodyHTML, esc } from './model';

const T = {
  title: ['รับข่าวสารทางอีเมล', 'Get our newsletter'],
  sub: ['รับข่าวและโปรโมชันก่อนใคร ยกเลิกได้ทุกเมื่อ', 'News and offers first — unsubscribe any time'],
  email: ['อีเมลของคุณ', 'Your email'],
  name: ['ชื่อ (ไม่บังคับ)', 'Name (optional)'],
  button: ['สมัครรับข่าว', 'Subscribe'],
  checkMail: ['เกือบเสร็จแล้ว! กดลิงก์ในอีเมลที่เราส่งไปเพื่อยืนยัน', 'Almost done! Click the link in the email we sent to confirm'],
  done: ['สมัครรับข่าวเรียบร้อย ขอบคุณ!', 'You are subscribed — thank you!'],
  bad: ['อีเมลไม่ถูกต้อง', 'Invalid email address'],
  busy: ['ส่งถี่เกินไป รอสักครู่แล้วลองใหม่', 'Too many tries — wait a moment'],
  confirmSubject: ['ยืนยันการรับข่าวสารจาก {site}', 'Confirm your subscription to {site}'],
  confirmBody: ['กดลิงก์นี้เพื่อยืนยันการรับข่าวสาร:', 'Click this link to confirm your subscription:'],
  ignore: ['ถ้าคุณไม่ได้สมัคร ไม่ต้องทำอะไร', 'If you did not sign up, ignore this mail'],
  confirmed: ['ยืนยันแล้ว คุณจะได้รับข่าวสารจากเรา', 'Confirmed — you will receive our newsletter'],
  invalid: ['ลิงก์ไม่ถูกต้องหรือหมดอายุ', 'This link is invalid'],
  unsubTitle: ['ยกเลิกรับข่าวสาร', 'Unsubscribe'],
  unsubAsk: ['ยกเลิกการรับข่าวสารของ {email}?', 'Stop sending the newsletter to {email}?'],
  unsubBtn: ['ยกเลิกรับข่าว', 'Unsubscribe'],
  unsubDone: ['ยกเลิกแล้ว คุณจะไม่ได้รับจดหมายข่าวอีก', 'Done — you will not receive the newsletter any more'],
  unsubLink: ['ยกเลิกรับข่าวสาร', 'Unsubscribe'],
  home: ['กลับหน้าแรก', 'Home'],
} as const;
type K = keyof typeof T;

export function lang(): 0 | 1 {
  try {
    return String(site.info().language || 'th').startsWith('th') ? 0 : 1;
  } catch {
    return 0;
  }
}
export const t = (k: K, p?: Record<string, string | number>) => {
  let s: string = T[k][lang()];
  if (p) for (const [a, b] of Object.entries(p)) s = s.split(`{${a}}`).join(String(b));
  return s;
};

export function siteURL(): string {
  try {
    return (site.info().url || '').replace(/\/+$/, '');
  } catch {
    return '';
  }
}
export const siteTitle = () => {
  try {
    return site.info().title || '';
  } catch {
    return '';
  }
};

export function mailFor(c: { subject: string; body: string }, s: { email: string; token: string }): MailMessage {
  const un = `${siteURL()}/newsletter/unsubscribe?t=${s.token}`;
  const footer = String(site.settings('footer') ?? '').trim();
  return {
    to: s.email,
    subject: c.subject,
    html: `<div style="max-width:600px;margin:0 auto;font-family:system-ui,sans-serif;line-height:1.6">${bodyHTML(c.body)}` +
      `<hr style="border:0;border-top:1px solid #e5e7eb;margin:24px 0"><p style="color:#6b7280;font-size:13px">${footer ? esc(footer).replace(/\n/g, '<br>') + '<br>' : ''}` +
      `<a href="${esc(un)}">${esc(t('unsubLink'))}</a></p></div>`,
    text: `${c.body}\n\n--\n${footer ? footer + '\n' : ''}${t('unsubLink')}: ${un}\n`,
  };
}

