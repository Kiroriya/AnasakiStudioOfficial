/* Page Translate — the floating translate button.
 * Free mode: the Google Translate Website widget (googtrans cookie + reload).
 * API mode: Google Cloud Translation API v2 on the page's text nodes, cached in localStorage.
 * Settings come from data-* on .ptw-root; the API key (API mode only) from <meta name="ptw-api-key"> written
 * by code/main.ts, so a key saved while the site uses the free mode never reaches the page.
 */
(function () {
  "use strict";

  var root = document.querySelector(".ptw-root");
  if (!root) return;
  // never let Google Translate touch our own widget UI
  root.classList.add("notranslate");
  root.setAttribute("translate", "no");

  var MODE = (root.getAttribute("data-mode") || "free").trim();
  var keyMeta = document.querySelector('meta[name="ptw-api-key"]');
  var API_KEY = MODE === "api" && keyMeta ? (keyMeta.getAttribute("content") || "").trim() : "";
  var SITE_LANG = (root.getAttribute("data-site-lang") || "th").trim();

  var btn = root.querySelector(".ptw-btn");
  var menu = root.querySelector(".ptw-menu");
  var status = root.querySelector(".ptw-status");

  var BATCH = 80;              // strings per API request
  var CACHE_PREFIX = "ptw:v2:"; // v2: plain-text translations (v1 stored HTML-escaped text)

  var nodes = [];              // [{node, original}]
  var seen = new WeakSet();    // text nodes already collected
  var currentLang = null;
  var busy = false;

  function setStatus(msg, isError) {
    if (!msg) { status.hidden = true; return; }
    status.textContent = msg;
    status.dataset.state = isError ? "error" : "ok";
    status.hidden = false;
  }

  function openMenu(open) {
    menu.hidden = !open;
    btn.setAttribute("aria-expanded", String(open));
  }

  // ===================== FREE MODE (Google Translate widget) ==========
  var widgetReady = false;

  window.googleTranslateElementInit = function () {
    new google.translate.TranslateElement({
      pageLanguage: SITE_LANG,
      autoDisplay: false,
      layout: google.translate.TranslateElement.InlineLayout.SIMPLE
    }, "google_translate_element");
    widgetReady = true;
  };

  function loadWidget(cb) {
    if (widgetReady) return cb();
    var s = document.createElement("script");
    s.src = "https://translate.google.com/translate_a/element.js?cb=googleTranslateElementInit";
    s.onload = function () {
      var tries = 0;
      (function poll() {
        var combo = document.querySelector(".goog-te-combo");
        if (combo || tries++ > 40) cb();
        else setTimeout(poll, 150);
      })();
    };
    s.onerror = function () {
      setStatus("Cannot load Google Translate widget", true);
      setTimeout(function () { setStatus(null); }, 4000);
    };
    document.head.appendChild(s);
  }

  // every scope a googtrans cookie may live on (Google's widget may write one on the parent domain too):
  // host-only, then the host and each parent suffix — used to clear them all
  function googtransDomains() {
    var parts = location.hostname.split(".");
    var domains = [""];
    for (var i = 0; i < parts.length - 1; i++) {
      var d = parts.slice(i).join(".");
      domains.push(d, "." + d);
    }
    return domains;
  }

  function clearGoogtrans() {
    googtransDomains().forEach(function (d) {
      document.cookie = "googtrans=;path=/" + (d ? ";domain=" + d : "") + ";expires=Thu, 01 Jan 1970 00:00:00 GMT";
    });
  }

  function setWidgetLang(lang) {
    // set the googtrans cookie and reload — the widget translates on load.
    // 1.4.0: host-only, so the choice does not leak to the other sites under the same parent domain
    var target = lang === "__restore" ? SITE_LANG : lang;
    clearGoogtrans();
    if (target !== SITE_LANG) document.cookie = "googtrans=/" + SITE_LANG + "/" + target + ";path=/;SameSite=Lax";
    location.reload();
  }

  function currentWidgetLang() {
    var m = document.cookie.match(/googtrans=\/[^/]+\/([^;]+)/);
    return m ? decodeURIComponent(m[1]) : null;
  }

  // ===================== API MODE ======================================
  var SKIP_TAGS = { SCRIPT: 1, STYLE: 1, NOSCRIPT: 1, TEXTAREA: 1, CODE: 1, PRE: 1 };

  /** Text nodes under `start` not collected yet. */
  function collect(start) {
    var found = [];
    var walker = document.createTreeWalker(start, NodeFilter.SHOW_TEXT, {
      acceptNode: function (n) {
        var p = n.parentElement;
        if (!p || SKIP_TAGS[p.tagName] || p.closest(".ptw-root, .notranslate, [translate=no]")) return NodeFilter.FILTER_REJECT;
        if (!n.textContent.trim() || seen.has(n)) return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      }
    });
    var n;
    while ((n = walker.nextNode())) {
      seen.add(n);
      var item = { node: n, original: n.textContent };
      nodes.push(item);
      found.push(item);
    }
    return found;
  }

  function cacheKey(text, target) { return CACHE_PREFIX + target + ":" + text; }

  function translateBatch(texts, target) {
    var url = "https://translation.googleapis.com/language/translate/v2?key=" + encodeURIComponent(API_KEY);
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      // 1.4.0: "text" — the result goes into textContent; "html" came back entity-escaped (&amp; &#39;)
      body: JSON.stringify({ q: texts, target: target, format: "text", source: SITE_LANG || undefined })
    }).then(function (res) {
      if (!res.ok) return res.json().catch(function () { return {}; }).then(function (err) {
        throw new Error((err.error && err.error.message) || ("HTTP " + res.status));
      });
      return res.json();
    }).then(function (data) {
      return data.data.translations.map(function (t) { return t.translatedText; });
    });
  }

  function applyTranslations(items, target) {
    if (target === "__restore") {
      items.forEach(function (it) { it.node.textContent = it.original; });
      return Promise.resolve();
    }
    var need = [], cached = [];
    items.forEach(function (it, i) {
      var hit = null;
      try { hit = localStorage.getItem(cacheKey(it.original, target)); } catch (e) { /* storage off */ }
      if (hit !== null) cached[i] = hit; else need.push(i);
    });
    var chain = Promise.resolve();
    for (var i = 0; i < need.length; i += BATCH) {
      (function (chunk) {
        chain = chain.then(function () {
          return translateBatch(chunk.map(function (j) { return items[j].original; }), target).then(function (out) {
            chunk.forEach(function (idx, k) {
              cached[idx] = out[k];
              try { localStorage.setItem(cacheKey(items[idx].original, target), out[k]); } catch (e) { /* full or off */ }
            });
          });
        });
      })(need.slice(i, i + BATCH));
    }
    return chain.then(function () {
      items.forEach(function (it, i) { if (cached[i] !== undefined) it.node.textContent = cached[i]; });
    });
  }

  function markMenu() {
    menu.querySelectorAll(".ptw-opt").forEach(function (o) {
      if (o.dataset.lang === currentLang) o.setAttribute("aria-checked", "true");
      else o.removeAttribute("aria-checked");
    });
  }

  // 1.4.0: text added after the first translation (lists that load later, code blocks) is translated too
  var observer = null, pending = [], timer = 0;
  function watch() {
    if (observer || !window.MutationObserver) return;
    observer = new MutationObserver(function (list) {
      if (!currentLang || busy) return;
      list.forEach(function (m) {
        m.addedNodes.forEach(function (n) {
          if (n.nodeType === 1 || n.nodeType === 3) pending.push(n.nodeType === 3 ? n.parentNode : n);
        });
      });
      clearTimeout(timer);
      timer = setTimeout(function () {
        var roots = pending.splice(0), items = [];
        roots.forEach(function (r) { if (r && r.isConnected) items = items.concat(collect(r)); });
        if (items.length && currentLang) applyTranslations(items, currentLang).catch(function () { /* next try on the next change */ });
      }, 400);
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  function setLang(target) {
    openMenu(false);
    if (MODE !== "api") { setWidgetLang(target); return; }
    if (busy || !API_KEY) return;
    busy = true;
    setStatus(target === "__restore" ? "Restoring…" : "Translating…");
    if (!nodes.length) collect(document.body);
    applyTranslations(nodes, target)
      .then(function () {
        currentLang = target === "__restore" ? null : target;
        markMenu();
        if (currentLang) watch();
        setStatus(null);
      })
      .catch(function (err) {
        setStatus("Translate error: " + err.message, true);
        setTimeout(function () { setStatus(null); }, 6000);
      })
      .then(function () { busy = false; });
  }

  // ===================== UI ============================================
  btn.addEventListener("click", function () {
    if (MODE === "api" && !API_KEY) {
      setStatus("No API key — set it in the Page Translate settings", true);
      setTimeout(function () { setStatus(null); }, 4000);
      return;
    }
    openMenu(menu.hidden);
  });
  menu.addEventListener("click", function (e) {
    var opt = e.target.closest("[data-lang]");
    if (opt) setLang(opt.dataset.lang);
  });
  document.addEventListener("click", function (e) {
    if (!menu.hidden && !root.contains(e.target)) openMenu(false);
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && !menu.hidden) { openMenu(false); btn.focus(); }
  });

  // free mode after the reload: load the widget so Google translates, and mark the chosen language
  if (MODE !== "api") {
    var active = currentWidgetLang();
    if (active && active !== SITE_LANG) {
      loadWidget(function () {});
      currentLang = active;
      markMenu();
    }
  }
})();
