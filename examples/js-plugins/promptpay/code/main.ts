// The promptpay unit: a public QR route for the shop owner's own PromptPay ID (setting "promptpayId",
// Site code → Settings → unit "promptpay"). Importers use the library (code/lib) directly.
import { payload, qrSvg } from './lib/index';

function build(query: Record<string, string | string[]>): { payload: string } | { error: string } {
  const id = site.settings('promptpayId');
  if (!id) return { error: 'promptpayId is not set (Site code → Settings → promptpay)' };
  const raw = Array.isArray(query.amount) ? query.amount[0] : query.amount;
  if (raw !== undefined && raw !== '' && !/^\d{1,9}(\.\d{1,2})?$/.test(raw)) return { error: 'amount: up to 2 decimals' };
  try {
    return { payload: payload({ id, amount: raw || undefined }) };
  } catch (e) {
    return { error: String((e as Error).message) };
  }
}

// <img src="/promptpay/qr.svg?amount=150"> — cached by the browser for an hour.
site.route('GET /promptpay/qr.svg', { access: ['*'] }, ({ request }) => {
  const r = build(request.query);
  if ('error' in r) return { status: 400, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: r.error };
  const size = Math.min(1024, Math.max(64, Number(request.query.size) || 256));
  return {
    status: 200,
    headers: { 'Content-Type': 'image/svg+xml', 'Cache-Control': 'public, max-age=3600' },
    body: qrSvg(r.payload, { size }),
  };
});

// The payload as JSON (for a client-side QR library or a banking app deep link).
site.route('GET /promptpay/payload', { access: ['*'] }, ({ request }) => {
  const r = build(request.query);
  return 'error' in r ? { status: 400, body: r } : r;
});

// For plugins that use promptpay only when it is installed (manifest "optionalRequires" + plugins.has /
// plugins.call, e.g. the shop sample): a QR for the shop owner's own PromptPay ID (this unit's
// "promptpayId" setting), or null when it is not set. Runs here, with promptpay's settings.
//   plugins.call('promptpay', 'qrFor', 150, { size: 200 }) → { payload, svg, url } | null
site.export('qrFor', (amount?: number | string, opts?: { size?: number }) => {
  const id = site.settings('promptpayId');
  if (!id) return null;
  const amt = amount === undefined || amount === null || amount === '' ? undefined : Number(amount).toFixed(2);
  const p = payload({ id, amount: amt });
  const size = Math.min(1024, Math.max(64, Number(opts?.size) || 256));
  return { payload: p, svg: qrSvg(p, { size }), url: '/promptpay/qr.svg' + (amt ? '?amount=' + amt : '') };
});
