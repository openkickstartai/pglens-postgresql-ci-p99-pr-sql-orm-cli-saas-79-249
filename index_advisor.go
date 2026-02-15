package main

import (
	"fmt"
	"regexp"
	"strings"
)

// IndexSuggestion represents a suggested index to improve query performance.
type IndexSuggestion struct {
	Table         string   `json:"table"`
	Columns       []string `json:"columns"`
	EstimatedRows int64    `json:"estimated_rows"`
	DDL           string   `json:"ddl"`
}

// SuggestIndexes walks an EXPLAIN plan tree and returns CREATE INDEX suggestions
// for every Seq Scan node that includes a Filter condition.
// It skips Index Scan, Index Only Scan, and other non-sequential access methods.
// It recurses into child nodes to handle subqueries and nested joins.
func SuggestIndexes(plan PlanNode) []IndexSuggestion {
	var suggestions []IndexSuggestion
	collectIndexSuggestions(&plan, &suggestions)
	return suggestions
}

func collectIndexSuggestions(n *PlanNode, suggestions *[]IndexSuggestion) {
	if n.NodeType == "Seq Scan" && n.Filter != "" {
		cols := ExtractColumns(n.Filter)
		if len(cols) > 0 {
			ddl := fmt.Sprintf("CREATE INDEX CONCURRENTLY idx_%s_%s ON %s (%s);",
				n.Relation,
				strings.Join(cols, "_"),
				n.Relation,
				strings.Join(cols, ", "),
			)
			*suggestions = append(*suggestions, IndexSuggestion{
				Table:         n.Relation,
				Columns:       cols,
				EstimatedRows: n.Rows,
				DDL:           ddl,
			})
		}
	}
	for i := range n.Children {
		collectIndexSuggestions(&n.Children[i], suggestions)
	}
}

// ExtractColumns parses a PostgreSQL EXPLAIN filter string and returns
// all referenced column names. Handles AND-combined conditions such as
// "(status = 'pending') AND (created_at > '2024-01-01')".
func ExtractColumns(filter string) []string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}

	// Split on AND keyword (case-insensitive, word boundary)
	re := regexp.MustCompile(`(?i)\bAND\b`)
	parts := re.Split(filter, -1)

	var cols []string
	seen := make(map[string]bool)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Normalize: strip any outer parens, then re-wrap so ExtractColumn
		// receives the format it expects: "(col op val)"
		part = strings.TrimPrefix(part, "(")
		part = strings.TrimSuffix(part, ")")
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		col := ExtractColumn("(" + part + ")")
		if col != "" && !seen[col] {
			cols = append(cols, col)
			seen[col] = true
		}
	}
	return cols
}
