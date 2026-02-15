package main

import (
	"bytes"
	"os"
	"testing"
)

func TestWalkPlanSeqScan(t *testing.T) {
	node := &PlanNode{NodeType: "Seq Scan", Relation: "users", Cost: 500, Rows: 5000, Filter: "(age > 25)"}
	issues := WalkPlan(node, 1000)
	if len(issues) == 0 {
		t.Fatal("expected seq scan warning")
	}
	if issues[0].Severity != "warning" {
		t.Errorf("got severity %q, want warning", issues[0].Severity)
	}
	if issues[0].Fix == "" {
		t.Error("expected index suggestion in Fix field")
	}
}

func TestWalkPlanHighCost(t *testing.T) {
	node := &PlanNode{NodeType: "Hash Join", Cost: 5000, Rows: 100,
		Children: []PlanNode{{NodeType: "Index Scan", Relation: "orders", Cost: 100, Rows: 50}}}
	issues := WalkPlan(node, 1000)
	found := false
	for _, i := range issues {
		if i.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected high cost error")
	}
}

func TestWalkPlanSmallTableOK(t *testing.T) {
	node := &PlanNode{NodeType: "Seq Scan", Relation: "config", Cost: 1.5, Rows: 5}
	issues := WalkPlan(node, 1000)
	if len(issues) != 0 {
		t.Errorf("small table seq scan should be OK, got %d issues", len(issues))
	}
}

func TestExtractColumn(t *testing.T) {
	cases := []struct{ in, want string }{
		{"(age > 25)", "age"},
		{"(email = 'test@x.com')", "email"},
		{"(price >= 100)", "price"},
		{"(status != 'active')", "status"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ExtractColumn(c.in); got != c.want {
			t.Errorf("ExtractColumn(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSuggestIndex(t *testing.T) {
	got := SuggestIndex("users", "(age > 25)")
	want := "CREATE INDEX CONCURRENTLY idx_users_age ON users (age);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	noFilter := SuggestIndex("orders", "")
	if noFilter != "CREATE INDEX CONCURRENTLY ON orders (...);" {
		t.Errorf("no-filter case wrong: %s", noFilter)
	}
	if s := SuggestIndex("", "(x > 1)"); s != "" {
		t.Errorf("empty table should return empty, got %s", s)
	}
}

func TestAnalyzeStatic(t *testing.T) {
	results := AnalyzeStatic([]string{
		"SELECT * FROM users",
		"SELECT id FROM users WHERE age > 25",
		"SELECT * FROM orders LIMIT 10",
	})
	if results[0].Pass {
		t.Error("SELECT * FROM users without WHERE should fail")
	}
	if len(results[0].Issues) < 2 {
		t.Error("expected warning + info for SELECT * without WHERE")
	}
	if !results[1].Pass {
		t.Error("query with WHERE should pass")
	}
	if !results[2].Pass {
		t.Error("query with LIMIT should pass")
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	code := Render([]Result{{Query: "SELECT 1", Pass: true}}, "json", &buf)
	if code != 0 {
		t.Error("expected exit 0 for passing query")
	}
	if buf.Len() == 0 {
		t.Error("expected JSON output")
	}
}

func TestRenderMarkdown(t *testing.T) {
	var buf bytes.Buffer
	Render([]Result{{Query: "SELECT * FROM t", Pass: false,
		Issues: []Issue{{"warning", "seq scan", "CREATE INDEX..."}}}}, "markdown", &buf)
	if !bytes.Contains(buf.Bytes(), []byte("PGLens")) {
		t.Error("expected PGLens header in markdown")
	}
}

func TestRenderTextExitCode(t *testing.T) {
	var buf bytes.Buffer
	code := Render([]Result{
		{Query: "SELECT 1", Pass: true},
		{Query: "SELECT * FROM big", Pass: false, Issues: []Issue{{"warning", "scan", ""}}},
	}, "text", &buf)
	if code != 1 {
		t.Error("expected exit 1 with failing query")
	}
	if !bytes.Contains(buf.Bytes(), []byte("FAIL")) {
		t.Error("expected FAIL in text output")
	}
}

func TestParseSQL(t *testing.T) {
	f, _ := os.CreateTemp("", "*.sql")
	f.WriteString("SELECT 1;\n-- comment\nSELECT 2;")
	f.Close()
	defer os.Remove(f.Name())
	queries, err := ParseSQL(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 {
		t.Errorf("got %d queries, want 2", len(queries))
	}
}
