package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type PlanNode struct {
	NodeType string     `json:"Node Type"`
	Relation string     `json:"Relation Name"`
	Cost     float64    `json:"Total Cost"`
	Rows     int64      `json:"Plan Rows"`
	Filter   string     `json:"Filter"`
	Children []PlanNode `json:"Plans"`
}

type Issue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
}

type Result struct {
	Query  string  `json:"query"`
	Cost   float64 `json:"cost"`
	Issues []Issue `json:"issues"`
	Pass   bool    `json:"pass"`
}

func AnalyzeLive(db *sql.DB, queries []string, maxCost float64) []Result {
	var results []Result
	for _, q := range queries {
		r := Result{Query: q, Pass: true}
		var raw string
		if err := db.QueryRow("EXPLAIN (FORMAT JSON) " + q).Scan(&raw); err != nil {
			r.Issues = append(r.Issues, Issue{"error", fmt.Sprintf("EXPLAIN failed: %v", err), ""})
			r.Pass = false
			results = append(results, r)
			continue
		}
		var plans []struct{ Plan PlanNode }
		if err := json.Unmarshal([]byte(raw), &plans); err == nil && len(plans) > 0 {
			r.Cost = plans[0].Plan.Cost
			r.Issues = WalkPlan(&plans[0].Plan, maxCost)
			r.Pass = !hasFails(r.Issues)
		}
		results = append(results, r)
	}
	return results
}

func WalkPlan(n *PlanNode, maxCost float64) []Issue {
	var issues []Issue
	if n.NodeType == "Seq Scan" && n.Rows > 100 {
		issues = append(issues, Issue{
			Severity: "warning",
			Message:  fmt.Sprintf("Seq Scan on '%s' (~%d rows)", n.Relation, n.Rows),
			Fix:      SuggestIndex(n.Relation, n.Filter),
		})
	}
	if n.Cost > maxCost {
		issues = append(issues, Issue{
			Severity: "error",
			Message:  fmt.Sprintf("Cost %.0f exceeds threshold %.0f", n.Cost, maxCost),
		})
	}
	for i := range n.Children {
		issues = append(issues, WalkPlan(&n.Children[i], maxCost)...)
	}
	return issues
}

func SuggestIndex(table, filter string) string {
	if table == "" {
		return ""
	}
	col := ExtractColumn(filter)
	if col == "" {
		return fmt.Sprintf("CREATE INDEX CONCURRENTLY ON %s (...);", table)
	}
	return fmt.Sprintf("CREATE INDEX CONCURRENTLY idx_%s_%s ON %s (%s);", table, col, table, col)
}

func ExtractColumn(filter string) string {
	if filter == "" {
		return ""
	}
	f := strings.Trim(filter, "()")
	for _, op := range []string{">=", "<=", "!=", "<>", "=", ">", "<"} {
		if parts := strings.SplitN(f, op, 2); len(parts) == 2 {
			return strings.TrimSpace(parts[0])
		}
	}
	return ""
}

func AnalyzeStatic(queries []string) []Result {
	var results []Result
	for _, q := range queries {
		r := Result{Query: q, Pass: true}
		u := strings.ToUpper(q)
		if strings.Contains(u, "SELECT") && !strings.Contains(u, "WHERE") && !strings.Contains(u, "LIMIT") {
			r.Issues = append(r.Issues, Issue{"warning", "SELECT without WHERE/LIMIT — potential full table scan", ""})
			r.Pass = false
		}
		if strings.Contains(u, "SELECT *") {
			r.Issues = append(r.Issues, Issue{"info", "SELECT * — consider selecting specific columns", ""})
		}
		results = append(results, r)
	}
	return results
}

func hasFails(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "error" || i.Severity == "warning" {
			return true
		}
	}
	return false
}
