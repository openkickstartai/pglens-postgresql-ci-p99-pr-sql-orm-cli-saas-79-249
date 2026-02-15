package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ExpectedResult defines the sidecar .expected.json schema.
type ExpectedResult struct {
	MaxCost        float64         `json:"max_cost"`
	ExpectedIssues []ExpectedIssue `json:"expected_issues"`
	ShouldPass     bool            `json:"should_pass"`
}

// ExpectedIssue describes a single expected rule trigger.
type ExpectedIssue struct {
	Severity        string `json:"severity"`
	MessageContains string `json:"message_contains"`
}

// TestIntegration_Fixtures loads every EXPLAIN JSON fixture in testdata/,
// runs WalkPlan, and asserts against the sidecar .expected.json.
func TestIntegration_Fixtures(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to read testdata/ directory: %v", err)
	}

	// Collect plan fixture files (skip .expected.json sidecars)
	var planFiles []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".expected.json") {
			planFiles = append(planFiles, filepath.Join("testdata", name))
		}
	}

	if len(planFiles) < 6 {
		t.Fatalf("expected at least 6 fixture files in testdata/, got %d", len(planFiles))
	}

	for _, fixture := range planFiles {
		baseName := filepath.Base(fixture)
		t.Run(baseName, func(t *testing.T) {
			// ---- (a) Load fixture — must not panic ----
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("cannot read fixture %s: %v", baseName, err)
			}

			// Validate it is legal JSON
			if !json.Valid(data) {
				t.Fatalf("fixture %s is not valid JSON", baseName)
			}

			// Parse as EXPLAIN (FORMAT JSON) output
			var plans []struct {
				Plan PlanNode `json:"Plan"`
			}
			if err := json.Unmarshal(data, &plans); err != nil {
				t.Fatalf("failed to unmarshal EXPLAIN JSON in %s: %v", baseName, err)
			}
			if len(plans) == 0 {
				t.Fatalf("empty plan array in %s", baseName)
			}

			// ---- Load sidecar expected file ----
			expectedPath := strings.TrimSuffix(fixture, ".json") + ".expected.json"
			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("missing sidecar %s: %v", filepath.Base(expectedPath), err)
			}

			var expected ExpectedResult
			if err := json.Unmarshal(expectedData, &expected); err != nil {
				t.Fatalf("invalid expected JSON in %s: %v", filepath.Base(expectedPath), err)
			}

			maxCost := expected.MaxCost
			if maxCost == 0 {
				maxCost = 1000
			}

			// ---- Run analyzer — must not panic ----
			issues := WalkPlan(&plans[0].Plan, maxCost)

			// ---- (b) Assert pass / fail ----
			pass := !hasFails(issues)
			if pass != expected.ShouldPass {
				t.Errorf("pass=%v, want %v\n  issues: %+v", pass, expected.ShouldPass, issues)
			}

			// ---- Assert expected rules were triggered ----
			for _, ei := range expected.ExpectedIssues {
				found := false
				for _, issue := range issues {
					if issue.Severity == ei.Severity && strings.Contains(issue.Message, ei.MessageContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue (severity=%q, contains=%q) not found.\n  actual issues: %+v",
						ei.Severity, ei.MessageContains, issues)
				}
			}

			// ---- Log all detected issues for -v output ----
			for _, issue := range issues {
				t.Logf("  [%s] %s (fix: %s)", issue.Severity, issue.Message, issue.Fix)
			}
		})
	}
}

// TestIntegration_FixtureCount ensures we have the required number of fixtures.
func TestIntegration_FixtureCount(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("testdata/ directory missing: %v", err)
	}

	planCount := 0
	expectedCount := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".expected.json") {
			expectedCount++
		} else if strings.HasSuffix(name, ".json") {
			planCount++
		}
	}

	if planCount < 6 {
		t.Errorf("need at least 6 plan fixtures, got %d", planCount)
	}
	if expectedCount < 6 {
		t.Errorf("need at least 6 expected sidecars, got %d", expectedCount)
	}
	if planCount != expectedCount {
		t.Errorf("plan fixtures (%d) and expected sidecars (%d) count mismatch", planCount, expectedCount)
	}
}

// TestIntegration_IndexScanNoIssues specifically validates that a clean
// index scan produces zero issues.
func TestIntegration_IndexScanNoIssues(t *testing.T) {
	data, err := os.ReadFile("testdata/index_scan_optimal.json")
	if err != nil {
		t.Skip("index_scan_optimal.json not found")
	}

	var plans []struct {
		Plan PlanNode `json:"Plan"`
	}
	if err := json.Unmarshal(data, &plans); err != nil {
		t.Fatal(err)
	}

	issues := WalkPlan(&plans[0].Plan, 1000)
	if len(issues) != 0 {
		t.Errorf("index scan should produce 0 issues, got %d: %+v", len(issues), issues)
	}
}

// TestIntegration_SeqScanHasFix ensures seq scan issues include an index suggestion.
func TestIntegration_SeqScanHasFix(t *testing.T) {
	data, err := os.ReadFile("testdata/seq_scan_large_table.json")
	if err != nil {
		t.Skip("seq_scan_large_table.json not found")
	}

	var plans []struct {
		Plan PlanNode `json:"Plan"`
	}
	if err := json.Unmarshal(data, &plans); err != nil {
		t.Fatal(err)
	}

	issues := WalkPlan(&plans[0].Plan, 1000)
	foundFix := false
	for _, issue := range issues {
		if issue.Severity == "warning" && strings.Contains(issue.Message, "Seq Scan") {
			if issue.Fix != "" && strings.Contains(issue.Fix, "CREATE INDEX") {
				foundFix = true
			}
		}
	}
	if !foundFix {
		t.Error("seq scan warning should include CREATE INDEX suggestion in Fix field")
	}
}

// TestIntegration_NestedChildrenTraversed verifies WalkPlan recurses into children.
func TestIntegration_NestedChildrenTraversed(t *testing.T) {
	data, err := os.ReadFile("testdata/nested_loop_no_index.json")
	if err != nil {
		t.Skip("nested_loop_no_index.json not found")
	}

	var plans []struct {
		Plan PlanNode `json:"Plan"`
	}
	if err := json.Unmarshal(data, &plans); err != nil {
		t.Fatal(err)
	}

	issues := WalkPlan(&plans[0].Plan, 1000)

	// Should find seq scan warnings for BOTH child tables
	ordersFound := false
	lineItemsFound := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "orders") {
			ordersFound = true
		}
		if strings.Contains(issue.Message, "line_items") {
			lineItemsFound = true
		}
	}
	if !ordersFound {
		t.Error("expected seq scan warning for 'orders' child node")
	}
	if !lineItemsFound {
		t.Error("expected seq scan warning for 'line_items' child node")
	}
}
