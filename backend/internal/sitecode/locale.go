package sitecode

import (
	"encoding/base64"
	"strings"

	"github.com/dop251/goja"
)

// localeJS replaces goja's toLocaleString family (2.1.1). There is no Intl in
// the runtime, and goja's Number.prototype.toLocaleString passes its first
// argument on as a radix, so (1234).toLocaleString('th-TH') threw "radix
// argument must be between 2 and 36". The replacement formats plainly and
// never throws: numbers get "," grouping and the min/max fraction digits,
// percent and currency of the options; dates follow the locale's order
// (th: day/month/Buddhist year, en-US: month/day/year, others: day/month/year)
// with Thai and English month and weekday names for { month, weekday }.
// Without a locale a date keeps the server's format.
const localeJS = `(function () {
  var NP = Number.prototype, DP = Date.prototype;
  var origDate = DP.toLocaleString, origDay = DP.toLocaleDateString, origTime = DP.toLocaleTimeString;
  var TH_M = ['มกราคม','กุมภาพันธ์','มีนาคม','เมษายน','พฤษภาคม','มิถุนายน','กรกฎาคม','สิงหาคม','กันยายน','ตุลาคม','พฤศจิกายน','ธันวาคม'];
  var TH_MS = ['ม.ค.','ก.พ.','มี.ค.','เม.ย.','พ.ค.','มิ.ย.','ก.ค.','ส.ค.','ก.ย.','ต.ค.','พ.ย.','ธ.ค.'];
  var TH_D = ['วันอาทิตย์','วันจันทร์','วันอังคาร','วันพุธ','วันพฤหัสบดี','วันศุกร์','วันเสาร์'];
  var TH_DS = ['อา.','จ.','อ.','พ.','พฤ.','ศ.','ส.'];
  var EN_M = ['January','February','March','April','May','June','July','August','September','October','November','December'];
  var EN_D = ['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday'];
  var SYM = { THB: '฿', USD: '$', EUR: '€', GBP: '£', JPY: '¥', CNY: '¥' };
  function lang(l) {
    if (Array.isArray(l)) l = l[0];
    return typeof l === 'string' ? l.toLowerCase() : '';
  }
  function opt(o, k) { return o && typeof o === 'object' ? o[k] : undefined; }
  function digits(x, d) {
    x = Number(x);
    return isFinite(x) && x >= 0 && x <= 20 ? Math.floor(x) : d;
  }
  function fmtNum(n, l, o) {
    n = Number(n);
    if (isNaN(n)) return 'NaN';
    if (!isFinite(n)) return n < 0 ? '-∞' : '∞';
    var style = opt(o, 'style'), cur = String(opt(o, 'currency') || '').toUpperCase();
    if (style === 'percent') n = n * 100;
    var defMin = style === 'currency' ? 2 : 0, defMax = style === 'currency' ? 2 : style === 'percent' ? 0 : 3;
    var min = digits(opt(o, 'minimumFractionDigits'), defMin);
    var max = digits(opt(o, 'maximumFractionDigits'), Math.max(min, defMax));
    if (max < min) max = min;
    var s = Math.abs(n).toFixed(max);
    if (max > min && s.indexOf('.') >= 0) {
      var cut = s.length;
      while (cut > s.length - (max - min) && s.charAt(cut - 1) === '0') cut--;
      s = s.slice(0, cut);
      if (s.charAt(s.length - 1) === '.') s = s.slice(0, -1);
    }
    var parts = s.split('.');
    var whole = opt(o, 'useGrouping') === false ? parts[0] : parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    var out = whole + (parts[1] ? '.' + parts[1] : '');
    if (n < 0 && /[1-9]/.test(out)) out = '-' + out;
    if (style === 'percent') return out + '%';
    if (style === 'currency' && cur) return SYM[cur] ? SYM[cur] + out : cur + ' ' + out;
    return out;
  }
  function pad(x) { return (x < 10 ? '0' : '') + x; }
  function parts(d, o) {
    var utc = String(opt(o, 'timeZone') || '').toUpperCase() === 'UTC';
    return utc
      ? { y: d.getUTCFullYear(), m: d.getUTCMonth(), d: d.getUTCDate(), w: d.getUTCDay(), h: d.getUTCHours(), mi: d.getUTCMinutes(), s: d.getUTCSeconds() }
      : { y: d.getFullYear(), m: d.getMonth(), d: d.getDate(), w: d.getDay(), h: d.getHours(), mi: d.getMinutes(), s: d.getSeconds() };
  }
  function fmtDay(p, l, o) {
    var th = l.indexOf('th') === 0, us = l === 'en-us' || l === 'en';
    var year = th ? p.y + 543 : p.y;
    var month = opt(o, 'month'), wd = opt(o, 'weekday');
    var hasParts = month || opt(o, 'day') || opt(o, 'year') || wd;
    var out;
    if (month === 'long' || month === 'short') {
      var mn = th ? (month === 'long' ? TH_M : TH_MS)[p.m] : (month === 'long' ? EN_M[p.m] : EN_M[p.m].slice(0, 3));
      var bits = [];
      if (!hasParts || opt(o, 'day')) bits.push(p.d);
      bits.push(mn);
      if (!hasParts || opt(o, 'year')) bits.push(year);
      out = us && bits.length === 3 ? mn + ' ' + p.d + ', ' + year : bits.join(' ');
    } else {
      var dd = opt(o, 'day') === '2-digit' ? pad(p.d) : p.d, mm = month === '2-digit' ? pad(p.m + 1) : p.m + 1;
      out = us ? mm + '/' + dd + '/' + year : dd + '/' + mm + '/' + year;
    }
    if (wd === 'long' || wd === 'short') {
      var w = th ? (wd === 'long' ? TH_D : TH_DS)[p.w] : (wd === 'long' ? EN_D[p.w] : EN_D[p.w].slice(0, 3));
      out = w + (th ? 'ที่ ' : ', ') + out;
    }
    return out;
  }
  function fmtTime(p, l, o) {
    var sec = opt(o, 'second') !== undefined || opt(o, 'hour') === undefined;
    if (l === 'en-us' || l === 'en') {
      var h = p.h % 12 || 12;
      return h + ':' + pad(p.mi) + (sec ? ':' + pad(p.s) : '') + (p.h < 12 ? ' AM' : ' PM');
    }
    return pad(p.h) + ':' + pad(p.mi) + (sec ? ':' + pad(p.s) : '');
  }
  function bad(d) { return isNaN(d.getTime()); }
  function hasDate(o) { return opt(o, 'year') || opt(o, 'month') || opt(o, 'day') || opt(o, 'weekday'); }
  function hasTime(o) { return opt(o, 'hour') || opt(o, 'minute') || opt(o, 'second'); }
  function def(p, v) { Object.defineProperty(p, v[0], { value: v[1], writable: true, configurable: true, enumerable: false }); }
  def(NP, ['toLocaleString', function toLocaleString(l, o) { return fmtNum(this, lang(l), o); }]);
  def(DP, ['toLocaleString', function toLocaleString(l, o) {
    var lg = lang(l);
    if (!lg && !o) return origDate.call(this);
    if (bad(this)) return 'Invalid Date';
    var p = parts(this, o);
    if (hasDate(o) && !hasTime(o)) return fmtDay(p, lg, o);
    if (hasTime(o) && !hasDate(o)) return fmtTime(p, lg, o);
    return fmtDay(p, lg, o) + (lg.indexOf('th') === 0 ? ' ' : ', ') + fmtTime(p, lg, o);
  }]);
  def(DP, ['toLocaleDateString', function toLocaleDateString(l, o) {
    var lg = lang(l);
    if (!lg && !o) return origDay.call(this);
    return bad(this) ? 'Invalid Date' : fmtDay(parts(this, o), lg, o);
  }]);
  def(DP, ['toLocaleTimeString', function toLocaleTimeString(l, o) {
    var lg = lang(l);
    if (!lg && !o) return origTime.call(this);
    return bad(this) ? 'Invalid Date' : fmtTime(parts(this, o), lg, o);
  }]);
})();`

