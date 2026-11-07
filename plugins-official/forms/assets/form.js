// forms 2.1 — progressive enhancement of the server-rendered form block (and the standalone /forms/<id> page):
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
  // 2.1: fields shown only when an earlier field has a value; hidden ones are disabled (not sent, not required)
  function valueOf(f, key) {
    var out = [];
    f.querySelectorAll('[name="' + key + '"],[name="' + key + '[]"]').forEach(function (i) {
      if (i.disabled) return;
      if (i.type === 'checkbox') { if (i.checked) out.push(i.name === key ? 'yes' : i.value); }
      else if (i.type === 'radio') { if (i.checked) out.push(i.value); }
      else if (i.value !== '') out.push(i.value);
    });
    return out;
  }
  function conditions(f) {
    f.querySelectorAll('[data-show-if]').forEach(function (w) {
      var vals = valueOf(f, w.getAttribute('data-show-if'));
      var want = w.getAttribute('data-show-value') || '';
      var show = want === '' ? vals.length > 0 : vals.indexOf(want) >= 0;
      w.hidden = !show;
      w.querySelectorAll('input,select,textarea').forEach(function (i) { i.disabled = !show; });
    });
  }
  function wire(f) {
    if (f.__rea) return;
    f.__rea = 1;
    if (f.querySelector('[data-show-if]')) {
      conditions(f);
      f.addEventListener('input', function () { conditions(f); });
      f.addEventListener('change', function () { conditions(f); });
    }
    var q = new URLSearchParams(location.search).get('sent');
    if (q && q === f.getAttribute('data-rea-form')) msg(f, f.getAttribute('data-success'), false);
    f.addEventListener('submit', function (e) {
      if (!window.fetch || !window.FormData || !window.URLSearchParams) return;
      e.preventDefault();
      var b = f.querySelector('.rea-form-submit');
      if (b) b.disabled = true;
      // 2.2: forms with a file field send multipart (the files), the others url-encoded
      var body = f.querySelector('input[type=file]') ? new FormData(f) : new URLSearchParams(new FormData(f));
      fetch(f.action, { method: 'POST', body: body, headers: { Accept: 'application/json' } })
        .then(function (r) { return r.json(); })
        .then(function (d) {
          if (d.ok) {
            if (d.redirect) { location.href = d.redirect; return; }
            f.reset();
            if (f.querySelector('[data-show-if]')) conditions(f);
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
