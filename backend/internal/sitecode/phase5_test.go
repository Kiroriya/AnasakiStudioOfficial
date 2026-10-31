package sitecode

import (
	"errors"
	"strings"
	"testing"
)

func TestLibraryExportsAndModuleTypes(t *testing.T) {
	files := map[string]string{
		"code/lib/index.ts": "export * from './fmt'\nexport const VERSION = '1'\nexport default function hello() {}\n",
		"code/lib/fmt.ts":   "export function baht(n: number) { return n + ' บาท' }\nexport class Money {}\n",
	}
	names := libraryExports(files, "code/lib/index.ts")
	if strings.Join(names, ",") != "Money,VERSION,baht,default" {
		t.Fatalf("library exports = %v", names)
	}
	out := moduleTypes([]moduleInfo{
		{ID: "thai", Version: "1.0.0", Library: names, Exports: []string{"createOrder", "baht"}},
		{ID: "money", Version: "2.0.0", DTS: "export declare function fmt(n: number): string;\ndeclare const x: number;\nexport interface Price { amount: number }\n"},
	})
	for _, want := range []string{
		"declare module 'money' {\n  export function fmt(n: number): string;\n  const x: number;\n  export interface Price { amount: number }\n}",
		"declare module 'anasaki:money' {\n  export * from 'money';\n}",
		"declare module 'thai' {",
		"export const baht: any;",
		"export const Money: any;",
		"export default _default;",
		"export function createOrder(...args: any[]): any;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Count(out, "baht") != 1 { // a library name wins over a proxy of the same name
		t.Errorf("duplicate baht:\n%s", out)
	}
	if moduleTypes(nil) != "" {
		t.Error("no plugins → nothing")
	}
	if libraryExports(map[string]string{"a.ts": "export const ("}, "a.ts") != nil {
		t.Error("broken library should give no names")
	}
}

func TestPlanSync(t *testing.T) {
	cur := map[string]string{"main.ts": "a", "lib.ts": "b"}
	s := func(v string) *string { return &v }
	h := ContentHash("a")
	plan, err := PlanSync(cur, []SyncPut{{Path: "main.ts", Content: s("a"), BaseSHA256: s(h)}, {Path: "new.ts", Content: s("n"), BaseSHA256: s("")}},
		[]SyncDelete{{Path: "lib.ts"}, {Path: "gone.ts"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Write) != 1 || plan.Write["new.ts"] != "n" || strings.Join(plan.Remove, ",") != "lib.ts" || len(plan.Result) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	_, err = PlanSync(cur, []SyncPut{{Path: "main.ts", Content: s("x"), BaseSHA256: s("old")}}, []SyncDelete{{Path: "lib.ts", BaseSHA256: s("")}})
	var c *SyncConflicts
	if !errors.As(err, &c) || len(c.List) != 2 || c.List[0].ServerSHA256 != h || c.List[1].ServerSHA256 != ContentHash("b") {
		t.Fatalf("conflicts = %v", err)
	}
	var pe *PathError
	if _, err := PlanSync(cur, []SyncPut{{Path: "Main.ts", Content: s("x")}}, nil); !errors.As(err, &pe) {
		t.Fatalf("case duplicate: %v", err)
	}
}
