/* Donations admin page: confirm / cancel a gift through anasaki.call('donations:setStatus'), and the downloads
   (the CSV export, a slip) through anasaki.download. The page is drawn on the server. */
(function () {
  'use strict';
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-dn]');
    if (b) {
      e.preventDefault();
      b.disabled = true;
      anasaki.call('donations:setStatus', { id: Number(b.getAttribute('data-id')), status: b.getAttribute('data-dn') }).then(function (r) {
        if (done(r)) location.reload();
        else { b.disabled = false; if (window.AK && AK.toast) AK.toast((r && r.message) || 'Error', 'danger'); }
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
