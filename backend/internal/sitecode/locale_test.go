package sitecode

import (
	"testing"

	"github.com/dop251/goja"
)

func TestLocaleFormatting(t *testing.T) {
	v := &vm{rt: goja.New()}
	v.installLocale()
	// 2026-11-03 19:05:09 UTC, a Tuesday
	_, _ = v.rt.RunString(`var d = new Date(Date.UTC(2026, 10, 3, 19, 5, 9));`)
	for src, want := range map[string]string{
		`(1234).toLocaleString('th-TH')`:                                                                                   "1,234",
		`(1234567.891).toLocaleString('en-US')`:                                                                            "1,234,567.891",
		`(1234.5).toLocaleString()`:                                                                                        "1,234.5",
		`(-0.0004).toLocaleString('th')`:                                                                                   "0",
		`(1234.5).toLocaleString('th', { minimumFractionDigits: 2 })`:                                                      "1,234.50",
		`(2.5).toLocaleString('th', { maximumFractionDigits: 0 })`:                                                         "3",
		`(0.256).toLocaleString('en', { style: 'percent' })`:                                                               "26%",
		`(99).toLocaleString('th', { style: 'currency', currency: 'THB' })`:                                                "฿99.00",
		`(1e6).toLocaleString('th', { useGrouping: false })`:                                                               "1000000",
		`(NaN).toLocaleString('th')`:                                                                                       "NaN",
		`d.toLocaleDateString('th-TH', { timeZone: 'UTC' })`:                                                               "3/11/2569",
		`d.toLocaleDateString('en-US', { timeZone: 'UTC' })`:                                                               "11/3/2026",
		`d.toLocaleDateString('en-GB', { timeZone: 'UTC' })`:                                                               "3/11/2026",
		`d.toLocaleDateString('th', { timeZone: 'UTC', day: 'numeric', month: 'long', year: 'numeric' })`:                  "3 พฤศจิกายน 2569",
		`d.toLocaleDateString('th', { timeZone: 'UTC', month: 'short', day: 'numeric' })`:                                  "3 พ.ย.",
		`d.toLocaleDateString('en-US', { timeZone: 'UTC', month: 'long', day: 'numeric', year: 'numeric' })`:               "November 3, 2026",
		`d.toLocaleDateString('th', { timeZone: 'UTC', weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })`: "วันอังคารที่ 3 พฤศจิกายน 2569",
		`d.toLocaleTimeString('th', { timeZone: 'UTC' })`:                                                                  "19:05:09",
		`d.toLocaleTimeString('en-US', { timeZone: 'UTC', hour: 'numeric', minute: '2-digit' })`:                           "7:05 PM",
		`d.toLocaleString('th-TH', { timeZone: 'UTC' })`:                                                                   "3/11/2569 19:05:09",
		`new Date(NaN).toLocaleDateString('th')`:                                                                           "Invalid Date",
	} {
		got, err := v.rt.RunString(src)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if got.String() != want {
			t.Errorf("%s = %q, want %q", src, got.String(), want)
		}
	}
	// without a locale a date keeps goja's own format
	if got, _ := v.rt.RunString(`d.toLocaleString() === Object.getPrototypeOf(d).toLocaleString.call(d)`); !got.ToBoolean() {
		t.Error("no-locale date")
	}
	if got, _ := v.rt.RunString(`Object.keys(Number.prototype).length + Object.keys(Date.prototype).length`); got.ToInteger() != 0 {
		t.Error("polyfill must not add enumerable properties")
	}
}
