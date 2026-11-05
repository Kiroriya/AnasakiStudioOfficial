/* Shop "Orders" admin page: the status buttons (anasaki.call('eshop:setStatus')) and the downloads
   (anasaki.download: the CSV export and a slip). The page itself is drawn on the server. */
(function () {
  'use strict';
  // in admin pages anasaki.call resolves to the action's answer: { data } or { message, errors }
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  var data = {};
  try { data = JSON.parse(document.getElementById('shop-admin-data').textContent || '{}'); } catch (e) { /* none */ }
  var en = data.locale === 'en';
  var ASK = en ? 'Mark this order as “{s}”? The customer is mailed for paid and shipped.' : 'เปลี่ยนสถานะเป็น “{s}”? ลูกค้าจะได้รับอีเมลเมื่อเป็น ชำระแล้ว หรือ จัดส่งแล้ว';
  function toast(m, kind) { if (window.AK && AK.toast) AK.toast(m, kind); }
  function ask(text, danger) { return window.AK && AK.confirm ? AK.confirm(text, { danger: !!danger }) : Promise.resolve(window.confirm(text)); }

  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-shop-status]');
    if (b) {
      e.preventDefault();
      var s = b.getAttribute('data-shop-status');
      ask(ASK.replace('{s}', b.textContent.trim()), s === 'cancelled').then(function (yes) {
        if (!yes) return;
        b.disabled = true;
        anasaki.call('eshop:setStatus', { id: Number(b.getAttribute('data-id')), status: s }).then(function (r) {
          if (done(r)) location.reload();
          else { b.disabled = false; toast((r && r.message) || 'Error', 'danger'); }
        });
      });
      return;
    }
    var d = e.target.closest && e.target.closest('[data-ak-download]');
    if (d) {
      e.preventDefault();
      var q = {};
      d.getAttribute('data-ak-download').split('&').forEach(function (p) {
        var i = p.indexOf('=');
        if (i > 0) q[decodeURIComponent(p.slice(0, i))] = decodeURIComponent(p.slice(i + 1));
      });
      anasaki.download(q);
    }
  });
})();