var localeProg = goja.MustCompile("locale.js", localeJS, true)

func (v *vm) installLocale() {
	if _, err := v.rt.RunProgram(localeProg); err != nil {
		panic(err) // a bug in localeJS: the tests run it
	}
	// atob / btoa (2.2.0), as in browsers: base64 ↔ a "binary string" (one char per byte)
	_ = v.rt.Set("btoa", func(c goja.FunctionCall) goja.Value {
		s := c.Argument(0).String()
		b := make([]byte, 0, len(s))
		for _, r := range s {
			if r > 0xff {
				v.throw("btoa: ข้อความมีตัวอักษรนอกช่วง Latin1 (เช่น ภาษาไทย) — แปลงเป็นไบต์ UTF-8 ก่อน: btoa(unescape(encodeURIComponent(s)))")
			}
			b = append(b, byte(r))
		}
		return v.rt.ToValue(base64.StdEncoding.EncodeToString(b))
	})
	_ = v.rt.Set("atob", func(c goja.FunctionCall) goja.Value {
		s := strings.Join(strings.Fields(c.Argument(0).String()), "") // browsers drop ASCII whitespace
		if m := len(s) % 4; m != 0 {
			s += strings.Repeat("=", 4-m)
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			v.throw("atob: base64 ไม่ถูกต้อง")
		}
		r := make([]rune, len(b))
		for i, x := range b {
			r[i] = rune(x)
		}
		return v.rt.ToValue(string(r))
	})
}
