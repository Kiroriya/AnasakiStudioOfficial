// Visitor-facing texts in the site's language (th, else English).

const T = {
  title: ['จองคิว', 'Book an appointment'],
  stepService: ['บริการ', 'Service'], stepDay: ['วัน', 'Day'], stepTime: ['เวลา', 'Time'], stepDetails: ['ข้อมูลผู้จอง', 'Your details'],
  min: ['นาที', 'min'],
  noServices: ['ยังไม่เปิดให้จอง', 'Bookings are not open yet'],
  change: ['เปลี่ยน', 'Change'],
  full: ['วันนี้ไม่มีเวลาว่างแล้ว ลองเลือกวันอื่น', 'No free times on this day — try another day'],
  name: ['ชื่อ', 'Name'], phone: ['เบอร์โทร', 'Phone'], email: ['อีเมล (รับการยืนยัน)', 'Email (for the confirmation)'], note: ['หมายเหตุ', 'Note'],
  confirm: ['ยืนยันการจอง', 'Confirm booking'],
  thanks: ['จองสำเร็จ! แล้วพบกัน', 'Booked — see you then!'],
  thanksPending: ['ได้รับการจองแล้ว ร้านจะยืนยันให้เร็วที่สุด', 'Booking received — the shop will confirm it soon'],
  noService: ['ไม่พบบริการนี้', 'Service not found'],
  check: ['กรุณากรอกชื่อและเบอร์โทรให้ถูกต้อง', 'Please enter a valid name and phone number'],
  busy: ['จองถี่เกินไป รอสักครู่แล้วลองใหม่', 'Too many bookings — wait a moment'],
  taken: ['ขออภัย เวลานี้เพิ่งถูกจองไป กรุณาเลือกเวลาอื่น', 'Sorry, this time was just taken — please pick another'],
  back: ['เลือกเวลาใหม่', 'Pick another time'],
  yourBooking: ['การจองของคุณ', 'Your booking'],
  status: ['สถานะ', 'Status'],
  s_pending: ['รอยืนยัน', 'Waiting for confirmation'], s_confirmed: ['ยืนยันแล้ว', 'Confirmed'], s_cancelled: ['ยกเลิกแล้ว', 'Cancelled'], s_done: ['เสร็จสิ้น', 'Completed'],
  cancel: ['ยกเลิกการจอง', 'Cancel booking'],
  cancelled: ['ยกเลิกการจองแล้ว', 'Booking cancelled'],
  keepLink: ['เก็บลิงก์หน้านี้ไว้เพื่อดูหรือยกเลิกการจอง', 'Keep this page’s link to see or cancel the booking'],
  notFound: ['ไม่พบการจอง', 'Booking not found'],
  mailPending: ['ได้รับการจองแล้ว (รอยืนยัน)', 'Booking received (waiting for confirmation)'],
  mailConfirmed: ['ยืนยันการจองแล้ว', 'Booking confirmed'],
  mailView: ['ดูหรือยกเลิกการจอง', 'See or cancel the booking'],
  bookNow: ['จองคิว', 'Book now'],
  // 1.1
  party: ['จำนวนคน', 'People'], staff: ['ผู้ให้บริการ', 'With'], anyStaff: ['ใครก็ได้', 'Anyone'], apply: ['ดูเวลาว่าง', 'Show times'],
  people: ['{n} คน', '{n} people'],
  depositNote: ['ต้องชำระมัดจำ {amount} เพื่อยืนยันการจอง', 'A deposit of {amount} confirms the booking'],
  answerAll: ['กรุณาตอบคำถามที่มีเครื่องหมาย *', 'Please answer the questions marked *'],
  thanksDeposit: ['จองไว้แล้ว — ชำระมัดจำและแนบสลิปด้านล่างเพื่อยืนยัน', 'Booked — pay the deposit and send the slip below to confirm'],
  s_deposit: ['รอชำระมัดจำ', 'Waiting for the deposit'],
  payDeposit: ['ชำระมัดจำ {amount}', 'Pay the deposit of {amount}'],
  account: ['ชื่อบัญชี: {name}', 'Account name: {name}'],
  slip: ['แนบสลิปการโอน', 'Upload the payment slip'], slipSend: ['ส่งสลิป', 'Send slip'],
  slipDone: ['ได้รับสลิปแล้ว ร้านจะตรวจและยืนยันการจองให้', 'Slip received — the booking is confirmed once it is checked'],
  slipBad: ['ส่งได้เฉพาะรูป JPG หรือ PNG', 'Only JPG or PNG images'],
  mailDeposit: ['จองไว้แล้ว รอชำระมัดจำ {amount}', 'Booked — deposit of {amount} due'],
  mailReminder: ['เตือนนัด', 'Reminder'],
  reminderText: ['แจ้งเตือนการจองของคุณ', 'A reminder of your booking'],
} as const;

export function lang(): 0 | 1 {
  try {
    return String(site.info().language || 'th').startsWith('th') ? 0 : 1;
  } catch {
    return 0;
  }
}

export function t(key: keyof typeof T, params?: Record<string, string | number>): string {
  let s: string = T[key][lang()];
  if (params) for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
  return s;
}
