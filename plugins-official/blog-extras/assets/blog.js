/* Blog extras on public pages: "copy link", replying to a comment, and posting a comment without leaving the
   page. The form also works without this script (it shows a small page with a link back). */
(function () {
  'use strict';
  if (window.__anasakiBlogExtras) return;
  window.__anasakiBlogExtras = true;

  // the part below a post (author, related posts, comments) when the page was rendered before it was cached
  function loadMore() {
    document.querySelectorAll('[data-bx-more]').forEach(function (el) {
      fetch(el.getAttribute('data-bx-more'), { credentials: 'same-origin' })
        .then(function (r) { return r.ok ? r.text() : ''; })
        .then(function (html) {
          if (!html) { el.remove(); return; }
          el.outerHTML = html;
          if (location.hash && location.hash.indexOf('#comment') === 0) {
            var target = document.querySelector(location.hash);
            if (target) target.scrollIntoView();
          }
        })
        .catch(function () { el.remove(); });
    });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', loadMore);
  else loadMore();

  document.addEventListener('click', function (e) {
    var copy = e.target.closest && e.target.closest('[data-bx-copy]');
    if (copy) {
      var url = copy.getAttribute('data-bx-copy');
      var done = function () { var old = copy.textContent; copy.textContent = '✓ ' + (copy.getAttribute('data-done') || 'Copied'); setTimeout(function () { copy.textContent = old; }, 1600); };
      if (navigator.share && /Mobi|Android/i.test(navigator.userAgent)) { navigator.share({ url: url, title: document.title }).catch(function () {}); return; }
      if (navigator.clipboard) navigator.clipboard.writeText(url).then(done, function () { prompt('', url); });
      else prompt('', url);
      return;
    }
    var reply = e.target.closest && e.target.closest('[data-bx-reply]');
    var form = document.getElementById('bx-comment-form');
    if (reply && form) {
      form.querySelector('[name=parent]').value = reply.getAttribute('data-bx-reply');
      var r = form.querySelector('.bx-replying');
      r.hidden = false;
      r.querySelector('span').textContent = '↩ ' + reply.textContent + ' ' + (reply.getAttribute('data-name') || '');
      form.querySelector('textarea').focus();
      return;
    }
    if (e.target.closest && e.target.closest('[data-bx-cancel]') && form) {
      form.querySelector('[name=parent]').value = '0';
      form.querySelector('.bx-replying').hidden = true;
    }
  });

  document.addEventListener('submit', function (e) {
    var f = e.target;
    if (!(f.matches && f.matches('#bx-comment-form'))) return;
    e.preventDefault();
    var b = f.querySelector('button[type=submit]');
    b.disabled = true;
    var data = {};
    new FormData(f).forEach(function (v, k) { data[k] = v; });
    fetch(f.action, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(data) })
      .then(function (r) { return r.json().then(function (j) { j.status = r.status; return j; }); })
      .then(function (j) {
        b.disabled = false;
        var old = f.parentNode.querySelector('.bx-note, .bx-err');
        if (old) old.remove();
        var p = document.createElement('p');
        p.className = j.status < 300 ? 'bx-note' : 'bx-err';
        p.setAttribute('role', 'status');
        p.textContent = j.message + (j.errors ? ' — ' + Object.keys(j.errors).map(function (k) { return j.errors[k]; }).join(' · ') : '');
        f.parentNode.insertBefore(p, f);
        if (j.status === 201) {
          f.reset();
          f.querySelector('[name=parent]').value = '0';
          f.querySelector('.bx-replying').hidden = true;
          if (j.posted) location.reload();
        }
      })
      .catch(function () { b.disabled = false; f.submit(); });
  });
})();
