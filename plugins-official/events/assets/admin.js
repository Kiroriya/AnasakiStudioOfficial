/* Event registrations admin page: check in / cancel through anasaki.call('events:…') and the CSV download. */
(function () {
  'use strict';
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-ev]');
    if (b) {
      e.preventDefault();
      b.disabled = true;
      anasaki.call('events:' + b.getAttribute('data-ev'), { id: Number(b.getAttribute('data-id')) }).then(function (r) {
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
