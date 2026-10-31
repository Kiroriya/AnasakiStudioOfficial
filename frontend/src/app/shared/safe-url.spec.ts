import { isSafeUrl, safeImageSrc, safeUrl } from './safe-url';

describe('safeUrl', () => {
  it('keeps safe links (trimmed)', () => {
    for (const u of ['https://example.com/a?b=1#c', 'http://x.test', 'mailto:a@b.co', 'tel:+6620000000', '/about', 'blog/post', '#top', '?q=1', '//cdn.example.com/x.js']) {
      expect(safeUrl(u)).toBe(u);
    }
    expect(safeUrl('  https://example.com  ')).toBe('https://example.com');
    expect(safeUrl('HTTPS://EXAMPLE.COM')).toBe('HTTPS://EXAMPLE.COM');
  });

  it('keeps empty input empty', () => {
    expect(safeUrl('')).toBe('');
    expect(safeUrl('   ')).toBe('');
    expect(safeUrl(null)).toBe('');
    expect(safeUrl(undefined)).toBe('');
  });

  it('rejects script and unknown schemes', () => {
    for (const u of ['javascript:alert(1)', 'JavaScript:alert(1)', 'vbscript:msgbox(1)', 'data:text/html,<script>alert(1)</script>', 'file:///etc/passwd', 'foo:bar']) {
      expect(safeUrl(u)).toBeNull();
    }
  });

  it('rejects obfuscated javascript: (control characters, whitespace, zero-width)', () => {
    expect(safeUrl('java\tscript:alert(1)')).toBeNull();
    expect(safeUrl('java\nscript:alert(1)')).toBeNull();
    expect(safeUrl('\u0001javascript:alert(1)')).toBeNull();
    expect(safeUrl(' javascript:alert(1)')).toBeNull();
    expect(safeUrl('java​script:alert(1)')).toBeNull();
  });

  it('rejects entity-encoded javascript: (including double-encoded)', () => {
    expect(safeUrl('&#106;avascript:alert(1)')).toBeNull();
    expect(safeUrl('&#x6A;avascript:alert(1)')).toBeNull();
    expect(safeUrl('javascript&colon;alert(1)')).toBeNull();
    expect(safeUrl('&amp;#106;avascript:alert(1)')).toBeNull();
  });

  it('isSafeUrl mirrors safeUrl', () => {
    expect(isSafeUrl('https://a.b')).toBe(true);
    expect(isSafeUrl('')).toBe(true);
    expect(isSafeUrl('javascript:x')).toBe(false);
  });
});

describe('safeImageSrc', () => {
  it('accepts inline raster images', () => {
    for (const t of ['png', 'jpeg', 'jpg', 'gif', 'webp', 'avif']) {
      const s = `data:image/${t};base64,iVBORw0KGgo=`;
      expect(safeImageSrc(s)).toBe(s);
    }
  });

  it('rejects other data: URLs, including SVG and HTML', () => {
    expect(safeImageSrc('data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==')).toBeNull();
    expect(safeImageSrc('data:text/html,<script>alert(1)</script>')).toBeNull();
    expect(safeImageSrc('data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=')).toBeNull();
    expect(safeImageSrc('data:image/png,<svg onload=alert(1)>')).toBeNull();
  });

  it('falls back to safeUrl for normal links', () => {
    expect(safeImageSrc('/uploads/a.png')).toBe('/uploads/a.png');
    expect(safeImageSrc('https://cdn.test/a.webp')).toBe('https://cdn.test/a.webp');
    expect(safeImageSrc('java\tscript:alert(1)')).toBeNull();
  });
});
