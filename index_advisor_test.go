package main

import (
	"testing"
)

func TestSuggestIndexes(t *testing.T) {
	// Test case 1: Single column filter on Seq Scan
	t.Run("single_column_filter", func(t *testing.T) {
		plan := PlanNode{
			NodeType: "Seq Scan",
			Relation: "users",
			Filter:   "(age > 25)",
			Rows:     5000,
		}
		suggestions := SuggestIndexes(plan)
		if len(suggestions) != 1 {
			t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
		}
		s := suggestions[0]
		if s.Table != "users" {
			t.Errorf("table = %q, want %q", s.Table, "users")
		}
		if len(s.Columns) != 1 || s.Columns[0] != "age" {
			t.Errorf("columns = %v, want [age]", s.Columns)
		}
		if s.EstimatedRows != 5000 {
			t.Errorf("estimated_rows = %d, want 5000", s.EstimatedRows)
		}
		expectedDDL := "CREATE INDEX CONCURRENTLY idx_users_age ON users (age);"
		if s.DDL != expectedDDL {
			t.Errorf("ddl = %q, want %q", s.DDL, expectedDDL)
		}
	})

	// Test case 2: Multi-column AND filter
	t.Run("multi_column_AND_filter", func(t *testing.T) {
		plan := PlanNode{
			NodeType: "Seq Scan",
			Relation: "orders",
			Filter:   "(status = 'pending') AND (created_at > '2024-01-01')",
			Rows:     10000,
		}
		suggestions := SuggestIndexes(plan)
		if len(suggestions) != 1 {
			t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
		}
		s := suggestions[0]
		if s.Table != "orders" {
			t.Errorf("table = %q, want %q", s.Table, "orders")
		}
		if len(s.Columns) != 2 {
			t.Fatalf("expected 2 columns, got %d: %v", len(s.Columns), s.Columns)
		}
		if s.Columns[0] != "status" || s.Columns[1] != "created_at" {
			t.Errorf("columns = %v, want [status created_at]", s.Columns)
		}
		expectedDDL := "CREATE INDEX CONCURRENTLY idx_orders_status_created_at ON orders (status, created_at);"
		if s.DDL != expectedDDL {
			t.Errorf("ddl = %q, want %q", s.DDL, expectedDDL)
		}
		if s.EstimatedRows != 10000 {
			t.Errorf("estimated_rows = %d, want 10000", s.EstimatedRows)
		}
	})

	// Test case 3: Index Scan should NOT produce suggestions
	t.Run("index_scan_no_duplicate_suggestion", func(t *testing.T) {
		plan := PlanNode{
			NodeType: "Index Scan",
			Relation: "users",
			Filter:   "(age > 25)",
			Rows:     500,
		}
		suggestions := SuggestIndexes(plan)
		if len(suggestions) != 0 {
			t.Errorf("expected 0 suggestions for Index Scan, got %d", len(suggestions))
		}

		// Also test Index Only Scan
		plan2 := PlanNode{
			NodeType: "Index Only Scan",
			Relation: "users",
			Filter:   "(age > 25)",
			Rows:     500,
		}
		suggestions2 := SuggestIndexes(plan2)
		if len(suggestions2) != 0 {
			t.Errorf("expected 0 suggestions for Index Only Scan, got %d", len(suggestions2))
		}
	})

	// Test case 4: Nested subquery / join plans
	t.Run("nested_subquery_plans", func(t *testing.T) {
		plan := PlanNode{
			NodeType: "Hash Join",
			Rows:     1000,
			Children: []PlanNode{
				{
					NodeType: "Seq Scan",
					Relation: "orders",
					Filter:   "(total > 100)",
					Rows:     5000,
				},
				{
					NodeType: "Hash",
					Children: []PlanNode{
						{
							NodeType: "Seq Scan",
							Relation: "customers",
							Filter:   "(active = true)",
							Rows:     2000,
						},
					},
				},
			},
		}
		suggestions := SuggestIndexes(plan)
		if len(suggestions) != 2 {
			t.Fatalf("expected 2 suggestions from nested plan, got %d", len(suggestions))
		}
		// First suggestion: orders.total
		if suggestions[0].Table != "orders" {
			t.Errorf("first suggestion table = %q, want %q", suggestions[0].Table, "orders")
		}
		if len(suggestions[0].Columns) != 1 || suggestions[0].Columns[0] != "total" {
			t.Errorf("first suggestion columns = %v, want [total]", suggestions[0].Columns)
		}
		expectedDDL1 := "CREATE INDEX CONCURRENTLY idx_orders_total ON orders (total);"
		if suggestions[0].DDL != expectedDDL1 {
			t.Errorf("first DDL = %q, want %q", suggestions[0].DDL, expectedDDL1)
		}
		if suggestions[0].EstimatedRows != 5000 {
			t.Errorf("first estimated_rows = %d, want 5000", suggestions[0].EstimatedRows)
		}
		// Second suggestion: customers.active
		if suggestions[1].Table != "customers" {
			t.Errorf("second suggestion table = %q, want %q", suggestions[1].Table, "customers")
		}
		if len(suggestions[1].Columns) != 1 || suggestions[1].Columns[0] != "active" {
			t.Errorf("second suggestion columns = %v, want [active]", suggestions[1].Columns)
		}
		expectedDDL2 := "CREATE INDEX CONCURRENTLY idx_customers_active ON customers (active);"
		if suggestions[1].DDL != expectedDDL2 {
			t.Errorf("second DDL = %q, want %q", suggestions[1].DDL, expectedDDL2)
		}
		if suggestions[1].EstimatedRows != 2000 {
			t.Errorf("second estimated_rows = %d, want 2000", suggestions[1].EstimatedRows)
		}
	})

	// Test case 5: Seq Scan without Filter should NOT trigger suggestion
	t.Run("seq_scan_without_filter_no_suggestion", func(t *testing.T) {
		plan := PlanNode{
			NodeType: "Seq Scan",
			Relation: "config",
			Rows:     10,
		}
		suggestions := SuggestIndexes(plan)
		if len(suggestions) != 0 {
			t.Errorf("expected 0 suggestions for Seq Scan without filter, got %d", len(suggestions))
		}

		// Also test with empty string filter
		plan2 := PlanNode{
			NodeType: "Seq Scan",
			Relation: "settings",
			Filter:   "",
			Rows:     100,
		}
		suggestions2 := SuggestIndexes(plan2)
		if len(suggestions2) != 0 {
			t.Errorf("expected 0 suggestions for empty filter, got %d", len(suggestions2))
		}
	})
}

