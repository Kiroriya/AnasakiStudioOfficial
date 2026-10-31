// HTTP client for /api/v1 (bearer token, JSON).
import { CliError } from './errors.mjs';

export class ApiError extends Error {
  constructor(status, body, message) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

export function encodePath(p) {
  return p.split('/').map(encodeURIComponent).join('/');
}

export function createClient({ site, token, version = '0', fetchImpl = globalThis.fetch, t }) {
  const base = site.replace(/\/+$/, '') + '/api/v1';

  async function request(method, p, { body, query, raw = false, signal } = {}) {
    let url = base + p;
    if (query) {
      const qs = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== null && v !== '') qs.set(k, String(v));
      const s = qs.toString();
      if (s) url += (url.includes('?') ? '&' : '?') + s;
    }
    const headers = { Accept: raw ? '*/*' : 'application/json', 'User-Agent': `anasaki-code/${version}` };
    if (token) headers.Authorization = `Bearer ${token}`;
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    let res;
    try {
      res = await fetchImpl(url, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal });
    } catch (e) {
      if (e && e.name === 'AbortError') throw e;
      const why = (e && e.cause && (e.cause.code || e.cause.message)) || (e && e.message) || String(e);
      throw new CliError(t('err.network', { site, msg: why }), 1);
    }
    const text = await res.text();
    if (res.ok && raw) return text;
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch { data = null; }
    }
    if (!res.ok) {
      const msg = (data && (data.error || data.message)) || text.slice(0, 300) || res.statusText;
      throw new ApiError(res.status, data, msg);
    }
    return data;
  }

  return {
    site,
    request,
    me: () => request('GET', '/auth/me'),
    tree: () => request('GET', '/code/tree'),
    file: (p) => request('GET', '/code/files/' + encodePath(p)),
    sync: (payload) => request('POST', '/code/sync', { body: payload }),
    status: () => request('GET', '/code/status'),
    build: () => request('POST', '/code/build', { body: {} }),
    publish: (payload) => request('POST', '/code/publish', { body: payload }),
    logs: (query, signal) => request('GET', '/code/logs', { query, signal }),
    types: () => request('GET', '/code/types', { raw: true }),
  };
}

/** apiFailure converts an ApiError into a CliError with a helpful message. */
export function apiFailure(e, t, site) {
  if (!(e instanceof ApiError)) return e;
  switch (e.status) {
    case 401:
      return new CliError(t('err.unauthorized', { msg: e.message, site }), 1);
    case 403:
      return new CliError(t('err.forbidden', { msg: e.message }), 1);
    case 400:
    case 409:
    case 413:
    case 422:
      return new CliError(t('err.http', { status: e.status, msg: e.message }), 2);
    default:
      return new CliError(t('err.http', { status: e.status, msg: e.message }), 1);
  }
}
