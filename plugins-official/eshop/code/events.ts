// Order events for other code (site.on('eshop.order.paid', …)) and the status mails.
import { OrderRow } from './model';

export const publicOrder = (o: OrderRow) =>
  ({ id: o.id, code: o.code, status: o.status, payment: o.payment, total: Number(o.total), name: o.name, email: o.email });

/** After a status change (admin page, the unpaid-order cron). */
export function statusChanged(before: OrderRow, after: OrderRow): void {
  if (before.status === after.status) return;
  site.emit('eshop.order.status', { ...publicOrder(after), from: before.status });
  if (after.status === 'paid') site.emit('eshop.order.paid', publicOrder(after));
  if (after.status === 'paid' || after.status === 'shipped') jobs.enqueue('mailStatus', { id: after.id });
}
