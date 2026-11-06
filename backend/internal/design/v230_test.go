package design

import (
	"strings"
	"testing"
)

// 2.3.0 (Qraft feedback 30): a button icon is 1.3em by default and props.iconSize sets it in px.
func TestButtonIconSize(t *testing.T) {
	doc := `{"id":"root","type":"frame","children":[
	 {"id":"a","type":"button","props":{"text":"Download","icon":"download"}},
	 {"id":"b","type":"button","props":{"text":"Go","icon":"download","iconSize":22}},
	 {"id":"c","type":"button","props":{"text":"Huge","icon":"download","iconSize":5000}}]}`
	h := renderDoc(t, doc, Context{}, nil).HTML
	must(t, h, `<a class="n-a rea-btn" href="#"><svg class="rea-btn-ic" viewBox="0 0 24 24" width="1.3em" height="1.3em"`,
		`<a class="n-b rea-btn" href="#"><svg class="rea-btn-ic" viewBox="0 0 24 24" width="22px" height="22px"`,
		`<a class="n-c rea-btn" href="#"><svg class="rea-btn-ic" viewBox="0 0 24 24" width="1.3em" height="1.3em"`)
}

// 2.3.0 (Qraft feedback 1): "stack on phones" is a mobile override of the row's layout mode.
func TestStackOnMobile(t *testing.T) {
	doc := `{"id":"root","type":"frame","children":[
	 {"id":"row","type":"frame","layout":{"mode":"horizontal","gap":12},"responsive":{"mobile":{"layout":{"mode":"vertical"}}},
	  "children":[{"id":"a","type":"text","props":{"text":"A"}},{"id":"b","type":"text","props":{"text":"B"}}]}]}`
	out := renderDoc(t, doc, Context{}, nil)
	i := strings.Index(out.CSS, "@media (max-width:640px)")
	if i < 0 || !strings.Contains(out.CSS[i:], `.n-row{`) || !strings.Contains(out.CSS[i:], "flex-direction:column") {
		t.Fatalf("no mobile column rule:\n%s", out.CSS)
	}
}
