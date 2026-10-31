(function () {
  var bar = document.querySelector('.anab');
  if (!bar) return;
  document.body.insertBefore(bar, document.body.firstChild); // show at the very top
  // boolean settings are rendered as "true" / "false"; when the bar is not
  // dismissible an earlier dismissal is ignored, so it always shows
  if (bar.getAttribute('data-dismissible') !== 'true') return;
  var key = 'anab:' + (bar.getAttribute('data-key') || '');
  try { if (localStorage.getItem(key) === '1') bar.classList.add('anab-hidden'); } catch (e) {}
  var close = bar.querySelector('.anab-close');
  if (close) close.addEventListener('click', function () {
    bar.classList.add('anab-hidden');
    try { localStorage.setItem(key, '1'); } catch (e) {}
  });
})();
