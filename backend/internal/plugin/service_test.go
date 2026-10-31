package plugin

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Same vector as sdk/go/anasaki: both sides must sign identically.
const sigVector = "40e6b9f8aed3e8e9b9eda1a1f7ed7239126bcf97f717cc17196f45d02a4b6c8d"

func TestServiceSignature(t *testing.T) {
	if got := ServiceSignature("s3cret", 1700000000, "post", "/blocks/x?y=1", []byte(`{"a":1}`)); got != sigVector {
		t.Fatalf("signature %s, want %s", got, sigVector)
	}
	body := []byte(`{"x":true}`)
	r := httptest.NewRequest("POST", "/api/service/events?a=b", strings.NewReader(string(body)))
	ts := time.Now().Unix()
	r.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	r.Header.Set(HeaderSignature, ServiceSignature("k", ts, "POST", "/api/service/events?a=b", body))
	if err := VerifyServiceSignature("k", r, body); err != nil {
		t.Fatalf("valid request refused: %v", err)
	}
	if VerifyServiceSignature("other", r, body) == nil || VerifyServiceSignature("k", r, []byte(`{}`)) == nil {
		t.Fatal("wrong secret or body accepted")
	}
	old := time.Now().Add(-10 * time.Minute).Unix()
	r.Header.Set(HeaderTimestamp, strconv.FormatInt(old, 10))
	r.Header.Set(HeaderSignature, ServiceSignature("k", old, "POST", "/api/service/events?a=b", body))
	if VerifyServiceSignature("k", r, body) == nil {
		t.Fatal("stale request accepted")
	}
}

func TestServiceManifest(t *testing.T) {
	ok := &ManifestService{Public: []string{"/shop", "/shop/cart"}, Events: []string{"post.published", "*"},
		Collections: []ServiceCollection{{Source: "products", Label: "Products"}},
		Admin:       []ServiceAdminPage{{Path: "orders", Label: "Orders", Cap: "manage_options"}}, API: []string{"site", "mail"}}
	if err := ok.validate(); err != nil {
		t.Fatalf("valid service refused: %v", err)
	}
	for _, bad := range []*ManifestService{
		{Public: []string{"/api/x"}}, {Public: []string{"/admin"}}, {Public: []string{"shop"}}, {Public: []string{"/../x"}},
		{API: []string{"database"}}, {Admin: []ServiceAdminPage{{Path: "../x", Label: "x"}}},
		{Collections: []ServiceCollection{{Source: "x", Label: ""}}},
	} {
		if bad.validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
