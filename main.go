package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

type Config struct {
	DSN, File, Format string
	MaxCost           float64
}

func main() {
	c := Config{}
	flag.StringVar(&c.DSN, "dsn", os.Getenv("PGLENS_DSN"), "PostgreSQL connection string")
	flag.StringVar(&c.File, "file", "", "SQL file to analyze")
	flag.Float64Var(&c.MaxCost, "max-cost", 1000, "Max allowed query cost")
	flag.StringVar(&c.Format, "format", "text", "Output: text|json|markdown")
	flag.Parse()
	if c.File == "" {
		fmt.Fprintln(os.Stderr, "Usage: pglens -file queries.sql [-dsn postgres://...]")
		os.Exit(1)
	}
	queries, err := ParseSQL(c.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	var results []Result
	if c.DSN != "" {
		db, err := sql.Open("postgres", c.DSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "DB error: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		results = AnalyzeLive(db, queries, c.MaxCost)
	} else {
		results = AnalyzeStatic(queries)
	}
	os.Exit(Render(results, c.Format, os.Stdout))
}

func ParseSQL(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, q := range strings.Split(string(data), ";") {
		if q = strings.TrimSpace(q); q != "" && !strings.HasPrefix(q, "--") {
			out = append(out, q)
		}
	}
	return out, nil
}

func Render(results []Result, format string, w io.Writer) int {
	code := 0
	for _, r := range results {
		if !r.Pass {
			code = 1
		}
	}
	switch format {
	case "json":
		json.NewEncoder(w).Encode(results)
	case "markdown":
		fmt.Fprintln(w, "## \U0001F50D PGLens Analysis Report\n")
		for i, r := range results {
			icon := "\u2705"
			if !r.Pass {
				icon = "\u274C"
			}
			fmt.Fprintf(w, "### %s Query %d\n```sql\n%s\n```\n", icon, i+1, trunc(r.Query, 200))
			if r.Cost > 0 {
				fmt.Fprintf(w, "**Cost:** %.1f\n\n", r.Cost)
			}
			for _, is := range r.Issues {
				fmt.Fprintf(w, "- **%s**: %s\n", is.Severity, is.Message)
				if is.Fix != "" {
					fmt.Fprintf(w, "  ```sql\n  %s\n  ```\n", is.Fix)
				}
			}
			fmt.Fprintln(w)
		}
	default:
		for i, r := range results {
			st := "PASS"
			if !r.Pass {
				st = "FAIL"
			}
			fmt.Fprintf(w, "[%s] Q%d: %s\n", st, i+1, trunc(r.Query, 80))
			for _, is := range r.Issues {
				fmt.Fprintf(w, "  %s: %s\n", is.Severity, is.Message)
				if is.Fix != "" {
					fmt.Fprintf(w, "  fix: %s\n", is.Fix)
				}
			}
		}
	}
	return code
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
