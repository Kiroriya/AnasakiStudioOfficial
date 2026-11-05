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
      post('/cart/add', { id: f.querySelector('[name=id]').value, qty: qty ? qty.value : 1 }).then(function (j) {
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
