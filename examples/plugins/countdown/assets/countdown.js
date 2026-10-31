(function () {
  // Starts every [data-countdown] element: on page load and whenever one is
  // added later (designer preview, client-side navigation) or its target changes.
  function pad(n) { return n < 10 ? '0' + n : '' + n; }
  function start(el) {
    var target = Date.parse(el.getAttribute('data-countdown'));
    if (el._cdTarget === target) return; // already running for this target
    if (el._cdTimer) clearInterval(el._cdTimer);
    el._cdTimer = 0;
    el._cdTarget = target;
    if (isNaN(target)) return;
    var cells = {};
    el.querySelectorAll('[data-cd]').forEach(function (c) { cells[c.getAttribute('data-cd')] = c; });
    var done = el.querySelector('.cd-done');
    el.classList.remove('cd-finished');
    if (done) done.hidden = true;
    function tick() {
      if (!el.isConnected) return false; // removed from the page
      var left = Math.max(0, target - Date.now());
      var s = Math.floor(left / 1000);
      if (cells.days) cells.days.textContent = Math.floor(s / 86400);
      if (cells.hours) cells.hours.textContent = pad(Math.floor(s / 3600) % 24);
      if (cells.minutes) cells.minutes.textContent = pad(Math.floor(s / 60) % 60);
      if (cells.seconds) cells.seconds.textContent = pad(s % 60);
      if (left > 0) return true;
      el.classList.add('cd-finished');
      if (done) { done.textContent = el.getAttribute('data-done') || ''; done.hidden = false; }
      return false;
    }
    if (tick()) {
      el._cdTimer = setInterval(function () {
        if (!tick()) { clearInterval(el._cdTimer); el._cdTimer = 0; }
      }, 1000);
    }
  }
  function scan(root) {
    if (root.nodeType !== 1) return;
    if (root.hasAttribute('data-countdown')) start(root);
    root.querySelectorAll('[data-countdown]').forEach(start);
  }
  function init() {
    scan(document.documentElement);
    if (!window.MutationObserver) return;
    new MutationObserver(function (records) {
      records.forEach(function (r) {
        if (r.type === 'attributes') start(r.target);
        else r.addedNodes.forEach(scan);
      });
    }).observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ['data-countdown'] });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init); else init();
})();
