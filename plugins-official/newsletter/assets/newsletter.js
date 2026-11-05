/* Newsletter form block: subscribe without leaving the page (the form also works without this script). */
(function () {
  'use strict';
  if (window.__anasakiNewsletter) return;
  window.__anasakiNewsletter = true;
  document.addEventListener('submit', function (e) {
    var f = e.target;
    if (!(f.matches && f.matches('form[data-nl-form]'))) return;
    e.preventDefault();
    var data = {};
    new FormData(f).forEach(function (v, k) { data[k] = v; });
    var b = f.querySelector('button[type=submit]');
    var msg = f.querySelector('.nl-msg');
    b.disabled = true;
    fetch(f.action, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(data) })
      .then(function (r) { return r.json(); })
      .then(function (j) {
        b.disabled = false;
        msg.hidden = false;
        msg.className = 'nl-msg' + (j.ok ? '' : ' nl-bad');
        msg.textContent = j.message || '';
        if (j.ok) f.querySelector('.nl-row').hidden = true;
      })
      .catch(function () { b.disabled = false; f.submit(); });
  });
})();
