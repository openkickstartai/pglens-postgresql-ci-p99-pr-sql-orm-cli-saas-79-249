# 🔍 PGLens

PostgreSQL CI-level query performance gate & index advisor.
Catch slow queries, missing indexes, and full table scans **in PRs** — before they wreck production p99.

## 🚀 Quick Start

```bash
go install github.com/pglens/pglens@latest

# Static analysis (no DB needed)
pglens -file queries.sql

# Live EXPLAIN analysis
pglens -file queries.sql -dsn "postgres://user:pass@localhost/mydb"

# Markdown for PR comments / GitHub Step Summary
pglens -file queries.sql -format markdown

# Set cost threshold (default 1000)
pglens -file queries.sql -dsn "$PG_DSN" -max-cost 500
```

## GitHub Actions

```yaml
- name: PGLens Gate
  run: pglens -file queries.sql -dsn ${{ secrets.PG_DSN }} -format markdown >> $GITHUB_STEP_SUMMARY
```

## ⚙️ What It Detects

| Check | How |
|-------|-----|
| Sequential scans on large tables | EXPLAIN plan walk, flags `Seq Scan` with >100 rows |
| Excessive query cost | Fails when `Total Cost` exceeds `-max-cost` threshold |
| Missing indexes | Auto-suggests `CREATE INDEX CONCURRENTLY` from scan filters |
| `SELECT *` anti-pattern | Static analysis, no DB required |
| Unbounded queries | `SELECT` without `WHERE` or `LIMIT` |

## 💰 Pricing

| Feature | Free (CLI) | Pro $79/mo | Enterprise $249/mo |
|---------|-----------|-----------|-------------------|
| Static SQL analysis | ✅ | ✅ | ✅ |
| EXPLAIN plan analysis | 5 queries | Unlimited | Unlimited |
| Seq scan + index advisor | ✅ | ✅ | ✅ |
| PR comment bot | ❌ | ✅ | ✅ |
| Baseline regression diff | ❌ | ✅ | ✅ |
| ORM query extraction | ❌ | ✅ | ✅ |
| Slack / PagerDuty alerts | ❌ | ❌ | ✅ |
| Historical trending | ❌ | ❌ | ✅ |
| SOC2 compliance reports | ❌ | ❌ | ✅ |
| Support | Community | Email | Dedicated SLA |

## 📊 Why Pay?

One slow-query incident costs **$5k–$50k** (downtime + engineers + customer churn).

| Without PGLens | With PGLens |
|---------------|-------------|
| Slow query hits prod at 3 AM | Caught in PR review |
| 4-hour incident, 3 engineers | 5-minute fix before merge |
| ~$15k avg cost per incident | $79/month prevention |

**ROI: $948/year vs $15k per incident = 15× return.**

## Architecture

```
SQL file → Parser → Static Analysis ──→ Issues → Renderer → exit code
                  └→ EXPLAIN (JSON) ──→ Plan Walker → Index Advisor ↗
```

## License

BSL 1.1 — Free for teams ≤ 5. Contact sales@pglens.dev for Pro/Enterprise.
