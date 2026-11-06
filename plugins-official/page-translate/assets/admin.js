/* Page Translate admin page (site.adminPage "settings") — submits the forms through
   anasaki.call('page-translate:…'). Drawn on the server; this only wires the buttons. */
(function () {
  'use strict';
  var M = {
    th: { saved: 'บันทึกแล้ว', keySaved: 'บันทึก API key แล้ว', keyCleared: 'ลบ API key แล้ว', confirmClear: 'ลบ API key? โหมด API จะใช้งานไม่ได้จนกว่าจะใส่ใหม่', empty: 'กรุณาใส่ API key', ok: 'ลบ', cancel: 'ยกเลิก' },
    en: { saved: 'Saved', keySaved: 'API key saved', keyCleared: 'API key cleared', confirmClear: 'Clear the API key? API mode stops working until a new one is set', empty: 'Enter an API key', ok: 'Clear', cancel: 'Cancel' },
  };
  var data = {};
  try { data = JSON.parse(document.getElementById('ptw-data').textContent || '{}'); } catch (e) { /* empty */ }
  var t = M[data.locale === 'en' ? 'en' : 'th'];
  var toast = function (m, k) { if (window.AK && AK.toast) AK.toast(m, k); else window.alert(m); };

  function call(name, input) {
    return anasaki.call('page-translate:' + name, input).then(function (d) {
      if (d && d.errors && Object.keys(d.errors).length) throw new Error(d.message || d.errors.form || 'Error');
      return d ? d.data : null;
    });
  }
  function busy(form, on) {
    Array.prototype.forEach.call(form.querySelectorAll('button'), function (b) { b.disabled = on; });
  }

  var general = document.getElementById('ptw-general');
  if (general) {
    general.addEventListener('submit', function (e) {
      e.preventDefault();
      busy(general, true);
      call('save', {
        mode: general.elements.mode.value,
        siteLang: general.elements.siteLang.value,
        buttonColor: general.elements.buttonColor.value,
        position: general.elements.position ? general.elements.position.value : '',
        buttonStyle: general.elements.buttonStyle ? general.elements.buttonStyle.value : '',
      }).then(function () { toast(t.saved, 'success'); })
        .catch(function (err) { toast(err.message, 'danger'); })
        .then(function () { busy(general, false); });
    });
  }

  var keyForm = document.getElementById('ptw-key-form');
  if (keyForm) {
    var reload = function () { setTimeout(function () { location.reload(); }, 600); };
    keyForm.addEventListener('submit', function (e) {
      e.preventDefault();
      var v = keyForm.elements.apiKey.value.trim();
      if (!v) { toast(t.empty, 'warning'); return; }
      busy(keyForm, true);
      call('saveApiKey', { apiKey: v })
        .then(function () { keyForm.elements.apiKey.value = ''; toast(t.keySaved, 'success'); reload(); })
        .catch(function (err) { toast(err.message, 'danger'); })
        .then(function () { busy(keyForm, false); });
    });
    var clear = keyForm.querySelector('[data-ptw-clear]');
    if (clear) {
      clear.addEventListener('click', function () {
        var ask = window.AK && AK.confirm ? AK.confirm(t.confirmClear, { ok: t.ok, cancel: t.cancel, danger: true }) : Promise.resolve(window.confirm(t.confirmClear));
        Promise.resolve(ask).then(function (yes) {
          if (!yes) return;
          busy(keyForm, true);
          return call('saveApiKey', { apiKey: '' })
            .then(function () { toast(t.keyCleared, 'success'); reload(); })
            .catch(function (err) { toast(err.message, 'danger'); })
            .then(function () { busy(keyForm, false); });
        });
      });
    }
  }
})();
