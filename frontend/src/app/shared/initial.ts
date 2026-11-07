/**
 * The letter shown in an avatar without a picture. Thai leading vowels (เ แ โ ใ ไ) are written before the
 * consonant they belong to, so "เจ้าของร้าน" starts with จ, not เ.
 */
export function avatarInitial(text: string | null | undefined): string {
  const s = (text ?? '').trim().replace(/^[เ-ไ]+/, '');
  const first = Array.from(s)[0];
  return first ? first.toUpperCase() : '?';
}
