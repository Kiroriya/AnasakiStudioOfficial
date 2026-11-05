/* Members admin page: enable / disable (anasaki.call('members:setStatus')) and the CSV export. */
(function () {
  'use strict';
  // in admin pages anasaki.call resolves to the action's answer: { data } or { message, errors }
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-member-status]');
    if (b) {
      e.preventDefault();
      b.disabled = true;
      anasaki.call('members:setStatus', { id: Number(b.getAttribute('data-id')), status: b.getAttribute('data-member-status') })
        .then(function (r) {
          if (done(r)) location.reload();
          else { b.disabled = false; if (window.AK && AK.toast) AK.toast((r && r.message) || 'Error', 'danger'); }
        });
      return;
    }
    var x = e.target.closest && e.target.closest('[data-members="export"]');
    if (x) {
      e.preventDefault();
      var q = x.getAttribute('data-q');
      anasaki.download(q ? { q: q } : {});
    }
  });
})();
