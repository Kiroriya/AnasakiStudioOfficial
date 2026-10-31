/** Colour and shadow helpers shared by the design panels. */

export interface Shadow { inset: boolean; x: number; y: number; blur: number; spread: number; color: string }

export function parseColor(c?: string): { hex: string; a: number } | null {
  if (!c) return null;
  const s = c.trim();
  let m = /^#([0-9a-f]{3})$/i.exec(s);
  if (m) return { hex: '#' + [...m[1]].map(x => x + x).join('').toLowerCase(), a: 1 };
  m = /^#([0-9a-f]{6})([0-9a-f]{2})?$/i.exec(s);
  if (m) return { hex: '#' + m[1].toLowerCase(), a: m[2] ? parseInt(m[2], 16) / 255 : 1 };
  m = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)(?:[\s,/]+([\d.]+%?))?\s*\)$/i.exec(s);
  if (m) {
    const h = (v: string) => Math.min(255, +v).toString(16).padStart(2, '0');
    const a = m[4] === undefined ? 1 : m[4].endsWith('%') ? parseFloat(m[4]) / 100 : parseFloat(m[4]);
    return { hex: '#' + h(m[1]) + h(m[2]) + h(m[3]), a };
  }
  if (s === 'white') return { hex: '#ffffff', a: 1 };
  if (s === 'black') return { hex: '#000000', a: 1 };
  return null;
}

export function toCss(hex: string, a: number): string {
  const p = parseColor(hex);
  if (!p) return hex;
  const alpha = Math.max(0, Math.min(1, a));
  if (alpha >= 1) return p.hex;
  const n = parseInt(p.hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${Math.round(alpha * 100) / 100})`;
}

/** Splits "a, b" CSS shadow lists while respecting rgba(…) commas. */
export function parseShadows(s?: string): Shadow[] {
  if (!s) return [];
  const parts: string[] = [];
  let depth = 0, cur = '';
  for (const ch of s) {
    if (ch === '(') depth++;
    if (ch === ')') depth--;
    if (ch === ',' && depth === 0) { parts.push(cur); cur = ''; } else cur += ch;
  }
  if (cur.trim()) parts.push(cur);
  return parts.map(p => {
    const inset = /\binset\b/.test(p);
    const color = /(rgba?\([^)]*\)|#[0-9a-f]{3,8}\b|\b[a-z]+\b(?<!inset))\s*$/i.exec(p.replace(/\binset\b/, '').trim())?.[1] ?? 'rgba(0,0,0,0.25)';
    const nums = (p.replace(color, '').match(/-?\d*\.?\d+(?=px|\s|$)/g) ?? []).map(Number);
    return { inset, x: nums[0] ?? 0, y: nums[1] ?? 0, blur: nums[2] ?? 0, spread: nums[3] ?? 0, color };
  });
}

export function formatShadows(list: Shadow[]): string {
  return list.map(s => `${s.inset ? 'inset ' : ''}${s.x}px ${s.y}px ${s.blur}px ${s.spread}px ${s.color}`).join(', ');
}


/** WCAG contrast ratio of two #rrggbb colors (1…21). */
export function contrast(a: string, b: string): number {
  const lum = (hex: string) => {
    const [r, g, bl] = [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16) / 255).map(v => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
  };
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}
