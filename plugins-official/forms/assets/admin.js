/* Forms admin page (site.adminPage "forms") — builder, submissions inbox, CSV export.
   Drawn with the admin UI kit (window.AK); data through anasaki.call('forms:…'). All user text goes in
   with textContent / value, never innerHTML. */
(function () {
  'use strict';
  var M = {
    th: {
      title: 'แบบฟอร์ม', desc: 'สร้างฟอร์มติดต่อ สมัครสมาชิก หรือรับสมัครงาน — ข้อความที่ส่งมาจะเก็บไว้ที่นี่',
      howto: 'วางฟอร์มบนหน้าเว็บ: เปิดหน้าออกแบบ → แผงเพิ่ม → บล็อกปลั๊กอิน → “ฟอร์ม” แล้วใส่หมายเลขฟอร์มในช่อง “ฟอร์ม (หมายเลข)”',
      'new': 'สร้างฟอร์ม', newName: 'ฟอร์มติดต่อ', count: '{n} รายการ', empty: 'ยังไม่มีฟอร์ม', build: 'ออกแบบฟอร์ม',
      subs: 'ข้อความที่ส่งมา', name: 'ชื่อฟอร์ม', submitLabel: 'ข้อความบนปุ่มส่ง', send: 'ส่งข้อความ',
      thanks: 'ขอบคุณ! เราได้รับข้อความของคุณแล้ว', message: 'ข้อความ', fields: 'ช่องกรอก', label: 'ชื่อช่อง', required: 'จำเป็น',
      optionsPh: 'ตัวเลือก คั่นด้วย ,', after: 'หลังกดส่ง', success: 'ข้อความขอบคุณ', redirect: 'พาไปหน้า',
      deleteText: 'ข้อความที่ส่งมาทั้งหมดของฟอร์มนี้จะถูกลบด้วย', deleteSub: 'ลบข้อความนี้?', noSubs: 'ยังไม่มีใครส่งข้อความ',
      markRead: 'ทำเครื่องหมายว่าอ่านแล้ว', markUnread: 'ทำเครื่องหมายว่ายังไม่อ่าน', save: 'บันทึก', saved: 'บันทึกแล้ว ✓', del: 'ลบ',
      created: 'สร้างเมื่อ', optional: '(ไม่ใส่ก็ได้)', cname: 'ชื่อ', cemail: 'อีเมล', blockId: 'หมายเลขสำหรับบล็อก',
      webhookHint: 'ส่ง POST JSON {event, form, submissionId, data, submittedAt} ไปยัง URL นี้ทุกครั้งที่ฟอร์มนี้มีข้อความใหม่',
      deleteForm: 'ลบฟอร์ม “{n}”?',
      type: { text: 'ข้อความสั้น', email: 'อีเมล', tel: 'เบอร์โทร', textarea: 'ข้อความยาว', select: 'ตัวเลือก', checkbox: 'ช่องติ๊ก', number: 'ตัวเลข', date: 'วันที่' },
    },
    en: {
      title: 'Forms', desc: 'Build contact, sign-up or job forms — submissions are stored here.',
      howto: 'To place a form: open the designer → Insert → Plugin blocks → “Form”, then enter the form number in “Form (number)”.',
      'new': 'New form', newName: 'Contact form', count: '{n} submissions', empty: 'No forms yet', build: 'Builder',
      subs: 'Submissions', name: 'Form name', submitLabel: 'Submit button text', send: 'Send',
      thanks: 'Thanks! We received your message.', message: 'Message', fields: 'Fields', label: 'Field label', required: 'Required',
      optionsPh: 'Options, comma separated', after: 'After submitting', success: 'Success message', redirect: 'Redirect to',
      deleteText: 'All its submissions are deleted too.', deleteSub: 'Delete this submission?', noSubs: 'No submissions yet',
      markRead: 'Mark as read', markUnread: 'Mark as unread', save: 'Save', saved: 'Saved ✓', del: 'Delete',
      created: 'Created', optional: '(optional)', cname: 'Name', cemail: 'Email', blockId: 'Number for the block',
      webhookHint: 'POSTs JSON {event, form, submissionId, data, submittedAt} to this URL on every submission of this form.',
      deleteForm: 'Delete the form “{n}”?',
      type: { text: 'Short text', email: 'Email', tel: 'Phone', textarea: 'Long text', select: 'Dropdown', checkbox: 'Checkbox', number: 'Number', date: 'Date' },
    },
  };
  var TYPES = ['text', 'email', 'tel', 'textarea', 'select', 'checkbox', 'number', 'date'];
  var ICON = { text: '🔤', email: '✉️', tel: '📞', textarea: '📝', select: '▾', checkbox: '☑️', number: '#', date: '📅' };
  var PER = 25;

  var boot = JSON.parse(document.getElementById('forms-data').textContent || '{}');
  var L = M[boot.locale === 'en' ? 'en' : 'th'];
  var loc = boot.locale === 'en' ? 'en-US' : 'th-TH';
  var st = { forms: boot.forms || [], sel: null, draft: null, view: 'build', subs: [], page: 1, busy: false };
  var root = document.getElementById('forms-app');

  function t(k, n) { return String(L[k] == null ? k : L[k]).replace('{n}', n == null ? '' : n); }
  function el(tag, attrs, kids) {
    var e = document.createElement(tag);
    for (var k in attrs || {}) {
      if (k === 'text') e.textContent = attrs[k];
      else if (k === 'class') e.className = attrs[k];
      else if (k.slice(0, 2) === 'on') e.addEventListener(k.slice(2), attrs[k]);
      else if (attrs[k] === true) e.setAttribute(k, '');
      else if (attrs[k] !== false && attrs[k] != null) e.setAttribute(k, attrs[k]);
    }
    (kids || []).forEach(function (c) { if (c != null) e.append(c); });
    return e;
  }
  function toast(m, kind) { if (window.AK) AK.toast(m, kind); else alert(m); }
  function ask(text, danger) { return window.AK ? AK.confirm(text, { danger: !!danger }) : Promise.resolve(confirm(text)); }
  function clone(x) { return JSON.parse(JSON.stringify(x)); }
  function call(name, input) {
    return anasaki.call('forms:' + name, input).then(function (d) {
      if (d && d.errors && Object.keys(d.errors).length) throw new Error(d.message || d.errors.form || 'Error');
      return d ? d.data : null;
    });
  }
  function fail(e) { toast(e && e.message ? e.message : String(e), 'error'); }

  function load() { return call('list', {}).then(function (f) { st.forms = f || []; }, fail); }

  function select(f) { st.sel = f; st.draft = clone(f); st.view = 'build'; draw(); }

  function newForm() {
    st.sel = null; st.view = 'build';
    st.draft = { name: t('newName'), submitLabel: t('send'), successMessage: t('thanks'), redirectUrl: '', webhookUrl: '', fields: [
      { key: 'name', label: t('cname'), type: 'text', required: true, options: [], placeholder: '' },
      { key: 'email', label: t('cemail'), type: 'email', required: true, options: [], placeholder: '' },
      { key: 'message', label: t('message'), type: 'textarea', required: false, options: [], placeholder: '' },
    ] };
    draw();
  }

  function addField(type) {
    var fs = st.draft.fields, n = fs.length + 1;
    while (fs.some(function (f) { return f.key === 'field_' + n; })) n++;
    fs.push({ key: 'field_' + n, label: L.type[type], type: type, required: false, options: type === 'select' ? ['A', 'B'] : [], placeholder: '' });
    draw();
  }

  function save() {
    if (st.busy) return;
    st.busy = true;
    call('save', { form: st.draft }).then(function (f) {
      return load().then(function () {
        select(st.forms.filter(function (x) { return x.id === f.id; })[0] || f);
        toast(t('saved'));
      });
    }, fail).then(function () { st.busy = false; });
  }

  function remove() {
    var d = st.draft;
    if (!d || !d.id) return;
    ask(t('deleteForm', d.name) + ' ' + t('deleteText'), true).then(function (ok) {
      if (!ok) return;
      call('delete', { id: d.id }).then(function () { st.draft = null; st.sel = null; return load(); }, fail).then(draw);
    });
  }

  function loadSubs() {
    if (!st.sel) return;
    st.view = 'subs'; st.page = 1;
    call('submissions', { formId: st.sel.id }).then(function (s) { st.subs = s || []; draw(); }, fail);
  }

  function toggleRead(s) {
    call('markRead', { id: s.id, read: !s.read }).then(function (u) {
      s.read = u ? u.read : !s.read;
      return load();
    }, fail).then(draw);
  }

  function removeSub(s) {
    ask(t('deleteSub'), true).then(function (ok) {
      if (!ok) return;
      call('deleteSubmission', { id: s.id }).then(function () {
        st.subs = st.subs.filter(function (x) { return x.id !== s.id; });
        return load();
      }, fail).then(draw);
    });
  }

  // ------------------------------------------------------------ drawing

  function nav() {
    var box = el('nav', { class: 'ak-card forms-nav', id: 'forms-list' });
    if (!st.forms.length) box.append(el('p', { class: 'ak-muted ak-small', text: t('empty') }));
    st.forms.forEach(function (f) {
      var b = el('button', { type: 'button', class: 'forms-item' + (st.sel && st.sel.id === f.id ? ' on' : ''), 'data-id': f.id, onclick: function () { select(f); } }, [
        el('span', { class: 'forms-item-name' }, [document.createTextNode(f.name + ' '), f.unreadCount ? el('span', { class: 'ak-badge ak-badge-info', text: String(f.unreadCount) }) : null]),
        el('span', { class: 'ak-muted ak-small', text: '#' + f.id + ' · ' + t('count', f.submissionCount) }),
      ]);
      box.append(b);
    });
    return box;
  }

  function input(label, value, onInput, attrs) {
    var i = el('input', Object.assign({ value: value == null ? '' : value }, attrs || {}));
    i.value = value == null ? '' : value;
    i.addEventListener('input', function () { onInput(i.value); });
    return el('label', { class: 'ak-field' }, [el('span', { text: label }), i]);
  }

  function fieldRow(f, i, last) {
    var d = st.draft;
    var row = el('li', { class: 'forms-field', 'data-key': f.key });
    var lab = el('input', { class: 'forms-field-label', placeholder: t('label') });
    lab.value = f.label;
    lab.addEventListener('input', function () { f.label = lab.value; });
    var sel = el('select', { onchange: function () { f.type = sel.value; if (f.type === 'select' && !f.options.length) f.options = ['A', 'B']; draw(); } });
    TYPES.forEach(function (ty) { var o = el('option', { value: ty, text: L.type[ty] }); if (ty === f.type) o.selected = true; sel.append(o); });
    var req = el('input', { type: 'checkbox' });
    req.checked = !!f.required;
    req.addEventListener('change', function () { f.required = req.checked; });
    function move(dir) { var fs = d.fields, x = fs[i]; fs[i] = fs[i + dir]; fs[i + dir] = x; draw(); }
    row.append(
      el('span', { class: 'forms-type', title: f.type, text: ICON[f.type] || '•' }), lab, sel,
      el('label', { class: 'ak-check' }, [req, document.createTextNode(' ' + t('required'))]),
      el('button', { type: 'button', class: 'ak-btn ak-btn-sm', disabled: i === 0, onclick: function () { move(-1); }, text: '↑' }),
      el('button', { type: 'button', class: 'ak-btn ak-btn-sm', disabled: last, onclick: function () { move(1); }, text: '↓' }),
      el('button', { type: 'button', class: 'ak-btn ak-btn-sm ak-btn-danger', onclick: function () { d.fields.splice(i, 1); draw(); }, text: '✕' })
    );
    if (f.type === 'select') {
      var opts = el('input', { class: 'forms-options', placeholder: t('optionsPh') });
      opts.value = f.options.join(', ');
      opts.addEventListener('input', function () { f.options = opts.value.split(',').map(function (x) { return x.trim(); }).filter(Boolean); });
      row.append(opts);
    }
    return row;
  }

  function builder() {
    var d = st.draft;
    var box = el('div', { id: 'forms-builder' });
    if (d.id) box.append(el('p', { class: 'ak-small' }, [el('span', { class: 'ak-muted', text: t('blockId') + ': ' }), el('b', { class: 'ak-mono', id: 'forms-block-id', text: String(d.id) })]));
    box.append(el('div', { class: 'forms-2' }, [
      input(t('name'), d.name, function (v) { d.name = v; }, { id: 'forms-name' }),
      input(t('submitLabel'), d.submitLabel, function (v) { d.submitLabel = v; }),
    ]));
    box.append(el('h3', { text: t('fields') }));
    var ul = el('ul', { class: 'forms-fields' });
    d.fields.forEach(function (f, i) { ul.append(fieldRow(f, i, i === d.fields.length - 1)); });
    box.append(ul);
    var pick = el('div', { class: 'ak-row forms-pick' });
    TYPES.forEach(function (ty) { pick.append(el('button', { type: 'button', class: 'ak-btn ak-btn-sm', 'data-add': ty, onclick: function () { addField(ty); }, text: '+ ' + ICON[ty] + ' ' + L.type[ty] })); });
    box.append(pick);
    box.append(el('h3', { text: t('after') }));
    box.append(input(t('success'), d.successMessage, function (v) { d.successMessage = v; }));
    box.append(el('div', { class: 'forms-2' }, [
      input(t('redirect') + ' ' + t('optional'), d.redirectUrl, function (v) { d.redirectUrl = v; }, { placeholder: '/thank-you' }),
      el('div', {}, [input('Webhook ' + t('optional'), d.webhookUrl, function (v) { d.webhookUrl = v; }, { placeholder: 'https://hooks…' }),
        el('p', { class: 'ak-hint', text: t('webhookHint') })]),
    ]));
    box.append(el('div', { class: 'ak-form-actions' }, [
      d.id ? el('button', { type: 'button', class: 'ak-btn ak-btn-danger', id: 'forms-delete', onclick: remove, text: '🗑 ' + t('del') }) : null,
      el('span', { class: 'ak-grow' }),
      el('button', { type: 'button', class: 'ak-btn ak-btn-primary', id: 'forms-save', onclick: save, text: '💾 ' + t('save') }),
    ]));
    return box;
  }

  function inbox() {
    var cols = (st.sel ? st.sel.fields : []).map(function (f) { return { key: f.key, label: f.label }; });
    var box = el('div', { id: 'forms-subs' });
    box.append(el('div', { class: 'ak-row' }, [
      el('span', { class: 'ak-muted ak-small', text: t('count', st.subs.length) }), el('span', { class: 'ak-grow' }),
      el('button', { type: 'button', class: 'ak-btn ak-btn-sm', id: 'forms-csv', onclick: function () { anasaki.download({ form: st.sel.id }); }, text: '⬇ CSV' }),
    ]));
    var head = el('tr', {}, [el('th')].concat(cols.map(function (c) { return el('th', { text: c.label }); }), [el('th', { text: t('created') }), el('th')]));
    var body = el('tbody');
    var shown = st.subs.slice((st.page - 1) * PER, st.page * PER);
    if (!shown.length) body.append(el('tr', {}, [el('td', { colspan: cols.length + 3, class: 'ak-empty', text: t('noSubs') })]));
    shown.forEach(function (s) {
      body.append(el('tr', { class: s.read ? '' : 'forms-unread', 'data-id': s.id }, [
        el('td', {}, [el('button', { type: 'button', class: 'ak-btn ak-btn-sm ak-btn-ghost', title: s.read ? t('markUnread') : t('markRead'), onclick: function () { toggleRead(s); }, text: s.read ? '○' : '●' })]),
      ].concat(cols.map(function (c) { return el('td', { text: s.data[c.key] == null ? '' : s.data[c.key] }); }), [
        el('td', { class: 'ak-muted ak-small', text: new Date(s.createdAt).toLocaleString(loc) }),
        el('td', {}, [el('button', { type: 'button', class: 'ak-btn ak-btn-sm ak-btn-danger', onclick: function () { removeSub(s); }, text: '🗑' })]),
      ])));
    });
    box.append(el('div', { class: 'ak-table-wrap' }, [el('table', { class: 'ak-table' }, [el('thead', {}, [head]), body])]));
    var pg = el('div', { id: 'forms-pager' });
    box.append(pg);
    var pages = Math.max(1, Math.ceil(st.subs.length / PER));
    if (pages > 1 && window.AK) AK.pager(pg, { page: st.page, pages: pages, onChange: function (p) { st.page = p; draw(); } });
    return box;
  }

  function draw() {
    root.replaceChildren();
    root.setAttribute('data-ready', '1');
    root.append(el('header', { class: 'ak-page-head' }, [
      el('span', { class: 'ak-page-icon', text: '📝' }),
      el('div', { class: 'ak-page-head-text' }, [el('h1', { text: t('title') }), el('p', { text: t('desc') })]),
      el('div', { class: 'ak-actions' }, [el('button', { type: 'button', class: 'ak-btn ak-btn-primary', id: 'forms-new', onclick: newForm, text: '+ ' + t('new') })]),
    ]));
    root.append(el('div', { class: 'ak-alert', text: '💡 ' + t('howto') }));
    var main = el('section', { class: 'ak-card', id: 'forms-editor' });
    var d = st.draft;
    if (d) {
      var tabs = el('div', { class: 'ak-tabs' }, [
        el('button', { type: 'button', class: 'ak-tab', 'aria-selected': st.view === 'build' ? 'true' : 'false', onclick: function () { st.view = 'build'; draw(); }, text: '🛠 ' + t('build') }),
        d.id ? el('button', { type: 'button', class: 'ak-tab', 'aria-selected': st.view === 'subs' ? 'true' : 'false', onclick: loadSubs, text: '📥 ' + t('subs') + ' (' + (d.submissionCount || 0) + ')' }) : null,
      ]);
      main.append(tabs, st.view === 'build' ? builder() : inbox());
    }
    root.append(el('div', { class: 'forms-layout' }, [nav(), d ? main : null]));
  }

  var css = document.createElement('style');
  css.textContent = '.forms-layout{display:grid;grid-template-columns:260px 1fr;gap:12px;align-items:start}' +
    '@media (max-width:760px){.forms-layout{grid-template-columns:1fr}}' +
    '.forms-nav{display:flex;flex-direction:column;gap:4px;padding:8px}.forms-item{display:flex;flex-direction:column;align-items:flex-start;gap:2px;text-align:left;' +
    'border:1px solid transparent;border-radius:8px;padding:8px 10px;background:none;color:inherit;font:inherit;cursor:pointer}' +
    '.forms-item.on{border-color:var(--ak-primary,#4f46e5);background:var(--ak-muted-bg,rgba(79,70,229,.08))}' +
    '.forms-2{display:grid;grid-template-columns:1fr 1fr;gap:12px}@media (max-width:760px){.forms-2{grid-template-columns:1fr}}' +
    '.forms-fields{list-style:none;margin:0 0 8px;padding:0;display:flex;flex-direction:column;gap:6px}' +
    '.forms-field{display:flex;flex-wrap:wrap;gap:6px;align-items:center}.forms-field-label{flex:1 1 180px}.forms-options{flex:1 1 100%}' +
    '.forms-type{width:24px;text-align:center}.forms-unread td{font-weight:600}.forms-pick{margin-bottom:8px}';
  document.head.append(css);

  var first = st.forms.filter(function (f) { return f.id === boot.selected; })[0] || st.forms[0];
  if (first) select(first); else draw();
})();
