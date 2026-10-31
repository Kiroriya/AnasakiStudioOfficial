import { ApiToken } from '../core/models';
import { DEFAULT_TOKEN_EXPIRY, TOKEN_EXPIRY_DAYS, cliLoginCommand, normalizeTokens, tokenCreateBody, tokenExpired, tokenFormError } from './api-tokens-utils';

const tok = (id: number, createdAt: string, extra: Partial<ApiToken> = {}): ApiToken =>
  ({ id, name: 't' + id, prefix: 'ast_abcd', scopes: ['code'], createdAt, ...extra });

describe('API token dialog', () => {
  it('offers 30 / 90 / 365 days and never, 90 by default', () => {
    expect([...TOKEN_EXPIRY_DAYS]).toEqual([30, 90, 365, 0]);
    expect(DEFAULT_TOKEN_EXPIRY).toBe(90);
  });

  it('needs a name and a scope', () => {
    expect(tokenFormError('', ['code'])).toBe('apitok.err.name');
    expect(tokenFormError('   ', ['code'])).toBe('apitok.err.name');
    expect(tokenFormError('x'.repeat(101), ['code'])).toBe('apitok.err.nameLong');
    expect(tokenFormError('laptop', [])).toBe('apitok.err.scope');
    expect(tokenFormError(' laptop ', ['code'])).toBe('');
  });

  it('builds the POST body (trimmed name, known scopes, allowed expiry)', () => {
    expect(tokenCreateBody('  Work laptop ', 30, ['code'])).toEqual({ name: 'Work laptop', scopes: ['code'], expiresInDays: 30 });
    expect(tokenCreateBody('a', 0, ['code', 'admin'])).toEqual({ name: 'a', scopes: ['code'], expiresInDays: 0 });
    expect(tokenCreateBody('a', 7, ['code']).expiresInDays).toBe(90);
  });

  it('shows the CLI login command', () => {
    expect(cliLoginCommand('https://example.com/', 'ast_X')).toBe('npx anasaki-code login https://example.com --token ast_X');
    expect(cliLoginCommand('http://localhost:8090', 'ast_Y')).toBe('npx anasaki-code login http://localhost:8090 --token ast_Y');
  });

  it('reads the list as {items} or an array, newest first', () => {
    const a = tok(1, '2026-01-01T00:00:00Z');
    const b = tok(2, '2026-02-01T00:00:00Z');
    expect(normalizeTokens({ items: [a, b] }).map(t => t.id)).toEqual([2, 1]);
    expect(normalizeTokens([a, b]).map(t => t.id)).toEqual([2, 1]);
    expect(normalizeTokens({ items: null })).toEqual([]);
    expect(normalizeTokens(null)).toEqual([]);
  });

  it('knows when a token has expired', () => {
    const now = Date.parse('2026-06-01T00:00:00Z');
    expect(tokenExpired({ expiresAt: null }, now)).toBe(false);
    expect(tokenExpired({ expiresAt: '2026-05-31T23:59:59Z' }, now)).toBe(true);
    expect(tokenExpired({ expiresAt: '2026-06-02T00:00:00Z' }, now)).toBe(false);
  });
});
