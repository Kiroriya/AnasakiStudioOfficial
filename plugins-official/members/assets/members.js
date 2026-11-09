/* Members on public pages: the sign-in / register / forgot / reset / profile / sign-out forms (JSON posts to
   /members/*, which is also their CSRF protection) and the member menu block (filled from GET /members/me). */
(function () {
  'use strict';
  if (window.__anasakiMembers) return;
  window.__anasakiMembers = true;

  var ROUTES = { login: '/members/login', register: '/members/register', forgot: '/members/forgot', reset: '/members/reset', profile: '/members/profile', password: '/members/password', resend: '/members/resend', logout: '/members/logout' };
  function post(path, data) {
    return fetch(path, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(data) })
      .then(function (r) { return r.json().catch(function () { return {}; }); });
  }
  function el(tag, cls, text) { var x = document.createElement(tag); if (cls) x.className = cls; if (text != null) x.textContent = text; return x; }
  /** A "send again" button with a 60-second wait between sends. */
  function resendButton(form, email) {
    var b = el('button', 'mb-btn mb-btn-ghost', form.getAttribute('data-resend') || 'Send again');
    b.type = 'button';
    var wait = function (s) {
      var label = form.getAttribute('data-resend') || 'Send again', w = form.getAttribute('data-wait');
      b.disabled = s > 0;
      b.textContent = s > 0 && w ? w.replace('{s}', s) : label;
      if (s > 0) setTimeout(function () { wait(s - 1); }, 1000);
    };
    b.onclick = function () { b.disabled = true; post(ROUTES.resend, { email: email }).then(function (j) { show(form, j.ok !== false, j.message); wait(60); }); };
    return { button: b, wait: wait };
  }

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
        // 1.4.0: not confirmed yet — a fresh link went out; offer to send it again
        if (!ok && kind === 'login' && j.data && j.data.unverified && !f.querySelector('.mb-resend')) {
          var r = resendButton(f, j.data.email || data.email);
          r.button.classList.add('mb-resend');
          r.button.style.marginTop = '10px';
          f.appendChild(r.button);
          r.wait(60);
        }
        if (!ok) return;
        if (kind === 'register' || kind === 'forgot' || kind === 'password') {
          f.reset();
          if (kind === 'register' && j.data && j.data.signedIn) { location.href = '/members/account'; return; }
          // 1.4.0: after signing up, the form becomes "check your e-mail" — the address, send again, and the way in
          if (kind === 'register') {
            f.querySelectorAll('.mb-field, button[type=submit], .mb-msg').forEach(function (x) { x.hidden = true; x.style.display = 'none'; });
            var email = (j.data && j.data.email) || data.email || '';
            var box = el('div', 'mb-sent');
            box.setAttribute('role', 'status');
            box.style.cssText = 'text-align:center;padding:8px 0 4px';
            var icon = el('div', '', '✉️'); icon.style.cssText = 'font-size:44px;line-height:1;margin-bottom:10px';
            var h = el('h2', '', f.getAttribute('data-sent-title') || 'Check your e-mail'); h.style.cssText = 'margin:0 0 8px;font-size:1.35rem;font-weight:700';
            var p = el('p', 'mb-sub', (f.getAttribute('data-sent-text') || '{email}').replace('{email}', email)); p.style.cssText = 'margin:0 0 16px;line-height:1.6';
            var msg = el('p', 'mb-msg'); msg.hidden = true;
            box.appendChild(icon); box.appendChild(h); box.appendChild(p); box.appendChild(msg);
            var again = resendButton(f, email);
            again.button.style.marginBottom = '10px';
            box.appendChild(again.button);
            var go = el('a', 'mb-btn', f.getAttribute('data-done') || 'Sign in');
            go.href = '/members/login'; go.style.cssText = 'display:flex;align-items:center;justify-content:center;text-decoration:none';
            box.appendChild(go);
            f.appendChild(box);
            // the "already have an account? sign in" line under the form repeats the button above
            var after = f.nextElementSibling;
            if (after && after.querySelector && after.querySelector('a[href="/members/login"]')) after.hidden = true;
            again.wait(60);
            f.scrollIntoView({ behavior: 'smooth', block: 'start' });
          }
          return;
        }
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
