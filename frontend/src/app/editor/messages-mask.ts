import { registerMessages } from '../core/messages';

// Masks and image focal point.
registerMessages({
  'mk.title': { th: 'Mask & ครอปรูป', en: 'Mask & crop' },
  'mk.none': { th: 'ไม่ใช้ mask', en: 'No mask' },
  'mk.hint': { th: 'ตัดชิ้นนี้ (เช่น รูป) ให้เป็นรูปทรง — วงกลม, blob, หกเหลี่ยม, ซุ้มโค้ง…', en: 'Clip this layer (e.g. a photo) to a shape — circle, blob, hexagon, arch…' },
  'mk.focal': { th: 'จุดโฟกัสของรูป', en: 'Focal point' },
  'mk.focalHint': { th: 'คลิก/ลากบนรูปเพื่อเลือกส่วนที่ต้องเห็นเสมอ เวลาครอปตามขนาดกรอบ (เช่น หน้าคนบนมือถือ)', en: 'Click or drag on the picture to choose what stays in view when it is cropped (e.g. a face on phones)' },
});
