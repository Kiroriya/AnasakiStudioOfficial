// forms 2.0 — progressive enhancement of the server-rendered form block (and the standalone /forms/<id> page):
// submits with fetch (JSON answer) and shows the message in place; without JavaScript the form posts normally
// and the visitor comes back with ?sent=<id>.
(function () {
  if (window.__reaForms) return;
  window.__reaForms = 1;
  function msg(f, t, err) {
    var m = f.querySelector('.rea-form-msg');
    if (!m) return;
    m.textContent = t;
    m.hidden = false;
    m.className = 'rea-form-msg' + (err ? ' is-error' : '');
  }
  function wire(f) {
    if (f.__rea) return;
    f.__rea = 1;
    var q = new URLSearchParams(location.search).get('sent');
    if (q && q === f.getAttribute('data-rea-form')) msg(f, f.getAttribute('data-success'), false);
    f.addEventListener('submit', function (e) {
      if (!window.fetch || !window.FormData || !window.URLSearchParams) return;
      e.preventDefault();
      var b = f.querySelector('.rea-form-submit');
      if (b) b.disabled = true;
      fetch(f.action, { method: 'POST', body: new URLSearchParams(new FormData(f)), headers: { Accept: 'application/json' } })
        .then(function (r) { return r.json(); })
        .then(function (d) {
          if (d.ok) {
            if (d.redirect) { location.href = d.redirect; return; }
            f.reset();
            msg(f, d.message, false);
          } else msg(f, d.message || 'Error', true);
        })
        .catch(function () { msg(f, 'เกิดข้อผิดพลาด กรุณาลองใหม่ / Something went wrong', true); })
        .finally(function () { if (b) b.disabled = false; });
    });
  }
  function init() { document.querySelectorAll('form[data-rea-form]').forEach(wire); }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
