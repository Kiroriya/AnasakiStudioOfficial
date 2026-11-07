/* Shop on public pages: add to the cart without leaving the page, the cart count, and the payment slip
   (the photo is shrunk to at most 1400 px JPEG before it is sent). Everything also works without it. */
(function () {
  'use strict';
  if (window.__anasakiShop) return;
  window.__anasakiShop = true;

  function setCount(n) {
    document.querySelectorAll('[data-shop-count]').forEach(function (el) {
      el.textContent = String(n);
      el.hidden = !(n > 0);
    });
  }
  function post(url, body) {
    return fetch(url, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(body) })
      .then(function (r) { return r.json().then(function (j) { j.status = r.status; return j; }); });
  }
  function note(form, text, link) {
    var p = form.querySelector('.shop-note');
    if (!p) { p = document.createElement('p'); p.className = 'shop-note'; p.setAttribute('role', 'status'); form.appendChild(p); }
    p.textContent = text + ' ';
    if (link) { var a = document.createElement('a'); a.href = '/cart'; a.textContent = link; p.appendChild(a); }
  }

  if (document.querySelector('[data-shop-count]')) {
    fetch('/cart/count', { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (j) { setCount(j.count || 0); }).catch(function () {});
  }

  document.addEventListener('submit', function (e) {
    var f = e.target;
    if (f.matches && f.matches('form[data-shop-add]')) {
      e.preventDefault();
      var btn = f.querySelector('button');
      if (btn) btn.disabled = true;
      var qty = f.querySelector('[name=qty]');
      var opt = f.querySelector('select[name=v]');
      post('/cart/add', { id: f.querySelector('[name=id]').value, qty: qty ? qty.value : 1, v: opt ? opt.value : '' }).then(function (j) {
        if (btn) btn.disabled = false;
        if (j.status >= 400) { note(f, j.error || 'Error'); return; }
        setCount(j.count);
        note(f, f.getAttribute('data-added') || 'Added', f.getAttribute('data-view'));
      }).catch(function () { if (btn) btn.disabled = false; f.submit(); });
      return;
    }
    if (f.matches && f.matches('form[data-shop-slip]')) {
      e.preventDefault();
      var input = f.querySelector('input[type=file]');
      var msg = f.querySelector('[role=status]');
      var file = input && input.files && input.files[0];
      if (!file || !/^image\/(jpeg|png)$/.test(file.type)) { msg.textContent = f.getAttribute('data-bad'); return; }
      var b = f.querySelector('button');
      b.disabled = true;
      shrink(file).then(function (dataUrl) {
        return post('/shop/order/' + f.getAttribute('data-shop-slip') + '/slip', { image: dataUrl });
      }).then(function (j) {
        if (j.status >= 400) { b.disabled = false; msg.textContent = j.error || f.getAttribute('data-bad'); return; }
        f.outerHTML = '<p class="shop-ok" id="shop-slip-done">' + (f.getAttribute('data-done') || 'OK').replace(/[<>&]/g, '') + '</p>';
      }).catch(function () { b.disabled = false; msg.textContent = f.getAttribute('data-bad'); });
    }
  });

  // a product's options: the price shown follows the chosen one
  document.addEventListener('change', function (e) {
    var s = e.target;
    if (!s.matches || !s.matches('form[data-shop-add] select[name=v]')) return;
    var o = s.options[s.selectedIndex];
    var box = s.closest('.shop-buy');
    var b = box && box.querySelector('.shop-price b');
    if (b && o && o.getAttribute('data-price')) {
      b.textContent = o.getAttribute('data-price');
      var from = box.querySelector('.shop-from');
      if (from) from.hidden = true;
    }
  });

  // more photos: open large over the page (Esc, a click or the arrows move / close)
  document.addEventListener('click', function (e) {
    var a = e.target.closest && e.target.closest('a[data-shop-zoom]');
    if (!a) return;
    e.preventDefault();
    var all = Array.prototype.slice.call(a.closest('[data-shop-gallery]').querySelectorAll('a[data-shop-zoom]'));
    var i = all.indexOf(a);
    var box = document.createElement('div');
    box.setAttribute('role', 'dialog');
    box.setAttribute('aria-modal', 'true');
    box.style.cssText = 'position:fixed;inset:0;z-index:2147483000;background:rgba(0,0,0,.88);display:flex;align-items:center;justify-content:center;cursor:zoom-out';
    var img = document.createElement('img');
    img.style.cssText = 'max-width:94vw;max-height:90vh;object-fit:contain;border-radius:6px';
    function show() { img.src = all[i].getAttribute('href'); img.alt = (all[i].querySelector('img') || {}).alt || ''; }
    function close() { box.remove(); document.removeEventListener('keydown', key); a.focus(); }
    function key(k) {
      if (k.key === 'Escape') close();
      else if (k.key === 'ArrowRight') { i = (i + 1) % all.length; show(); }
      else if (k.key === 'ArrowLeft') { i = (i - 1 + all.length) % all.length; show(); }
    }
    show();
    box.appendChild(img);
    box.addEventListener('click', close);
    document.addEventListener('keydown', key);
    document.body.appendChild(box);
    box.tabIndex = -1;
    box.focus();
  });

  function shrink(file) {
    return new Promise(function (resolve, reject) {
      var img = new Image();
      var url = URL.createObjectURL(file);
      img.onload = function () {
        var max = 1400, w = img.naturalWidth, h = img.naturalHeight;
        if (w > max || h > max) { var k = Math.min(max / w, max / h); w = Math.round(w * k); h = Math.round(h * k); }
        var c = document.createElement('canvas');
        c.width = w; c.height = h;
        var g = c.getContext('2d');
        g.fillStyle = '#fff'; g.fillRect(0, 0, w, h);
        g.drawImage(img, 0, 0, w, h);
        URL.revokeObjectURL(url);
        resolve(c.toDataURL('image/jpeg', 0.82));
      };
      img.onerror = function () { URL.revokeObjectURL(url); reject(new Error('image')); };
      img.src = url;
    });
  }
})();
