package sitecode

import (
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

func ref(id string, at int, req map[string]string) plugin.CodeUnitRef {
	return plugin.CodeUnitRef{ID: id, Version: "1.0.0", Active: true, Requires: req,
		InstalledAt: time.Date(2026, 1, 1, 0, 0, at, 0, time.UTC)}
}

func ids(refs []plugin.CodeUnitRef) string {
	var out []string
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return strings.Join(out, ",")
}

func TestTopoOrder(t *testing.T) {
	// requires first, ties by install time
	order, errs := topoOrder([]plugin.CodeUnitRef{
		ref("shop", 1, map[string]string{"money": "^1.0.0"}),
		ref("money", 3, nil),
		ref("blog-x", 2, nil),
		ref("tax", 0, map[string]string{"money": "^1", "shop": ">=1.0.0"}),
	})
	if len(errs) != 0 || ids(order) != "blog-x,money,shop,tax" {
		t.Fatalf("order %s errs %v", ids(order), errs)
	}
	// cycles: a ↔ b not loaded; c (depends on a) not loaded; d fine
	order, errs = topoOrder([]plugin.CodeUnitRef{
		ref("aaa", 0, map[string]string{"bbb": "*"}),
		ref("bbb", 1, map[string]string{"aaa": "*"}),
		ref("ccc", 2, map[string]string{"aaa": "*"}),
		ref("ddd", 3, nil),
	})
	if ids(order) != "ddd" || !strings.Contains(errs["aaa"], "วง") || !strings.Contains(errs["bbb"], "วง") || errs["ccc"] == "" {
		t.Fatalf("order %s errs %v", ids(order), errs)
	}
	// missing and incompatible dependencies, and what depends on them
	order, errs = topoOrder([]plugin.CodeUnitRef{
		ref("eee", 0, map[string]string{"nope": "^1.0.0"}),
		ref("fff", 1, map[string]string{"ggg": "^2.0.0"}),
		ref("ggg", 2, nil),
		ref("hhh", 3, map[string]string{"eee": "*"}),
	})
	if ids(order) != "ggg" || !strings.Contains(errs["eee"], "nope") || !strings.Contains(errs["fff"], "^2.0.0") ||
		!strings.Contains(errs["hhh"], "eee") {
		t.Fatalf("order %s errs %v", ids(order), errs)
	}
}

func TestUnitNaming(t *testing.T) {
	d := &Data{N: Naming{Prefix: "sc_", Fallback: true}}
	n := d.UnitNaming("my-shop", true)
	if n.Prefix != "sc_p_my_shop_" || n.Phys("orders") != "sc_p_my_shop_orders" {
		t.Fatalf("naming %+v", n)
	}
	if _, err := NewQuery(n, "core_users"); err == nil || !strings.Contains(err.Error(), "ไม่มีสิทธิ์ใช้ content") {
		t.Fatalf("core view without content scope: %v", err)
	}
	for _, other := range []string{"p_money_rates", "sc_x", "script_jobs"} {
		if _, err := NewQuery(n, other); err == nil {
			t.Fatalf("%s must be refused", other)
		}
	}
	if _, err := NewQuery(Naming{}, "p_shop_orders"); err == nil {
		t.Fatal("Site Code must not reach a plugin's tables")
	}
	if err := ValidTableName("p_x"); err == nil {
		t.Fatal("p_ is reserved")
	}
	q, _ := NewQuery(n, "orders as o")
	if s, _ := q.CountSQL(100); !strings.Contains(s, "`sc_p_my_shop_orders` AS `o`") {
		t.Fatalf("sql %s", s)
	}
	u, local := SplitUnit("shop:products")
	if u != "shop" || local != "products" {
		t.Fatal(u, local)
	}
	if u, local := SplitUnit("products"); u != SiteUnit || local != "products" {
		t.Fatal(u, local)
	}
}
