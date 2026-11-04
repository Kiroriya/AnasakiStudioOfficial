// Anasaki Studio — Site Code client runtime (phases 3 and 5).
// Progressive enhancement for pages with Site Code bindings: "call code"
// actions (POST /_a/{name}), filter inputs and no-reload / load-more
// pagination (GET /_c/{postId}/{nodeId}). Pagination and filters keep
// working without JavaScript (plain links and GET forms).
// Phase 5: window.anasaki = {call, get, post, refresh, on} for Frontend code
// blocks and page scripts; DOM events anasaki:action / anasaki:refresh.
// Minified when served (design.RuntimeScript); must stay < 7 KB minified
// (6 KB until phase 5 added get/post/refresh/on).
(function () {
  var d = document, P = window.__reaP || 0, busy = {};

  function cls(id) { return 'n-' + String(id).replace(/[^a-zA-Z0-9_-]/g, ''); }
  function byId(id) { return d.querySelector('.' + cls(id)); }
  function coll(id) { return d.querySelector('[data-rea-collection="' + id + '"]'); }
  function csrf() {
    var m = d.cookie.match(/(?:^|;\s*)anasaki_csrf=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }
  function fire(t, detail, el) {
    (el || d).dispatchEvent(new CustomEvent('anasaki:' + t, { bubbles: true, detail: detail }));
  }
  function loading(els, on) {
    els.forEach(function (e) { e && e.classList.toggle('rea-loading', on); });
  }

  function toast(msg, err) {
    if (!msg) return;
    var t = d.createElement('div');
    t.className = 'rea-toast' + (err ? ' rea-err' : '');
    t.setAttribute('role', err ? 'alert' : 'status');
    t.textContent = msg;
    d.body.appendChild(t);
    setTimeout(function () { t.remove(); }, 3500);
  }

  function frag(html) {
    var t = d.createElement('template');
    t.innerHTML = html;
    return t.content;
  }

  // swap replaces each returned node (outerHTML), keeping the focused field
  function swap(nodes) {
    Object.keys(nodes || {}).forEach(function (id) {
      var el = byId(id), html = nodes[id];
      if (!el) return;
      if (!html) { el.hidden = true; el.innerHTML = ''; return; }
      var a = d.activeElement, name = a && el.contains(a) && a.name, pos;
      try { pos = name && a.selectionStart; } catch (e) {}
      el.replaceWith(frag(html));
      if (name) {
        var n = byId(id);
        var f = n && n.querySelector('[name="' + name + '"]');
        if (f) {
          f.focus({ preventScroll: true });
          try { if (pos != null) f.setSelectionRange(pos, pos); } catch (e) {}
        }
      }
    });
  }

  function json(r) {
    return r.json().catch(function () { return {}; }).then(function (b) { b._s = r.status; return b; });
  }

  // part re-renders a collection (and its pagination) for the query search
  function part(id, search, more) {
    var c = coll(id);
    if (busy[id]) busy[id].abort();
    var ac = busy[id] = new AbortController();
    loading([c], true);
    return fetch('/_c/' + P + '/' + encodeURIComponent(id) + search, {
      headers: { 'X-Rea-Path': location.pathname, Accept: 'application/json' },
      credentials: 'same-origin', signal: ac.signal
    }).then(json).then(function (b) {
      var nodes = b.nodes || {};
      if (more && c && nodes[id]) {
        var n = frag(nodes[id]).firstElementChild;
        while (n && n.firstChild) c.appendChild(n.firstChild);
        delete nodes[id];
      }
      swap(nodes);
      fire('refresh', { collection: id });
    }).catch(function (e) { if (e.name != 'AbortError') toast('โหลดไม่สำเร็จ / Failed to load', 1); })
      .finally(function () { loading([c, coll(id)], false); });
  }

  // run calls a site.action from a trigger element
  function run(el) {
    var name = el.getAttribute('data-rea-action'), form = el.closest('form[data-rea-form]'), input = {};
    var msg = el.getAttribute('data-rea-confirm');
    if (msg && !confirm(msg)) return;
    if (form) {
      if (!form.checkValidity()) { form.reportValidity(); return; }
      [].forEach.call(form.elements, function (f) {
        if (!f.name || f.disabled) return;
        if (f.type == 'checkbox') input[f.name] = f.checked;
        else if (f.type != 'radio' || f.checked) input[f.name] = f.value;
      });
    }
    try { Object.assign(input, JSON.parse(el.getAttribute('data-rea-input') || '{}')); } catch (e) {}
    var up = (el.getAttribute('data-rea-update') || '').split(' ').filter(Boolean);
    var targets = [el].concat(up.map(byId));
    var scope = form || d;
    [].forEach.call(scope.querySelectorAll('[data-rea-error-for]'), function (e) { e.textContent = ''; });
    loading(targets, true);
    return call(name, input, up).then(function (b) {
      Object.keys(b.errors || {}).forEach(function (k) {
        var e = scope.querySelector('[data-rea-error-for="' + k + '"]') || d.querySelector('[data-rea-error-for="' + k + '"]');
        if (e) e.textContent = b.errors[k];
      });
      swap(b.nodes);
      var ok = b._s < 300 && !b.errors;
      toast(b.error || b.message || (ok && el.getAttribute('data-rea-success')), !ok && !b.message);
      var to = b.redirect || (ok && el.getAttribute('data-rea-redirect'));
      if (to) location.assign(to);
      fire('action', { name: name, result: b }, el);
    }).catch(function () { toast('ทำรายการไม่สำเร็จ / Failed', 1); })
      .finally(function () { loading(targets.map(function (t) { return t && t.isConnected ? t : null; }), false); });
  }

  function call(name, input, update) {
    var q = {};
    new URLSearchParams(location.search).forEach(function (v, k) { q[k] = v; });
    return fetch('/_a/' + encodeURIComponent(name), {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf(), Accept: 'application/json' },
      body: JSON.stringify({ input: input || {}, ctx: { postId: P, path: location.pathname, query: q }, update: update || [] })
    }).then(json);
  }
  // route requests for page scripts: same origin only, JSON in and out
  function req(m, u, b) {
    if (!/^\/(?!\/)/.test(u)) return Promise.reject(new Error('same-origin path only'));
    var h = { Accept: 'application/json' };
    if (m != 'GET') { h['Content-Type'] = 'application/json'; h['X-CSRF-Token'] = csrf(); }
    return fetch(u, { method: m, credentials: 'same-origin', headers: h, body: b }).then(function (r) {
      return r.text().then(function (t) {
        try { t = JSON.parse(t); } catch (e) {}
        if (r.ok) return t;
        var e = new Error('HTTP ' + r.status); e.status = r.status; e.body = t; throw e;
      });
    });
  }
  // one load per file per page; the promise rejects on a bad name or a failed load
  var assets = {};
  function asset(n, ext, make) {
    n = String(n || '');
    if (!/^[A-Za-z0-9_-][A-Za-z0-9._/-]*$/.test(n) || n.indexOf('..') >= 0) return Promise.reject(new Error('bad file name: ' + n));
    if (n.slice(-ext.length) !== ext) n += ext;
    if (!assets[n]) assets[n] = new Promise(function (ok, no) {
      var el = make('/site-assets/' + n);
      el.onload = function () { ok(); };
      el.onerror = function () { delete assets[n]; no(new Error('cannot load /site-assets/' + n)); };
      d.head.appendChild(el);
    });
    return assets[n];
  }
  window.anasaki = {
    call: function (n, i) {
      return call(n, i).then(function (b) {
        swap(b.nodes);
        var r = { ok: b._s < 300 && !b.errors, data: b.data, errors: b.errors, message: b.message || b.error, redirect: b.redirect, status: b._s };
        fire('action', { name: n, result: r });
        return r;
      });
    },
    get: function (u, p) {
      var q = new URLSearchParams(p || {}) + '';
      return req('GET', u + (q ? (u.indexOf('?') < 0 ? '?' : '&') + q : ''));
    },
    post: function (u, b) { return req('POST', u, JSON.stringify(b === undefined ? {} : b)); },
    refresh: function (n) {
      return Promise.all([].map.call(d.querySelectorAll('[data-rea-source="code:' + n + '"],[data-rea-source="' + n + '"],[data-rea-collection="' + n + '"]'),
        function (c) { return part(c.getAttribute('data-rea-collection'), location.search); }));
    },
    // shared files of the site (2.2.0): Site Code's public/ folder, served at /site-assets/. lib('qr') runs
    // public/qr.js once for the whole page (every code block awaits the same promise); style('base') adds public/base.css
    lib: function (n) { return asset(n, '.js', function (u) { var s = d.createElement('script'); s.src = u; return s; }); },
    style: function (n) { return asset(n, '.css', function (u) { var l = d.createElement('link'); l.rel = 'stylesheet'; l.href = u; return l; }); },
    // a layer by its "name for code" (data-ref, 2.2.0); refs: every match (component placements, lists)
    ref: function (n) { return d.querySelector('[data-ref="' + CSS.escape(n) + '"]'); },
    refs: function (n) { return [].slice.call(d.querySelectorAll('[data-ref="' + CSS.escape(n) + '"]')); },
    on: function (t, f) {
      t = t.indexOf(':') < 0 ? 'anasaki:' + t : t;
      function h(e) { f(e.detail, e); }
      d.addEventListener(t, h);
      return function () { d.removeEventListener(t, h); };
    }
  };

  // filters: ?<key>_<param>=value, back to page 1, no reload
  var timer;
  function filter(f, now) {
    clearTimeout(timer);
    timer = setTimeout(function () {
      var p = new URLSearchParams(location.search), v = f.type == 'checkbox' ? (f.checked ? '1' : '') : f.value;
      if (v) p.set(f.name, v); else p.delete(f.name);
      p.delete(f.getAttribute('data-rea-reset'));
      p.delete('page');
      var s = p.toString(), search = s ? '?' + s : '';
      history.replaceState(history.state, '', location.pathname + search);
      part(f.getAttribute('data-rea-filter'), search);
    }, now ? 0 : 300);
  }

  d.addEventListener('click', function (e) {
    var t = e.target.closest && e.target.closest('[data-rea-action],[data-rea-pagination] a[href]');
    if (!t || e.button || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (t.hasAttribute('data-rea-action')) { e.preventDefault(); run(t); return; }
    var nav = t.closest('[data-rea-pagination]'), id = nav.getAttribute('data-rea-pagination'), c = coll(id);
    var more = t.hasAttribute('data-rea-more');
    if (!c || !(more || c.hasAttribute('data-rea-noreload'))) return;
    e.preventDefault();
    var u = new URL(t.href, location.href);
    if (more) { part(id, u.search, 1); return; }
    history.pushState({ rea: id }, '', u.pathname + u.search);
    part(id, u.search).then(function () {
      var n = coll(id);
      if (n && n.getBoundingClientRect().top < 0) n.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  });
  d.addEventListener('keydown', function (e) {
    var t = e.target;
    if ((e.key == 'Enter' || e.key == ' ') && t.hasAttribute && t.hasAttribute('data-rea-action') && !/^(A|BUTTON)$/.test(t.tagName)) {
      e.preventDefault(); run(t);
    }
  });
  d.addEventListener('input', function (e) {
    var f = e.target;
    if (f.hasAttribute && f.hasAttribute('data-rea-filter') && f.tagName != 'SELECT' && f.type != 'checkbox') filter(f);
  });
  d.addEventListener('change', function (e) {
    var f = e.target;
    if (f.hasAttribute && f.hasAttribute('data-rea-filter') && (f.tagName == 'SELECT' || f.type == 'checkbox')) filter(f, 1);
  });
  d.addEventListener('submit', function (e) {
    var form = e.target, f = form.querySelector('[data-rea-filter]');
    if (f) { e.preventDefault(); filter(f, 1); return; }
    var a = form.matches('[data-rea-form]') && form.querySelector('[data-rea-action]');
    if (a) { e.preventDefault(); run(a); }
  });
  addEventListener('popstate', function (e) {
    if (e.state && e.state.rea) part(e.state.rea, location.search);
  });
  [].forEach.call(d.querySelectorAll('[data-rea-action]'), function (t) {
    if (!/^(A|BUTTON)$/.test(t.tagName)) { t.setAttribute('role', 'button'); t.tabIndex = 0; }
  });
})();
