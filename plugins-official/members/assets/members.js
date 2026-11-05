/* Members on public pages: the sign-in / register / forgot / reset / profile / sign-out forms (JSON posts to
   /members/*, which is also their CSRF protection) and the member menu block (filled from GET /members/me). */
(function () {
  'use strict';
  if (window.__anasakiMembers) return;
  window.__anasakiMembers = true;

  var ROUTES = { login: '/members/login', register: '/members/register', forgot: '/members/forgot', reset: '/members/reset', profile: '/members/profile', logout: '/members/logout' };

  function show(form, ok, text) {
    var p = form.querySelector('.mb-msg');
    if (!p) return;
    p.hidden = !text;
    p.className = 'mb-msg ' + (ok ? 'mb-ok' : 'mb-bad');
    p.textContent = text || '';
  }

  document.addEventListener('submit', function (e) {
    var f = e.target;
    var kind = f.getAttribute && f.getAttribute('data-members');
    if (!kind || !ROUTES[kind]) return;
    e.preventDefault();
    var data = {};
    new FormData(f).forEach(function (v, k) { data[k] = v; });
    f.querySelectorAll('.mb-field').forEach(function (d) {
      d.classList.remove('is-invalid');
      var s = d.querySelector('.mb-err');
      if (s) { s.hidden = true; s.textContent = ''; }
    });
    var b = f.querySelector('button[type=submit]');
    if (b) b.disabled = true;
    fetch(ROUTES[kind], { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(data) })
      .then(function (r) { return r.json().catch(function () { return {}; }).then(function (j) { j.httpStatus = r.status; return j; }); })
      .then(function (j) {
        if (b) b.disabled = false;
        var ok = j.httpStatus < 300 && j.ok !== false;
        show(f, ok, j.message);
        if (j.errors) Object.keys(j.errors).forEach(function (k) {
          var d = f.querySelector('.mb-field[data-field="' + k + '"]');
          if (!d) return;
          d.classList.add('is-invalid');
          var s = d.querySelector('.mb-err');
          if (s) { s.hidden = false; s.textContent = j.errors[k]; }
        });
        if (!ok) return;
        if (kind === 'register' || kind === 'forgot') { f.reset(); if (kind === 'register' && j.data && j.data.signedIn) location.href = '/members/account'; return; }
        var next = f.getAttribute('data-next');
        if (next) location.href = next;
      })
      .catch(function () { if (b) b.disabled = false; show(f, false, 'Network error'); });
  });

  var menus = document.querySelectorAll('[data-members-menu]');
  if (menus.length) {
    fetch('/members/me', { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (j) {
      var v = j && j.visitor;
      if (!v) return;
      menus.forEach(function (m) {
        m.textContent = '';
        var a = document.createElement('a');
        a.href = '/members/account';
        a.textContent = '👤 ' + (v.name || m.getAttribute('data-account') || 'Account');
        m.appendChild(a);
      });
    }).catch(function () {});
  }
})();
