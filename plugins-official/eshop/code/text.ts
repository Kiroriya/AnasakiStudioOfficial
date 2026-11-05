// Visitor-facing texts in the site's language (Settings → General → site language: th, else English).

const T = {
  cart: ['ตะกร้าสินค้า', 'Cart'],
  checkout: ['สั่งซื้อและชำระเงิน', 'Checkout'],
  order: ['คำสั่งซื้อ', 'Order'],
  empty: ['ยังไม่มีสินค้าในตะกร้า', 'Your cart is empty'],
  keepShopping: ['เลือกซื้อสินค้าต่อ', 'Continue shopping'],
  product: ['สินค้า', 'Product'],
  price: ['ราคา', 'Price'],
  qty: ['จำนวน', 'Qty'],
  total: ['รวม', 'Total'],
  subtotal: ['ยอดสินค้า', 'Subtotal'],
  shipping: ['ค่าส่ง', 'Shipping'],
  free: ['ฟรี', 'Free'],
  grand: ['ยอดชำระ', 'Total to pay'],
  update: ['อัปเดต', 'Update'],
  remove: ['ลบ', 'Remove'],
  toCheckout: ['ไปชำระเงิน', 'Checkout'],
  addToCart: ['ใส่ตะกร้า', 'Add to cart'],
  added: ['ใส่ตะกร้าแล้ว', 'Added to the cart'],
  viewCart: ['ดูตะกร้า', 'View cart'],
  soldOut: ['สินค้าหมด', 'Sold out'],
  low: ['เหลือไม่พอ', 'Not enough in stock'],
  left: ['เหลือ {n} ชิ้น', '{n} left'],
  name: ['ชื่อผู้รับ', 'Name'],
  phone: ['เบอร์โทร', 'Phone'],
  email: ['อีเมล (รับใบยืนยัน)', 'Email (for the confirmation)'],
  address: ['ที่อยู่จัดส่ง', 'Shipping address'],
  note: ['หมายเหตุ', 'Note'],
  payWith: ['ชำระเงินด้วย', 'Pay with'],
  m_promptpay: ['พร้อมเพย์ (สแกน QR)', 'PromptPay (scan the QR)'],
  m_bank: ['โอนเงินเข้าบัญชี', 'Bank transfer'],
  m_cod: ['เก็บเงินปลายทาง', 'Cash on delivery'],
  placeOrder: ['ยืนยันคำสั่งซื้อ', 'Place order'],
  noMethods: ['ร้านยังไม่ได้ตั้งค่าการชำระเงิน', 'The shop has not set up payments yet'],
  e_required: ['กรุณากรอก', 'Required'],
  e_tooLong: ['ยาวเกินไป', 'Too long'],
  e_phone: ['เบอร์โทรไม่ถูกต้อง', 'Invalid phone number'],
  e_email: ['อีเมลไม่ถูกต้อง', 'Invalid email address'],
  e_address: ['กรุณากรอกที่อยู่ให้ครบ', 'Please enter the full address'],
  e_payment: ['เลือกวิธีชำระเงิน', 'Choose how to pay'],
  e_empty: ['ตะกร้าว่าง', 'The cart is empty'],
  e_soldOut: ['มีสินค้าที่หมดแล้วในตะกร้า', 'Something in your cart is sold out'],
  e_low: ['มีสินค้าที่เหลือไม่พอ ลดจำนวนแล้วลองอีกครั้ง', 'Not enough stock for an item — lower the quantity and try again'],
  e_busy: ['สั่งซื้อถี่เกินไป รอสักครู่แล้วลองใหม่', 'Too many orders — wait a moment and try again'],
  fix: ['ตรวจข้อมูลที่ไฮไลต์', 'Please check the highlighted fields'],
  thanks: ['ขอบคุณสำหรับคำสั่งซื้อ', 'Thank you for your order'],
  orderNo: ['เลขที่คำสั่งซื้อ', 'Order number'],
  keepLink: ['เก็บลิงก์หน้านี้ไว้เพื่อดูสถานะ', 'Keep this page’s link to check the status'],
  scan: ['สแกนจ่ายด้วยแอปธนาคาร ยอด {amount}', 'Scan with your banking app to pay {amount}'],
  account: ['ชื่อบัญชี: {name}', 'Account name: {name}'],
  transferTo: ['โอนเงิน {amount} มาที่', 'Transfer {amount} to'],
  codNote: ['ชำระ {amount} กับพนักงานส่งของ', 'Pay {amount} on delivery'],
  slip: ['แนบสลิปการโอน', 'Upload the payment slip'],
  slipSend: ['ส่งสลิป', 'Send slip'],
  slipDone: ['ได้รับสลิปแล้ว ร้านจะตรวจสอบและยืนยันให้เร็วที่สุด', 'Slip received — the shop will check it soon'],
  slipBad: ['ส่งได้เฉพาะรูป JPG หรือ PNG', 'Only JPG or PNG images'],
  status: ['สถานะ', 'Status'],
  s_pending: ['รอชำระเงิน', 'Awaiting payment'],
  s_paid: ['ชำระแล้ว', 'Paid'],
  s_shipped: ['จัดส่งแล้ว', 'Shipped'],
  s_done: ['สำเร็จ', 'Completed'],
  s_cancelled: ['ยกเลิก', 'Cancelled'],
  notFound: ['ไม่พบคำสั่งซื้อ', 'Order not found'],
  items: ['รายการ', 'Items'],
  shipTo: ['จัดส่งถึง', 'Ship to'],
  mailNewSubject: ['คำสั่งซื้อ #{id} — {site}', 'Order #{id} — {site}'],
  mailPaidSubject: ['ยืนยันการชำระเงิน #{id} — {site}', 'Payment confirmed #{id} — {site}'],
  mailShippedSubject: ['จัดส่งแล้ว #{id} — {site}', 'Shipped #{id} — {site}'],
  mailView: ['ดูคำสั่งซื้อ', 'View the order'],
  mailAdminSubject: ['คำสั่งซื้อใหม่ #{id} ยอด {amount}', 'New order #{id}: {amount}'],
  chooseProduct: ['เลือกสินค้าในแผงคุณสมบัติ (หรือวางในแม่แบบหน้าสินค้า)', 'Choose a product in the properties (or place it in the product template)'],
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
