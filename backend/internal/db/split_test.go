package db

import (
	"strings"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	script := "-- a comment; with a semicolon\n" +
		"CREATE TABLE a (x VARCHAR(10) DEFAULT 'a;b', y TEXT COMMENT \"it's; fine\") ENGINE=InnoDB;\n" +
		"# hash comment ; here\n" +
		"/* block ; comment */ INSERT INTO a VALUES ('it''s;', 'back\\'slash;');\n" +
		"CREATE TABLE `odd;name` (id INT); -- trailing ; comment\n" +
		";;\n" +
		"SELECT 1 - -1"
	got := SplitStatements(script)
	want := []string{
		"CREATE TABLE a (x VARCHAR(10) DEFAULT 'a;b', y TEXT COMMENT \"it's; fine\") ENGINE=InnoDB",
		"INSERT INTO a VALUES ('it''s;', 'back\\'slash;')",
		"CREATE TABLE `odd;name` (id INT)",
		"SELECT 1 - -1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d statements: %q", len(got), got)
	}
	for i := range want {
		if strings.Join(strings.Fields(got[i]), " ") != want[i] {
			t.Errorf("statement %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
	// the embedded schema splits into CREATE TABLE statements only
	for _, s := range SplitStatements(schema) {
		if !strings.HasPrefix(s, "CREATE TABLE IF NOT EXISTS") {
			t.Errorf("unexpected schema statement: %.60q", s)
		}
	}
	for _, tbl := range []string{"password_resets", "demo_meta"} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+tbl) {
			t.Errorf("schema.sql must create %s", tbl)
		}
	}
}
