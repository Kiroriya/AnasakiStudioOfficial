/* Blog extras "Comments" admin page: approve / hide / spam / delete through anasaki.call('blog-extras:moderate'). */
(function () {
  'use strict';
  // in admin pages anasaki.call resolves to the action's answer: { data } or { message, errors }
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  var data = {};
  try { data = JSON.parse(document.getElementById('bx-admin-data').textContent || '{}'); } catch (e) { /* none */ }
  function ask(text) { return window.AK && AK.confirm ? AK.confirm(text, { danger: true }) : Promise.resolve(window.confirm(text)); }
  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-bx-action]');
    if (!b) return;
    e.preventDefault();
    var action = b.getAttribute('data-bx-action');
    (action === 'delete' ? ask(data.confirmDel || 'Delete?') : Promise.resolve(true)).then(function (yes) {
      if (!yes) return;
      b.disabled = true;
      anasaki.call('blog-extras:moderate', { id: Number(b.getAttribute('data-id')), action: action }).then(function (r) {
        if (done(r)) {
          var row = document.getElementById('bx-row-' + b.getAttribute('data-id'));
          if (row) row.remove();
          if (window.AK && AK.toast) AK.toast('✓', 'success');
        } else { b.disabled = false; if (window.AK && AK.toast) AK.toast((r && r.message) || 'Error', 'danger'); }
      });
    });
  });
})();
