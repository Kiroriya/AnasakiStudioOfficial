/**
 * Random password from an unambiguous alphabet. Rejection sampling keeps every
 * character equally likely (a plain `n % chars.length` would favour the first ones).
 */
export function randomPassword(length: number): string {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%';
  const limit = 256 - (256 % chars.length); // bytes at or above this would bias the result
  const out: string[] = [];
  const buf = new Uint8Array(length * 2);
  while (out.length < length) {
    crypto.getRandomValues(buf);
    for (const b of buf) if (b < limit && out.length < length) out.push(chars[b % chars.length]);
  }
  return out.join('');
}
