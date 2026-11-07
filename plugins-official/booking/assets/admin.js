/* Bookings admin page: forms and buttons through anasaki.call('booking:…'); the page reloads after a change. */
(function () {
  'use strict';
  // in admin pages anasaki.call resolves to the action's answer: { data } or { message, errors }
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  function toast(m, kind) { if (window.AK && AK.toast) AK.toast(m, kind); }
  function call(name, input) {
    return anasaki.call('booking:' + name, input).then(function (r) {
      if (!done(r)) { toast((r && r.message) || 'Error', 'danger'); throw new Error('failed'); }
      return r;
    });
  }
  document.addEventListener('submit', function (e) {
    var f = e.target;
    var kind = f.getAttribute && f.getAttribute('data-bk-form');
    if (!kind) return;
    e.preventDefault();
    var o = {};
    var svc = [];
    new FormData(f).forEach(function (v, k) { if (k === 'svc') svc.push(v); else o[k] = v; });
    var input = kind === 'saveHours' ? { values: o } : o;
    if (kind === 'saveStaff') {
      if (o.id) input.id = Number(o.id); else delete input.id;
      if (!o.active) input.active = 'off';
      input.services = svc.join(',');
    }
    if (kind === 'saveService') {
      if (o.id) input.id = Number(o.id); else delete input.id;
      if (!o.active) input.active = 'off';
      ['minutes', 'price', 'capacity', 'max_party', 'deposit'].forEach(function (k) { if (o[k] !== undefined) input[k] = Number(o[k]); });
    }
    call(kind, input).then(function () { location.reload(); }).catch(function () {});
  });
  document.addEventListener('click', function (e) {
    var d = e.target.closest && e.target.closest('[data-ak-download]');
    if (d) {
      e.preventDefault();
      var q = {};
      d.getAttribute('data-ak-download').split('&').forEach(function (p) {
        var i = p.indexOf('=');
        if (i > 0) q[decodeURIComponent(p.slice(0, i))] = decodeURIComponent(p.slice(i + 1));
      });
      anasaki.download(q);
      return;
    }
    var b = e.target.closest && e.target.closest('[data-bk]');
    if (!b) return;
    e.preventDefault();
    var what = b.getAttribute('data-bk');
    var input = { id: Number(b.getAttribute('data-id')) };
    if (what === 'setStatus') input.status = b.getAttribute('data-status');
    b.disabled = true;
    call(what, input).then(function () { location.reload(); }).catch(function () { b.disabled = false; });
  });
})();
