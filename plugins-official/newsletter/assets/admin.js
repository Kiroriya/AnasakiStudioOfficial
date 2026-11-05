/* Newsletter admin page: forms and buttons through anasaki.call('newsletter:…'), the CSV export through
   anasaki.download. The page is drawn on the server and reloaded after a change. */
(function () {
  'use strict';
  // in admin pages anasaki.call resolves to the action's answer: { data } or { message, errors }
  function done(r) { return !!r && r.ok !== false && !(r.errors && Object.keys(r.errors).length); }
  function toast(m, kind) { if (window.AK && AK.toast) AK.toast(m, kind); }
  function ask(text, danger) { return window.AK && AK.confirm ? AK.confirm(text, { danger: !!danger }) : Promise.resolve(window.confirm(text)); }
  function call(name, input) {
    return anasaki.call('newsletter:' + name, input).then(function (r) {
      if (!done(r)) { toast((r && r.message) || 'Error', 'danger'); throw new Error('failed'); }
      if (r.message) toast(r.message, 'success');
      return r;
    });
  }
  function formData(f) {
    var o = {};
    new FormData(f).forEach(function (v, k) { o[k] = v; });
    return o;
  }
  function campaignForm() { return document.querySelector('form[data-nl-form="saveCampaign"]'); }
  /** Saves the open campaign (new or draft) and answers its id. */
  function save() {
    var f = campaignForm();
    var input = formData(f);
    if (f.getAttribute('data-id')) input.id = Number(f.getAttribute('data-id'));
    return call('saveCampaign', input).then(function (r) {
      var id = r.data && r.data.id;
      if (id && !f.getAttribute('data-id')) f.setAttribute('data-id', String(id));
      return id;
    });
  }

  document.addEventListener('submit', function (e) {
    var f = e.target;
    var kind = f.getAttribute && f.getAttribute('data-nl-form');
    if (!kind) return;
    e.preventDefault();
    if (kind === 'saveCampaign') {
      save().then(function (id) { location.search = '?tab=camps&edit=' + id; }).catch(function () {});
      return;
    }
    call(kind, formData(f)).then(function () { location.reload(); }).catch(function () {});
  });

  document.addEventListener('click', function (e) {
    var b = e.target.closest && e.target.closest('[data-nl]');
    if (b) {
      e.preventDefault();
      var what = b.getAttribute('data-nl');
      if (what === 'removeSubscriber') {
        ask('?', true).then(function (yes) { if (yes) call('removeSubscriber', { id: Number(b.getAttribute('data-id')) }).then(function () { location.reload(); }).catch(function () {}); });
      } else if (what === 'sendTest') {
        save().then(function (id) { return call('sendTest', { id: id }); }).catch(function () {});
      } else if (what === 'send') {
        var subject = campaignForm().querySelector('[name=subject]').value;
        ask((b.getAttribute('data-ask') || '').replace('{s}', subject), false).then(function (yes) {
          if (!yes) return;
          save().then(function (id) { return call('send', { id: id }).then(function () { location.search = '?tab=camps'; }); }).catch(function () {});
        });
      }
      return;
    }
    var d = e.target.closest && e.target.closest('[data-nl-download]');
    if (d) {
      e.preventDefault();
      var s = d.getAttribute('data-nl-download');
      anasaki.download(s ? { status: s } : {});
    }
  });
})();