func TestExtractColumns(t *testing.T) {
	t.Run("single_column", func(t *testing.T) {
		cols := ExtractColumns("(age > 25)")
		if len(cols) != 1 || cols[0] != "age" {
			t.Errorf("got %v, want [age]", cols)
		}
	})

	t.Run("two_columns_AND", func(t *testing.T) {
		cols := ExtractColumns("(status = 'active') AND (created_at > '2024-01-01')")
		if len(cols) != 2 {
			t.Fatalf("got %d columns, want 2: %v", len(cols), cols)
		}
		if cols[0] != "status" || cols[1] != "created_at" {
			t.Errorf("got %v, want [status created_at]", cols)
		}
	})

	t.Run("three_columns_AND", func(t *testing.T) {
		cols := ExtractColumns("(region = 'us') AND (status = 'active') AND (score > 80)")
		if len(cols) != 3 {
			t.Fatalf("got %d columns, want 3: %v", len(cols), cols)
		}
		if cols[0] != "region" || cols[1] != "status" || cols[2] != "score" {
			t.Errorf("got %v, want [region status score]", cols)
		}
	})

	t.Run("empty_filter", func(t *testing.T) {
		cols := ExtractColumns("")
		if len(cols) != 0 {
			t.Errorf("got %v, want empty", cols)
		}
	})

	t.Run("dedup_same_column", func(t *testing.T) {
		cols := ExtractColumns("(age > 18) AND (age < 65)")
		if len(cols) != 1 || cols[0] != "age" {
			t.Errorf("got %v, want [age] (deduplicated)", cols)
		}
	})
}
