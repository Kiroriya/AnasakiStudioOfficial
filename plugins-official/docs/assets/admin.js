/* Docs admin page (site.adminPage "docs") — the bundle import and the settings forms.
   Drawn with the admin UI kit (window.AK); data through anasaki.call('docs:…'). All user text goes in with
   textContent / value, never innerHTML. */
(function () {
  'use strict';
  var M = {
    th: {
      impTitle: 'นำเข้าชุดเอกสาร (JSON)', impHint: 'รูปแบบ {"products":[…], "hubTitle", "hubIntro", "docs":[{slug,title,excerpt,product,section,order,sectionOrder,content}], "legacyRedirects":[{from,to}]} — ทุกช่องยกเว้น docs ใส่หรือไม่ก็ได้ · บทความที่มี slug ซ้ำจะถูกอัปเดต (ไม่สร้างซ้ำ)',
      file: 'เลือกไฟล์ .json', paste: 'หรือวาง JSON ที่นี่', status: 'สถานะของบทความใหม่', publish: 'เผยแพร่ทันที', draft: 'ฉบับร่าง',
      withRedirects: 'ใช้การตั้งค่าในชุดข้อมูลด้วย — products, hubTitle, hubIntro, legacyRedirects (ต้องเป็นผู้ดูแลระบบ)', run: 'นำเข้า', running: 'กำลังนำเข้า… {d}/{n}',
      bad: 'JSON ไม่ถูกต้อง: ', noDocs: 'ไม่พบ docs ในชุดข้อมูล', done: 'นำเข้าเสร็จ: สร้าง {c} · อัปเดต {u} · ผิดพลาด {e}',
      reload: 'โหลดรายการใหม่', setTitle: 'การตั้งค่าศูนย์รวมเอกสาร', hubTitle: 'หัวข้อหน้า /docs', hubIntro: 'คำแนะนำใต้หัวข้อ',
      products: 'ผลิตภัณฑ์ / หมวดใหญ่', productsHint: 'แต่ละรายการเป็นการ์ดบนหน้า /docs และ URL /docs/<รหัส> — บทความเลือกผลิตภัณฑ์ด้วยรหัสนี้',
      pIcon: 'ไอคอน', pId: 'รหัส (a-z 0-9 -)', pNameTh: 'ชื่อ (ไทย)', pNameEn: 'ชื่อ (อังกฤษ)', pDescTh: 'คำอธิบาย (ไทย)', pDescEn: 'คำอธิบาย (อังกฤษ)',
      pAdd: '＋ เพิ่มผลิตภัณฑ์', pLocked: 'มีบทความ {n} บทใช้รหัสนี้ — เปลี่ยนรหัสไม่ได้ (ย้ายบทความออกก่อน)', pDocs: '{n} บทความ',
      redirects: 'เปลี่ยนเส้นทางจากลิงก์เดิม', redirectsHint: 'ลิงก์เดิม เช่น /blog/old-post → ลิงก์ใหม่ เช่น /docs/guide/new — ตอบ 301 ก่อนถึงหน้าเว็บ',
      rFrom: 'ลิงก์เดิม', rTo: 'ลิงก์ใหม่', rAdd: '＋ เพิ่มการเปลี่ยนเส้นทาง', remove: 'ลบ', up: 'เลื่อนขึ้น', down: 'เลื่อนลง',
      general: 'ทั่วไป', editUrl: 'ลิงก์ “แก้ไขหน้านี้”', editUrlHint: 'เว้นว่าง = ไม่แสดง · เช่น https://github.com/<org>/<repo>/edit/main/docs/{product}/{slug}.md — ใช้ {id} {slug} {product} ได้ · ทุกคนเห็นลิงก์นี้ จึงไม่ควรใส่ที่อยู่หลังบ้าน',
      save: 'บันทึก', saved: 'บันทึกแล้ว', onlyAdmin: 'เฉพาะผู้ดูแลระบบแก้การตั้งค่าได้', errSlug: 'slug', errMsg: 'ปัญหา',
      saveOrder: 'บันทึกลำดับ', orderSaved: 'บันทึกลำดับแล้ว ({n} บทความ)', orderDirty: 'ลำดับยังไม่ได้บันทึก', cancel: 'ยกเลิก',
      rename: 'เปลี่ยนชื่อหมวด', renameHint: 'ชื่อหมวดใหม่ (Enter = ตกลง, Esc = ยกเลิก)',
    },
    en: {
      impTitle: 'Import a bundle (JSON)', impHint: 'Format {"products":[…], "hubTitle", "hubIntro", "docs":[{slug,title,excerpt,product,section,order,sectionOrder,content}], "legacyRedirects":[{from,to}]} — only docs is required · existing slugs are updated (never duplicated)',
      file: 'Choose a .json file', paste: 'or paste JSON here', status: 'Status of new articles', publish: 'Published', draft: 'Draft',
      withRedirects: 'Also apply the bundle\'s settings — products, hubTitle, hubIntro, legacyRedirects (administrators)', run: 'Import', running: 'Importing… {d}/{n}',
      bad: 'Invalid JSON: ', noDocs: 'No docs in the bundle', done: 'Done: {c} created · {u} updated · {e} failed',
      reload: 'Reload the list', setTitle: 'Docs hub settings', hubTitle: 'Title of /docs', hubIntro: 'Intro under the title',
      products: 'Products / top-level groups', productsHint: 'Each one is a card on /docs and the URL /docs/<id> — articles pick their product by this id',
      pIcon: 'Icon', pId: 'Id (a-z 0-9 -)', pNameTh: 'Name (Thai)', pNameEn: 'Name (English)', pDescTh: 'Description (Thai)', pDescEn: 'Description (English)',
      pAdd: '＋ Add a product', pLocked: '{n} articles use this id — it cannot change (move them first)', pDocs: '{n} articles',
      redirects: 'Redirects from old links', redirectsHint: 'Old link, e.g. /blog/old-post → new link, e.g. /docs/guide/new — answered with 301 before routing',
      rFrom: 'Old link', rTo: 'New link', rAdd: '＋ Add a redirect', remove: 'Remove', up: 'Move up', down: 'Move down',
      general: 'General', editUrl: '“Edit this page” link', editUrlHint: 'Empty = hidden · e.g. https://github.com/<org>/<repo>/edit/main/docs/{product}/{slug}.md — {id} {slug} {product} are replaced · everyone sees it, so do not put the admin address here',
      save: 'Save', saved: 'Saved', onlyAdmin: 'Only administrators can change the settings', errSlug: 'slug', errMsg: 'Problem',
      saveOrder: 'Save order', orderSaved: 'Order saved ({n} articles)', orderDirty: 'The order is not saved yet', cancel: 'Cancel',
      rename: 'Rename section', renameHint: 'New section name (Enter = OK, Esc = cancel)',
    },
  };
  var data = {};
  try { data = JSON.parse(document.getElementById('docs-data').textContent || '{}'); } catch (e) { /* empty */ }
  var t = M[data.locale === 'en' ? 'en' : 'th'];
  var fmt = function (s, o) { return s.replace(/\{(\w+)\}/g, function (_, k) { return o[k]; }); };
  var toast = function (m, k) { if (window.AK && AK.toast) AK.toast(m, k); else window.alert(m); };

  function el(tag, attrs, kids) {
    var e = document.createElement(tag);
    Object.keys(attrs || {}).forEach(function (k) {
      if (k === 'text') e.textContent = attrs[k];
      else if (k === 'className') e.className = attrs[k];
      else e.setAttribute(k, attrs[k]);
    });
    (kids || []).forEach(function (c) { if (c) e.appendChild(c); });
    return e;
  }
  function field(label, input, hint) {
    return el('label', { className: 'ak-field' }, [el('span', { text: label }), input, hint ? el('p', { className: 'ak-hint', text: hint }) : null]);
  }
  function call(name, input) {
    return anasaki.call('docs:' + name, input).then(function (d) {
      if (d && d.errors && Object.keys(d.errors).length) throw new Error(d.message || d.errors.form || 'Error');
      return d ? d.data : null;
    });
  }

  // ------------------------------------------------------------ import
  var imp = document.getElementById('docs-import');
  if (imp) {
    var file = el('input', { type: 'file', accept: '.json,application/json' });
    var area = el('textarea', { className: 'ak-mono', rows: '10', placeholder: '{"docs":[…]}', spellcheck: 'false' });
    var status = el('select', {}, [el('option', { value: 'publish', text: t.publish }), el('option', { value: 'draft', text: t.draft })]);
    var withRed = el('input', { type: 'checkbox', checked: 'checked' });
    var btn = el('button', { type: 'button', className: 'ak-btn ak-btn-primary', text: t.run });
    var out = el('div', { 'aria-live': 'polite' });
    file.addEventListener('change', function () {
      var f = file.files && file.files[0];
      if (!f) return;
      var r = new FileReader();
      r.onload = function () { area.value = String(r.result || ''); };
      r.readAsText(f, 'utf-8');
    });
    imp.appendChild(el('div', { className: 'ak-card-head' }, [el('h2', { text: t.impTitle })]));
    imp.appendChild(el('p', { className: 'ak-hint', text: t.impHint }));
    imp.appendChild(field(t.file, file));
    imp.appendChild(field(t.paste, area));
    imp.appendChild(field(t.status, status));
    if (data.canSettings) imp.appendChild(el('label', { className: 'ak-check' }, [withRed, el('span', { text: ' ' + t.withRedirects })]));
    imp.appendChild(el('div', { className: 'ak-form-actions' }, [btn]));
    imp.appendChild(out);

    btn.addEventListener('click', function () {
      var bundle;
      try { bundle = JSON.parse(area.value); } catch (e) { toast(t.bad + e.message, 'error'); return; }
      var docs = bundle && Array.isArray(bundle.docs) ? bundle.docs : Array.isArray(bundle) ? bundle : null;
      if (!docs || !docs.length) { toast(t.noDocs, 'error'); return; }
      btn.disabled = true;
      out.textContent = '';
      var size = data.maxBatch || 25, done = 0, created = 0, updated = 0, errors = [];
      var batches = [];
      for (var i = 0; i < docs.length; i += size) batches.push(docs.slice(i, i + size));
      var prog = el('p', { className: 'ak-muted' });
      out.appendChild(prog);
      var step = function () {
        prog.textContent = fmt(t.running, { d: done, n: docs.length });
        if (!batches.length) return Promise.resolve();
        var b = batches.shift();
        return call('import', { docs: b, status: status.value }).then(function (r) {
          done += b.length; created += r.created; updated += r.updated; errors = errors.concat(r.errors || []);
          return step();
        });
      };
      // the bundle's own settings go first: its articles may use products this site does not have yet
      var conf = {};
      if (bundle && !Array.isArray(bundle) && data.canSettings && withRed.checked) {
        if (Array.isArray(bundle.products) && bundle.products.length) conf.products = bundle.products;
        if (Array.isArray(bundle.legacyRedirects)) conf.legacyRedirects = bundle.legacyRedirects;
        if (typeof bundle.hubTitle === 'string') conf.hubTitle = bundle.hubTitle;
        if (typeof bundle.hubIntro === 'string') conf.hubIntro = bundle.hubIntro;
      }
      var pre = Object.keys(conf).length ? call('saveSettings', conf) : Promise.resolve();
      pre.then(step).then(function () {
        prog.textContent = fmt(t.done, { c: created, u: updated, e: errors.length });
        toast(prog.textContent, errors.length ? 'warning' : 'success');
        if (errors.length) {
          var tb = el('tbody');
          errors.forEach(function (x) { tb.appendChild(el('tr', {}, [el('td', { className: 'ak-mono', text: x.slug }), el('td', { text: x.error })])); });
          out.appendChild(el('div', { className: 'ak-table-wrap' }, [el('table', { className: 'ak-table' }, [
            el('thead', {}, [el('tr', {}, [el('th', { text: t.errSlug }), el('th', { text: t.errMsg })])]), tb])]));
        }
        var re = el('button', { type: 'button', className: 'ak-btn', text: t.reload });
        re.addEventListener('click', function () { window.location.reload(); });
        out.appendChild(el('div', { className: 'ak-form-actions' }, [re]));
      }).catch(function (e) { toast(e && e.message ? e.message : String(e), 'error'); prog.textContent = ''; })
        .then(function () { btn.disabled = false; if (window.AK && AK.height) AK.height(); });
    });
  }

  // ------------------------------------------------------------ settings
  var set = document.getElementById('docs-settings');
  if (set) {
    set.appendChild(el('div', { className: 'ak-card-head' }, [el('h2', { text: t.setTitle })]));
    if (!data.canSettings) {
      set.appendChild(el('div', { className: 'ak-alert ak-alert-warn', text: t.onlyAdmin }));
    } else {
      var hubTitle = el('input', { className: 'ak-input', maxlength: '120', name: 'hubTitle' });
      hubTitle.value = data.hubTitle || '';
      var hubIntro = el('textarea', { rows: '2', maxlength: '500', name: 'hubIntro' });
      hubIntro.value = data.hubIntro || '';
      var editUrl = el('input', { className: 'ak-input ak-mono', maxlength: '500', name: 'editUrl', placeholder: 'https://…/{slug}.md' });
      editUrl.value = data.editUrl || '';
      var i18n = function (v, lang) { return typeof v === 'string' ? v : (v && v[lang]) || ''; };
      var input = function (value, attrs) { var n = el('input', attrs); n.value = value || ''; return n; };
      var rowBtns = function (list, row, redraw) {
        var b = function (text, label, fn) { var x = el('button', { type: 'button', className: 'ak-btn ak-btn-sm ak-btn-ghost', text: text, 'aria-label': label, title: label }); x.addEventListener('click', fn); return x; };
        return el('div', { className: 'docs-row-btns' }, [
          b('↑', t.up, function () { var i = list.indexOf(row); if (i > 0) { list.splice(i, 1); list.splice(i - 1, 0, row); redraw(); } }),
          b('↓', t.down, function () { var i = list.indexOf(row); if (i < list.length - 1) { list.splice(i, 1); list.splice(i + 1, 0, row); redraw(); } }),
          b('✕', t.remove, function () { list.splice(list.indexOf(row), 1); redraw(); }),
        ]);
      };

      // products: one row each (icon, id, names, descriptions); an id in use by articles is locked
      var prodRows = (Array.isArray(data.products) ? data.products : []).map(function (p) {
        return { icon: p.icon || '', id: p.id, nameTh: i18n(p.name, 'th'), nameEn: i18n(p.name, 'en'), descTh: i18n(p.desc, 'th'), descEn: i18n(p.desc, 'en'), saved: p.id };
      });
      var prodBox = el('div', { className: 'docs-rows', id: 'docs-products' });
      var drawProducts = function () {
        prodBox.textContent = '';
        prodRows.forEach(function (r) {
          var n = (data.counts || {})[r.saved] || 0;
          var bind = function (key, attrs) { var x = input(r[key], attrs); x.addEventListener('input', function () { r[key] = x.value; }); return x; };
          var id = bind('id', { className: 'ak-input ak-mono', name: 'p-id', placeholder: t.pId, 'aria-label': t.pId, maxlength: '40' });
          if (n) { id.readOnly = true; id.title = t.pLocked.replace('{n}', n); }
          prodBox.appendChild(el('div', { className: 'docs-row docs-prow' }, [
            bind('icon', { className: 'ak-input docs-icon', name: 'p-icon', placeholder: '📘', 'aria-label': t.pIcon, maxlength: '16' }),
            el('div', { className: 'docs-grow' }, [
              el('div', { className: 'docs-line' }, [id, el('span', { className: 'ak-muted ak-small', text: n ? t.pDocs.replace('{n}', n) : '' })]),
              el('div', { className: 'docs-line' }, [bind('nameTh', { className: 'ak-input', name: 'p-name-th', placeholder: t.pNameTh, 'aria-label': t.pNameTh, maxlength: '80' }),
                bind('nameEn', { className: 'ak-input', name: 'p-name-en', placeholder: t.pNameEn, 'aria-label': t.pNameEn, maxlength: '80' })]),
              el('div', { className: 'docs-line' }, [bind('descTh', { className: 'ak-input', name: 'p-desc-th', placeholder: t.pDescTh, 'aria-label': t.pDescTh, maxlength: '300' }),
                bind('descEn', { className: 'ak-input', name: 'p-desc-en', placeholder: t.pDescEn, 'aria-label': t.pDescEn, maxlength: '300' })]),
            ]),
            rowBtns(prodRows, r, drawProducts),
          ]));
        });
        if (window.AK && AK.height) AK.height();
      };
      var addProd = el('button', { type: 'button', className: 'ak-btn ak-btn-sm', text: t.pAdd, id: 'docs-add-product' });
      addProd.addEventListener('click', function () {
        prodRows.push({ icon: '📘', id: '', nameTh: '', nameEn: '', descTh: '', descEn: '', saved: '' });
        drawProducts();
        var last = prodBox.querySelectorAll('input[name="p-id"]');
        if (last.length) last[last.length - 1].focus();
      });
      drawProducts();

      // redirects: old path → new path
      var redRows = (Array.isArray(data.legacyRedirects) ? data.legacyRedirects : []).map(function (r) { return { from: r.from, to: r.to }; });
      var redBox = el('div', { className: 'docs-rows', id: 'docs-redirects' });
      var drawRedirects = function () {
        redBox.textContent = '';
        redRows.forEach(function (r) {
          var bind = function (key, attrs) { var x = input(r[key], attrs); x.addEventListener('input', function () { r[key] = x.value; }); return x; };
          redBox.appendChild(el('div', { className: 'docs-row' }, [
            bind('from', { className: 'ak-input ak-mono', name: 'r-from', placeholder: '/blog/old-post', 'aria-label': t.rFrom }),
            el('span', { className: 'ak-muted', text: '→', 'aria-hidden': 'true' }),
            bind('to', { className: 'ak-input ak-mono', name: 'r-to', placeholder: '/docs/guide/new', 'aria-label': t.rTo }),
            rowBtns(redRows, r, drawRedirects),
          ]));
        });
        if (window.AK && AK.height) AK.height();
      };
      var addRed = el('button', { type: 'button', className: 'ak-btn ak-btn-sm', text: t.rAdd, id: 'docs-add-redirect' });
      addRed.addEventListener('click', function () { redRows.push({ from: '', to: '' }); drawRedirects(); });
      drawRedirects();

      var save = el('button', { type: 'button', className: 'ak-btn ak-btn-primary', text: t.save, id: 'docs-save-settings' });
      [field(t.hubTitle, hubTitle), field(t.hubIntro, hubIntro), field(t.editUrl, editUrl, t.editUrlHint),
        el('div', { className: 'ak-field' }, [el('span', { text: t.products }), el('p', { className: 'ak-hint', text: t.productsHint }), prodBox, addProd]),
        el('div', { className: 'ak-field' }, [el('span', { text: t.redirects }), el('p', { className: 'ak-hint', text: t.redirectsHint }), redBox, addRed]),
        el('div', { className: 'ak-form-actions' }, [save])].forEach(function (n) { set.appendChild(n); });
      save.addEventListener('click', function () {
        var products = prodRows.map(function (r) {
          return { id: r.id.trim(), icon: r.icon.trim(), name: { th: r.nameTh.trim(), en: r.nameEn.trim() }, desc: { th: r.descTh.trim(), en: r.descEn.trim() } };
        });
        var reds = redRows.filter(function (r) { return r.from.trim() || r.to.trim(); }).map(function (r) { return { from: r.from.trim(), to: r.to.trim() }; });
        save.disabled = true;
        call('saveSettings', { products: products, legacyRedirects: reds, hubTitle: hubTitle.value, hubIntro: hubIntro.value, editUrl: editUrl.value })
          .then(function () {
            toast(t.saved, 'success');
            prodRows.forEach(function (r) { r.saved = r.id.trim(); });
          }, function (e) { toast(e.message, 'error'); })
          .then(function () { save.disabled = false; });
      });
    }
  }

  // ------------------------------------------------------------ order: drag docs and sections, rename sections
  var listEl = document.getElementById('docs-list');
  if (listEl && listEl.querySelector('.docs-sec')) {
    listEl.insertBefore(el('p', { className: 'ak-hint docs-drag-hint', text: data.dragHint || '' }), listEl.firstChild);
    var bar = el('div', { className: 'ak-alert docs-orderbar', hidden: 'hidden' });
    var saveOrder = el('button', { type: 'button', className: 'ak-btn ak-btn-sm ak-btn-primary', text: t.saveOrder, id: 'docs-save-order' });
    var cancelOrder = el('button', { type: 'button', className: 'ak-btn ak-btn-sm ak-btn-ghost', text: t.cancel });
    bar.appendChild(el('span', { text: t.orderDirty }));
    bar.appendChild(el('span', { className: 'docs-grow' }));
    bar.appendChild(cancelOrder);
    bar.appendChild(saveOrder);
    listEl.insertBefore(bar, listEl.firstChild);
    var dirty = function () { bar.hidden = false; if (window.AK && AK.height) AK.height(); };
    cancelOrder.addEventListener('click', function () { window.location.reload(); });

    var dragging = null; // a <tr> or a .docs-sec
    var grip = function (label) {
      var g = el('button', { type: 'button', className: 'docs-grip', text: '⠿', title: label, 'aria-label': label, draggable: 'true' });
      return g;
    };
    var moveBtns = function (target) {
      var up = el('button', { type: 'button', className: 'docs-mini', text: '↑', title: data.moveUp || t.up, 'aria-label': data.moveUp || t.up });
      var down = el('button', { type: 'button', className: 'docs-mini', text: '↓', title: data.moveDown || t.down, 'aria-label': data.moveDown || t.down });
      up.addEventListener('click', function () { var p = target.previousElementSibling; if (p && p.matches(target.tagName === 'TR' ? 'tr' : '.docs-sec')) { target.parentNode.insertBefore(target, p); dirty(); up.focus(); } });
      down.addEventListener('click', function () { var n = target.nextElementSibling; if (n && n.matches(target.tagName === 'TR' ? 'tr' : '.docs-sec')) { target.parentNode.insertBefore(n, target); dirty(); down.focus(); } });
      return [up, down];
    };
    Array.prototype.forEach.call(listEl.querySelectorAll('.docs-sec'), function (sec) {
      var head = sec.querySelector('.docs-sec-head');
      var g = grip(data.dragHint || '');
      head.insertBefore(g, head.firstChild);
      g.addEventListener('dragstart', function (e) { dragging = sec; sec.classList.add('is-dragging'); e.dataTransfer.effectAllowed = 'move'; try { e.dataTransfer.setData('text/plain', 'sec'); } catch (x) { /* IE */ } });
      g.addEventListener('dragend', function () { sec.classList.remove('is-dragging'); dragging = null; });
      var rn = el('button', { type: 'button', className: 'docs-mini', text: '✎', title: t.rename, 'aria-label': t.rename });
      rn.addEventListener('click', function () {
        var name = head.querySelector('.docs-sec-name');
        var inp = el('input', { className: 'ak-input docs-rename', maxlength: '80', 'aria-label': t.renameHint, title: t.renameHint });
        inp.value = sec.getAttribute('data-section') || '';
        name.hidden = true;
        head.insertBefore(inp, name);
        inp.focus();
        var done = function (ok) {
          if (ok) {
            var v = inp.value.trim();
            if (v !== (sec.getAttribute('data-section') || '')) { sec.setAttribute('data-section', v); name.textContent = v || t.general; dirty(); }
          }
          inp.remove(); name.hidden = false; rn.focus();
        };
        inp.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); done(true); } else if (e.key === 'Escape') { e.preventDefault(); done(false); } });
        inp.addEventListener('blur', function () { if (inp.isConnected) done(true); });
      });
      moveBtns(sec).concat([rn]).forEach(function (b) { head.appendChild(b); });
      sec.addEventListener('dragover', function (e) {
        if (!dragging) return;
        var sameProduct = dragging.closest('[data-product]') === sec.closest('[data-product]');
        if (!sameProduct) return;
        e.preventDefault();
        if (dragging.classList && dragging.classList.contains('docs-sec')) {
          if (dragging === sec) return;
          var r = sec.getBoundingClientRect();
          sec.parentNode.insertBefore(dragging, e.clientY < r.top + r.height / 2 ? sec : sec.nextSibling);
          dirty();
        } else {
          var tbody = sec.querySelector('tbody');
          var rows = Array.prototype.filter.call(tbody.children, function (x) { return x !== dragging; });
          var before = null;
          for (var i = 0; i < rows.length; i++) { var rr = rows[i].getBoundingClientRect(); if (e.clientY < rr.top + rr.height / 2) { before = rows[i]; break; } }
          if (dragging.parentNode !== tbody || dragging.nextSibling !== before) { tbody.insertBefore(dragging, before); dirty(); }
        }
      });
      sec.addEventListener('drop', function (e) { if (dragging) e.preventDefault(); });
    });
    Array.prototype.forEach.call(listEl.querySelectorAll('.docs-sec tbody tr[data-id]'), function (tr) {
      var cell = el('td', { className: 'docs-grip-cell' });
      var g = grip(data.dragHint || '');
      cell.appendChild(g);
      moveBtns(tr).forEach(function (b) { cell.appendChild(b); });
      tr.insertBefore(cell, tr.firstChild);
      g.addEventListener('dragstart', function (e) { dragging = tr; tr.classList.add('is-dragging'); e.dataTransfer.effectAllowed = 'move'; try { e.dataTransfer.setData('text/plain', tr.getAttribute('data-id')); } catch (x) { /* IE */ } });
      g.addEventListener('dragend', function () { tr.classList.remove('is-dragging'); dragging = null; });
    });
    Array.prototype.forEach.call(listEl.querySelectorAll('.docs-sec thead tr'), function (tr) { tr.insertBefore(el('th', { className: 'docs-grip-cell' }), tr.firstChild); });

    saveOrder.addEventListener('click', function () {
      var items = [];
      Array.prototype.forEach.call(listEl.querySelectorAll('[data-product]'), function (card) {
        Array.prototype.forEach.call(card.querySelectorAll('.docs-sec'), function (sec, si) {
          var name = sec.getAttribute('data-section') || '';
          Array.prototype.forEach.call(sec.querySelectorAll('tbody tr[data-id]'), function (tr, di) {
            items.push({ id: Number(tr.getAttribute('data-id')), section: name, order: (di + 1) * 10, sectionOrder: si + 1 });
          });
        });
      });
      saveOrder.disabled = true;
      call('reorder', { items: items })
        .then(function (r) {
          if (r && r.errors && r.errors.length) throw new Error(r.errors.map(function (x) { return '#' + x.id + ': ' + x.error; }).join(', '));
          toast(t.orderSaved.replace('{n}', r ? r.updated : 0), 'success');
          bar.hidden = true;
        })
        .catch(function (e) { toast(e.message, 'error'); })
        .then(function () { saveOrder.disabled = false; if (window.AK && AK.height) AK.height(); });
    });
  }

  // layout of the rows and the drag handles (admin UI kit tokens)
  var css = el('style');
  css.textContent =
    '.docs-rows{display:flex;flex-direction:column;gap:8px;margin:8px 0}' +
    '.docs-row{display:flex;align-items:center;gap:8px}.docs-row>.ak-input{flex:1;min-width:0}' +
    '.docs-prow{align-items:flex-start;padding:10px;border:1px solid var(--ak-border,#e6e8ef);border-radius:10px}' +
    '.docs-grow{flex:1;min-width:0;display:flex;flex-direction:column;gap:6px}.docs-line{display:flex;gap:6px;align-items:center}.docs-line>.ak-input{flex:1;min-width:0}' +
    '.docs-row>.ak-input.docs-icon{flex:0 0 60px;width:60px;text-align:center;font-size:18px}.docs-row-btns{display:flex;gap:2px;flex:none}' +
    '.docs-sec{border-top:1px solid var(--ak-border,#e6e8ef)}.docs-sec.is-dragging,tr.is-dragging{opacity:.45}' +
    '.docs-sec-head{display:flex;align-items:center;gap:6px;margin:0;padding:10px 16px 4px;font-size:14px}.docs-sec-name{flex:1}' +
    '.docs-grip{cursor:grab;border:0;background:none;color:var(--ak-muted,#6b7280);font-size:16px;line-height:1;padding:2px 4px;border-radius:6px}.docs-grip:active{cursor:grabbing}' +
    '.docs-mini{border:0;background:none;color:var(--ak-muted,#6b7280);cursor:pointer;padding:2px 5px;border-radius:6px;font-size:13px}.docs-mini:hover,.docs-grip:hover{background:var(--ak-hover,#f1f2f6)}' +
    '.docs-grip-cell{width:84px;white-space:nowrap}.docs-rename{max-width:280px}' +
    '.docs-orderbar[hidden]{display:none}.docs-orderbar{display:flex;align-items:center;gap:8px;position:sticky;top:0;z-index:2;margin-bottom:10px}.docs-drag-hint{margin:0 0 10px}';
  document.head.appendChild(css);
  if (window.AK && AK.height) AK.height();
})();
