package sitecode

import (
	"context"
	"fmt"
	"strings"
)

// GeneratedTypes renders row interfaces for the developer tables of snap
// (declaration-merged into TableRows so db.table('orders') is typed).
func GeneratedTypes(snap *Snapshot) string {
	if snap == nil || len(snap.Tables) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n// ---------------------------------------------------------------- generated from the database\n")
	b.WriteString("// Row types of your tables (regenerated after each publish). / type ของแถวในตารางของคุณ\n")
	var entries []string
	for _, name := range snap.Names() {
		t := snap.Tables[name]
		iface := pascal(name) + "Row"
		fmt.Fprintf(&b, "\n/** Table `%s`. */\ninterface %s {\n", name, iface)
		for _, c := range t.Columns {
			ts := tsType(c.Type)
			if c.Nullable {
				ts += " | null"
			}
			doc := c.Type
			if c.Comment != "" {
				doc += " — " + strings.ReplaceAll(c.Comment, "*/", "* /")
			}
			fmt.Fprintf(&b, "  /** %s */\n  %s: %s;\n", doc, c.Name, ts)
		}
		b.WriteString("}\n")
		entries = append(entries, fmt.Sprintf("  %s: %s;", name, iface))
	}
	b.WriteString("\ninterface TableRows {\n" + strings.Join(entries, "\n") + "\n}\n")
	return b.String()
}

func pascal(s string) string {
	var b strings.Builder
	for _, p := range strings.Split(s, "_") {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	out := b.String()
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "T" + out
	}
	return out
}

// tsType maps a MySQL column type to the TypeScript type scripts receive.
func tsType(t string) string {
	t = strings.ToLower(t)
	switch {
	case strings.HasPrefix(t, "tinyint") && !strings.Contains(t, "unsigned"):
		return "boolean"
	case strings.HasPrefix(t, "decimal"):
		return "string"
	case strings.HasPrefix(t, "bigint"):
		return "number"
	case strings.HasPrefix(t, "int"), strings.HasPrefix(t, "smallint"), strings.HasPrefix(t, "mediumint"), strings.HasPrefix(t, "tinyint"),
		strings.HasPrefix(t, "float"), strings.HasPrefix(t, "double"), strings.HasPrefix(t, "year"):
		return "number"
	case t == "json":
		return "any"
	case strings.HasPrefix(t, "enum("):
		vals := splitEnum(strings.TrimSuffix(strings.TrimPrefix(t, "enum("), ")"))
		for i, v := range vals {
			vals[i] = fmt.Sprintf("%q", strings.ReplaceAll(v, "''", "'"))
		}
		return strings.Join(vals, " | ")
	}
	return "string"
}

// TypesFile is anasaki.d.ts plus the generated row types of the current
// developer tables and the modules of the active JS plugins.
func (e *Engine) TypesFile() string {
	mods := moduleTypes(e.pluginModules())
	if e.data == nil {
		return Types + mods
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	snap, err := e.data.LoadSnapshot(ctx)
	if err != nil {
		return Types + mods
	}
	return Types + GeneratedTypes(snap) + mods
}
