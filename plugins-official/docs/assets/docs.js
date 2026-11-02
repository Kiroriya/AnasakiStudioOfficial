/* Docs plugin — public pages: mobile drawer, product switcher, search dialog (client-side index from
   /docs/search.json), TOC scroll-spy, copy buttons on code blocks, sticky offset under a fixed site header.
   No dependencies; every text from the index goes in with textContent. */
(function () {
  'use strict';
  var root = document.querySelector('[data-dx]');
  if (!root) return;
  var cfg = {};
  try { cfg = JSON.parse((document.getElementById('dx-data') || {}).textContent || '{}'); } catch (e) { /* defaults */ }
  var T = cfg.t || {};
  var $ = function (sel, el) { return (el || root).querySelector(sel); };
  var $$ = function (sel, el) { return Array.prototype.slice.call((el || root).querySelectorAll(sel)); };
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  // ------------------------------------------------------------ sticky offset under a fixed/sticky site header
  function measureTop() {
    var top = 0;
    var x = Math.round(window.innerWidth / 2);
    var stack = document.elementsFromPoint ? document.elementsFromPoint(x, 2) : [];
    for (var i = 0; i < stack.length; i++) {
      var n = stack[i];
      if (root.contains(n)) continue;
      for (var p = n; p && p !== document.body; p = p.parentElement) {
        var pos = getComputedStyle(p).position;
        if (pos === 'fixed' || pos === 'sticky') {
          var r = p.getBoundingClientRect();
          if (r.top <= 1 && r.bottom > top && r.height < window.innerHeight / 3) top = r.bottom;
          break;
        }
      }
    }
    root.style.setProperty('--dx-top', Math.round(top) + 'px');
  }
  measureTop();
  window.addEventListener('resize', measureTop);

  // ------------------------------------------------------------ drawer (mobile sidebar)
  var openBtn = $('[data-dx-open]');
  var sidebar = $('#dx-sidebar');
  var scrim = $('.dx-scrim');
  var lastFocus = null;
  function setDrawer(open) {
    if (!sidebar) return;
    root.classList.toggle('is-open', open);
    if (scrim) scrim.hidden = !open;
    if (openBtn) openBtn.setAttribute('aria-expanded', String(open));
    document.documentElement.style.overflow = open ? 'hidden' : '';
    if (open) {
      lastFocus = document.activeElement;
      var cur = $('.dx-nav a.is-active', sidebar) || $('a, button', sidebar);
      setTimeout(function () { if (cur) { cur.focus({ preventScroll: true }); cur.scrollIntoView({ block: 'center' }); } }, 60);
    } else if (lastFocus && lastFocus.focus) {
      lastFocus.focus();
    }
  }
  if (openBtn) openBtn.addEventListener('click', function () { setDrawer(true); });
  $$('[data-dx-close]').forEach(function (b) { b.addEventListener('click', function () { setDrawer(false); }); });
  if (sidebar) {
    sidebar.addEventListener('keydown', function (e) {
      if (!root.classList.contains('is-open') || e.key !== 'Tab') return;
      var f = $$('a[href], button:not([disabled]), input', sidebar).filter(function (n) { return n.offsetParent !== null; });
      if (!f.length) return;
      if (e.shiftKey && document.activeElement === f[0]) { e.preventDefault(); f[f.length - 1].focus(); }
      else if (!e.shiftKey && document.activeElement === f[f.length - 1]) { e.preventDefault(); f[0].focus(); }
    });
  }
  // the current page in view inside the sidebar (desktop)
  var active = $('.dx-nav a.is-active');
  if (active && sidebar && window.innerWidth > 860) {
    var sr = sidebar.getBoundingClientRect(), ar = active.getBoundingClientRect();
    if (ar.bottom > sr.bottom - 40) sidebar.scrollTop += ar.top - sr.top - sr.height / 3;
  }

  // ------------------------------------------------------------ product switcher
  $$('[data-dx-switch]').forEach(function (sw) {
    var btn = $('.dx-switch-btn', sw), menu = $('.dx-switch-menu', sw);
    if (!btn || !menu) return;
    var items = function () { return $$('a', menu); };
    function set(open, focusFirst) {
      menu.hidden = !open;
      btn.setAttribute('aria-expanded', String(open));
      if (open && focusFirst) { var a = $('a.is-active', menu) || items()[0]; if (a) a.focus(); }
    }
    btn.addEventListener('click', function () { set(menu.hidden, false); });
    btn.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); set(true, true); }
    });
    menu.addEventListener('keydown', function (e) {
      var list = items(), i = list.indexOf(document.activeElement);
      if (e.key === 'ArrowDown') { e.preventDefault(); list[(i + 1) % list.length].focus(); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); list[(i - 1 + list.length) % list.length].focus(); }
      else if (e.key === 'Escape') { e.preventDefault(); set(false); btn.focus(); }
      else if (e.key === 'Tab') set(false);
    });
    document.addEventListener('click', function (e) { if (!sw.contains(e.target)) set(false); });
  });

  // ------------------------------------------------------------ search dialog
  var index = null, loading = null;
  function load() {
    if (index) return Promise.resolve(index);
    if (!loading) {
      loading = fetch(cfg.search || '/docs/search.json', { headers: { Accept: 'application/json' } })
        .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
        .then(function (d) {
          index = (d.items || []).map(function (it) {
            var hs = (it.headings || []).map(function (h) { return typeof h === 'string' ? { id: '', text: h } : h; });
            return { it: it, hs: hs, t: norm(it.title), e: norm(it.excerpt), s: norm(it.section + ' ' + it.productName), h: hs.map(function (h) { return norm(h.text); }) };
          });
          return index;
        })
        .catch(function (e) { loading = null; throw e; });
    }
    return loading;
  }
  function norm(s) { return String(s || '').toLowerCase().normalize('NFC'); }
  function icons() {
    var m = {};
    $$('.dx-switch-item').forEach(function (a) {
      var id = (a.getAttribute('href') || '').split('/').pop();
      var ic = $('.dx-picon', a);
      if (id && ic) m[id] = ic.textContent;
    });
    return m;
  }
  function search(q) {
    var terms = norm(q).trim().split(/\s+/).filter(Boolean);
    if (!terms.length) return [];
    var full = terms.join(' ');
    var out = [];
    index.forEach(function (d) {
      var score = 0, head = null;
      for (var i = 0; i < terms.length; i++) {
        var w = terms[i], hit = 0;
        if (d.t.indexOf(w) >= 0) hit += d.t.indexOf(w) === 0 ? 14 : 10;
        for (var j = 0; j < d.h.length; j++) if (d.h[j].indexOf(w) >= 0) { hit += 5; if (!head) head = d.hs[j]; break; }
        if (d.e.indexOf(w) >= 0) hit += 3;
        if (d.s.indexOf(w) >= 0) hit += 2;
        if (!hit) return;
        score += hit;
      }
      if (d.t.indexOf(full) >= 0) score += 8;
      if (d.t === full) score += 20;
      if (head && d.t.indexOf(full) >= 0) head = null; // the title itself matched: open the page top
      out.push({ d: d, score: score, head: head });
    });
    out.sort(function (a, b) { return b.score - a.score; });
    return out.slice(0, 12);
  }
  function highlight(node, text, q) {
    var terms = norm(q).trim().split(/\s+/).filter(Boolean);
    var lower = norm(text), pos = 0, marks = [];
    terms.forEach(function (w) { var i = lower.indexOf(w); if (i >= 0) marks.push([i, i + w.length]); });
    marks.sort(function (a, b) { return a[0] - b[0]; });
    marks.forEach(function (m) {
      if (m[0] < pos) return;
      node.appendChild(document.createTextNode(text.slice(pos, m[0])));
      node.appendChild(el('mark', '', text.slice(m[0], m[1])));
      pos = m[1];
    });
    node.appendChild(document.createTextNode(text.slice(pos)));
    return node;
  }

  var SEARCH_SVG = '<svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true"><circle cx="9" cy="9" r="6" fill="none" stroke="currentColor" stroke-width="1.7"/><path d="M13.5 13.5L17 17" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>';
  var GO_SVG = '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M6 3l5 5-5 5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  var dlg = null, input = null, list = null, sel = -1, hits = [], opener = null;
  function buildDialog() {
    dlg = el('div', 'dx dx-dialog');
    dlg.hidden = true;
    dlg.setAttribute('role', 'dialog');
    dlg.setAttribute('aria-modal', 'true');
    dlg.setAttribute('aria-label', T.label || 'Search');
    var panel = el('div', 'dx-dialog-panel');
    var head = el('div', 'dx-dialog-head');
    head.innerHTML = SEARCH_SVG;
    input = el('input');
    input.type = 'search';
    input.placeholder = T.placeholder || 'Search…';
    input.setAttribute('aria-label', T.label || 'Search');
    input.setAttribute('autocomplete', 'off');
    input.setAttribute('spellcheck', 'false');
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-controls', 'dx-hits');
    input.setAttribute('aria-expanded', 'true');
    var esc = el('button', 'dx-kbd', 'Esc');
    esc.type = 'button';
    esc.style.cursor = 'pointer';
    esc.setAttribute('aria-label', T.close || 'Close');
    esc.addEventListener('click', closeDialog);
    head.appendChild(input);
    head.appendChild(esc);
    list = el('div', 'dx-dialog-list');
    list.id = 'dx-hits';
    list.setAttribute('role', 'listbox');
    var foot = el('div', 'dx-dialog-foot');
    [['↑↓', T.navigate || 'navigate'], ['Enter', T.open || 'open'], ['Esc', T.close || 'close']].forEach(function (k) {
      var s = el('span');
      s.appendChild(el('kbd', 'dx-kbd', k[0]));
      s.appendChild(document.createTextNode(k[1]));
      foot.appendChild(s);
    });
    panel.appendChild(head);
    panel.appendChild(list);
    panel.appendChild(foot);
    dlg.appendChild(panel);
    document.body.appendChild(dlg);
    dlg.addEventListener('mousedown', function (e) { if (e.target === dlg) closeDialog(); });
    input.addEventListener('input', render);
    input.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); select(Math.min(sel + 1, hits.length - 1)); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); select(Math.max(sel - 1, 0)); }
      else if (e.key === 'Enter') { var a = hits[sel]; if (a) { e.preventDefault(); a.click(); } }
    });
    dlg.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.preventDefault(); closeDialog(); }
      if (e.key === 'Tab') { e.preventDefault(); input.focus(); }
    });
  }
  function select(i) {
    hits.forEach(function (h, j) { h.setAttribute('aria-selected', String(j === i)); });
    sel = i;
    if (hits[i]) { hits[i].scrollIntoView({ block: 'nearest' }); input.setAttribute('aria-activedescendant', hits[i].id); }
  }
  function message(text) {
    list.textContent = '';
    list.appendChild(el('div', 'dx-dialog-empty', text));
    hits = []; sel = -1;
  }
  function render() {
    var q = input.value;
    if (!index) { message(T.loading || '…'); return; }
    var ic = icons();
    var res;
    var group;
    if (!q.trim()) {
      // suggestions: the first doc of every product, then the rest in order
      var seen = {};
      res = index.filter(function (d) { if (seen[d.it.product]) return false; seen[d.it.product] = 1; return true; }).map(function (d) { return { d: d }; });
      group = T.suggested || 'Suggested';
    } else {
      res = search(q);
      group = T.results || 'Results';
      if (!res.length) { message((T.noResults || 'No results for') + ' “' + q.trim() + '”'); return; }
    }
    list.textContent = '';
    list.appendChild(el('div', 'dx-dialog-group', group));
    hits = res.map(function (r, i) {
      var it = r.d.it;
      var a = el('a', 'dx-hit');
      a.href = it.url + (r.head && r.head.id ? '#' + encodeURIComponent(r.head.id) : '');
      a.id = 'dx-hit-' + i;
      a.setAttribute('role', 'option');
      a.appendChild(el('span', 'dx-hit-icon', ic[it.product] || '📄'));
      var body = el('span', 'dx-hit-body');
      body.appendChild(highlight(el('span', 'dx-hit-title'), it.title, q));
      var sub = el('span', 'dx-hit-sub');
      if (r.head) highlight(sub, '# ' + r.head.text, q);
      else sub.textContent = [it.productName, it.section].filter(Boolean).join(' › ') + (it.excerpt ? ' — ' + it.excerpt : '');
      body.appendChild(sub);
      a.appendChild(body);
      var go = el('span', 'dx-hit-go');
      go.innerHTML = GO_SVG;
      a.appendChild(go);
      a.addEventListener('mousemove', function () { if (sel !== i) select(i); });
      list.appendChild(a);
      return a;
    });
    select(hits.length ? 0 : -1);
  }
  function openDialog(from) {
    if (!dlg) buildDialog();
    opener = from || document.activeElement;
    dlg.hidden = false;
    document.documentElement.style.overflow = 'hidden';
    input.value = '';
    input.focus();
    render();
    load().then(render, function () { message(T.error || 'Error'); });
  }
  function closeDialog() {
    if (!dlg || dlg.hidden) return;
    dlg.hidden = true;
    document.documentElement.style.overflow = root.classList.contains('is-open') ? 'hidden' : '';
    if (opener && opener.focus) opener.focus();
  }
  $$('[data-dx-search]').forEach(function (b) { b.addEventListener('click', function () { openDialog(b); }); });
  document.addEventListener('keydown', function (e) {
    var tag = (e.target && e.target.tagName) || '';
    var typing = /^(INPUT|TEXTAREA|SELECT)$/.test(tag) || (e.target && e.target.isContentEditable);
    if ((e.key === 'k' || e.key === 'K') && (e.ctrlKey || e.metaKey)) { e.preventDefault(); openDialog(); }
    else if (e.key === '/' && !typing) { e.preventDefault(); openDialog(); }
    else if (e.key === 'Escape' && root.classList.contains('is-open')) setDrawer(false);
  });
  // show ⌘K on Apple devices
  if (/Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)) $$('.dx-search-btn .dx-kbd').forEach(function (k) { k.textContent = '⌘K'; });

  // ------------------------------------------------------------ copy buttons on code blocks
  $$('.dx-content pre').forEach(function (pre) {
    var wrap = el('div', 'dx-code');
    pre.parentNode.insertBefore(wrap, pre);
    var code = pre.querySelector('code');
    var m = /(?:^|\s)(?:language|lang)-([\w+#.-]+)/.exec((code && code.className) || pre.className || '');
    var btn = el('button', 'dx-copy', T.copy || 'Copy');
    btn.type = 'button';
    btn.setAttribute('aria-label', (T.copy || 'Copy') + ' ' + (T.code || 'code'));
    if (m) {
      var bar = el('div', 'dx-code-bar');
      bar.appendChild(el('span', '', m[1]));
      bar.appendChild(btn);
      wrap.appendChild(bar);
    } else {
      wrap.appendChild(btn);
    }
    wrap.appendChild(pre);
    pre.setAttribute('tabindex', '0');
    btn.addEventListener('click', function () {
      var text = (code || pre).innerText.replace(/\n$/, '');
      var done = function () {
        btn.textContent = T.copied || 'Copied';
        btn.classList.add('is-done');
        setTimeout(function () { btn.textContent = T.copy || 'Copy'; btn.classList.remove('is-done'); }, 1600);
      };
      if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(done, fallback);
      else fallback();
      function fallback() {
        var ta = el('textarea');
        ta.value = text;
        ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        try { document.execCommand('copy'); done(); } catch (e) { /* ignore */ }
        document.body.removeChild(ta);
      }
    });
  });

  function store(key, value) {
    try {
      if (value === undefined) return JSON.parse(localStorage.getItem(key) || 'null');
      localStorage.setItem(key, JSON.stringify(value));
    } catch (e) { /* private mode */ }
    return null;
  }
  function copyText(text, done) {
    if (navigator.clipboard && window.isSecureContext) { navigator.clipboard.writeText(text).then(done, fallback); return; }
    fallback();
    function fallback() {
      var ta = el('textarea');
      ta.value = text;
      ta.style.position = 'fixed'; ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); done(); } catch (e) { /* ignore */ }
      document.body.removeChild(ta);
    }
  }

  // ------------------------------------------------------------ code tabs: <div class="dx-tabs"> around code blocks
  // (tab label = the block's title attribute, else its language); the picked label is remembered site-wide
  var TAB_KEY = 'dx-tab';
  var tabGroups = $$('.dx-content .dx-tabs').map(function (box) {
    var panes = $$('.dx-code', box).filter(function (c) { return c.parentNode === box || c.parentNode.parentNode === box; });
    if (panes.length < 2) return null;
    var bar = el('div', 'dx-tabs-bar');
    bar.setAttribute('role', 'tablist');
    var tabs = panes.map(function (pane, i) {
      var pre = $('pre', pane), code = pre && $('code', pre);
      var m = /(?:^|\s)(?:language|lang)-([\w+#.-]+)/.exec((code && code.className) || (pre && pre.className) || '');
      var label = (pre && pre.getAttribute('title')) || (m && m[1]) || String(i + 1);
      if (pre) pre.removeAttribute('title');
      var bar1 = $('.dx-code-bar span', pane);
      if (bar1) bar1.textContent = m ? m[1] : '';
      var b = el('button', 'dx-tab', label);
      b.type = 'button';
      b.setAttribute('role', 'tab');
      b.addEventListener('click', function () { pick(label); store(TAB_KEY, label); });
      bar.appendChild(b);
      return { b: b, pane: pane, label: label };
    });
    box.insertBefore(bar, box.firstChild);
    function pick(label) {
      var hit = tabs.some(function (t) { return t.label === label; }) ? label : tabs[0].label;
      tabs.forEach(function (t) {
        var on = t.label === hit;
        t.b.setAttribute('aria-selected', String(on));
        t.b.tabIndex = on ? 0 : -1;
        t.pane.hidden = !on;
      });
    }
    pick(store(TAB_KEY));
    return pick;
  }).filter(Boolean);
  if (tabGroups.length > 1) {
    root.addEventListener('click', function (e) {
      var b = e.target && e.target.closest && e.target.closest('.dx-tab');
      if (b) tabGroups.forEach(function (pick) { pick(b.textContent); });
    });
  }

  // ------------------------------------------------------------ collapsible sidebar groups (remembered per browser)
  var GROUPS_KEY = 'dx-closed';
  var closed = store(GROUPS_KEY) || [];
  if (!Array.isArray(closed)) closed = [];
  $$('.dx-nav-group[data-dx-group]').forEach(function (g) {
    var key = g.getAttribute('data-dx-group');
    if (closed.indexOf(key) >= 0 && !$('a.is-active', g)) g.open = false;
    g.addEventListener('toggle', function () {
      var now = (store(GROUPS_KEY) || []).filter(function (k) { return k !== key; });
      if (!g.open) now.push(key);
      store(GROUPS_KEY, now.slice(-200));
    });
  });

  // ------------------------------------------------------------ copy the article as Markdown
  $$('[data-dx-copy-md]').forEach(function (b) {
    var label = $('span', b);
    b.addEventListener('click', function () {
      fetch(b.getAttribute('data-dx-copy-md'), { headers: { Accept: 'text/markdown' } })
        .then(function (r) { if (!r.ok) throw new Error(r.status); return r.text(); })
        .then(function (md) {
          copyText(md, function () {
            if (label) label.textContent = T.copiedMd || 'Copied';
            b.classList.add('is-done');
            setTimeout(function () { if (label) label.textContent = T.copyMd || 'Copy as Markdown'; b.classList.remove('is-done'); }, 1800);
          });
        })
        .catch(function () { window.location.href = b.getAttribute('data-dx-copy-md'); });
    });
  });

  // ------------------------------------------------------------ TOC scroll-spy
  var links = $$('.dx-toc [data-dx-toc]');
  if (links.length && 'IntersectionObserver' in window) {
    var heads = links.map(function (a) { return document.getElementById(a.getAttribute('data-dx-toc')); }).filter(Boolean);
    var current = '';
    var setActive = function (id) {
      if (id === current) return;
      current = id;
      links.forEach(function (a) {
        var on = a.getAttribute('data-dx-toc') === id;
        a.classList.toggle('is-active', on);
        if (on) a.setAttribute('aria-current', 'location'); else a.removeAttribute('aria-current');
      });
    };
    var pick = function () {
      var top = (parseFloat(getComputedStyle(root).getPropertyValue('--dx-top')) || 0) + 96;
      var id = heads.length ? heads[0].id : '';
      for (var i = 0; i < heads.length; i++) { if (heads[i].getBoundingClientRect().top <= top) id = heads[i].id; else break; }
      if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 4 && heads.length) id = heads[heads.length - 1].id;
      setActive(id);
    };
    var io = new IntersectionObserver(pick, { rootMargin: '0px 0px -60% 0px', threshold: [0, 1] });
    heads.forEach(function (h) { io.observe(h); });
    window.addEventListener('scroll', function () { window.requestAnimationFrame(pick); }, { passive: true });
    pick();
  }
  // mobile TOC: close after picking a heading
  $$('.dx-toc-m a').forEach(function (a) { a.addEventListener('click', function () { var d = a.closest('details'); if (d) d.open = false; }); });
  // back to top
  $$('[data-dx-top]').forEach(function (a) { a.addEventListener('click', function (e) { e.preventDefault(); window.scrollTo({ top: 0, behavior: 'smooth' }); }); });
})();
