/* Donations: the slip upload of a gift (the photo is shrunk to at most 1400 px JPEG before it is sent). */
(function () {
  'use strict';
  if (window.__anasakiDonate) return;
  window.__anasakiDonate = true;

  document.addEventListener('submit', function (e) {
    var f = e.target;
    if (!f.matches || !f.matches('form[data-dn-slip]')) return;
    e.preventDefault();
    var input = f.querySelector('input[type=file]');
    var msg = f.querySelector('[role=status]');
    var file = input && input.files && input.files[0];
    if (!file || !/^image\/(jpeg|png)$/.test(file.type)) { msg.textContent = f.getAttribute('data-bad'); return; }
    var b = f.querySelector('button');
    b.disabled = true;
    shrink(file).then(function (dataUrl) {
      return fetch('/donate/d/' + f.getAttribute('data-dn-slip') + '/slip', {
        method: 'POST', credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ image: dataUrl }),
      }).then(function (r) { return r.json().then(function (j) { j.status = r.status; return j; }); });
    }).then(function (j) {
      if (j.status >= 400) { b.disabled = false; msg.textContent = j.error || f.getAttribute('data-bad'); return; }
      var p = document.createElement('p');
      p.className = 'dn-msg';
      p.id = 'dn-slip-done';
      p.textContent = f.getAttribute('data-done') || 'OK';
      f.replaceWith(p);
    }).catch(function () { b.disabled = false; msg.textContent = f.getAttribute('data-bad'); });
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
