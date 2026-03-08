# Phase 27: Dashboard — Insights & Charts

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [Chart.js Integration](#3-chartjs-integration)
4. [Data API](#4-data-api)
5. [Category Breakdown](#5-category-breakdown)
6. [Spending Over Time](#6-spending-over-time)
7. [Balance Trends](#7-balance-trends)
8. [Monthly Comparison](#8-monthly-comparison)
9. [Dashboard Summary Cards](#9-dashboard-summary-cards)
10. [Dashboard UI](#10-dashboard-ui)
11. [Implementation Tasks](#11-implementation-tasks)
12. [Dependencies](#12-dependencies)

---

## 1. Overview

Phase 27 adds visual spending intelligence to the Breadbox admin dashboard. Users get interactive charts showing where their money goes (category breakdown), how spending changes over time, how account balances trend, and how this month compares to previous months. A net income/spending summary card on the main dashboard provides at-a-glance financial health. All charts use Chart.js via CDN with Alpine.js integration, consistent with the existing no-build-step approach.

---

## 2. Goals

- **Category visibility.** Instantly see which categories consume the most spending, with drill-down from primary to detailed categories.
- **Temporal awareness.** Understand spending patterns over weeks/months/years, filtered by category, account, or family member.
- **Balance tracking.** Visualize account balance trends over time to spot anomalies or seasonal patterns.
- **Month-over-month comparison.** Compare this month's spending to last month and to the rolling average, highlighting categories that are over/under their norm.
- **Dashboard at-a-glance.** A net income/spending summary card on the main dashboard so users see financial health without navigating to a sub-page.

---

## 3. Chart.js Integration

### 3.1 CDN Setup

Add Chart.js to `internal/templates/layout/base.html` alongside the existing Lucide and Alpine.js scripts:

```html
<script src="https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js"></script>
```

Pin to a specific minor version for stability (e.g., `chart.js@4.4.7`), consistent with how Lucide is pinned to `0.468.0`.

No additional plugins are needed for the initial implementation. The built-in bar, line, and doughnut chart types cover all Phase 27 requirements.

### 3.2 Alpine.js Integration Pattern

Charts are initialized inside Alpine.js `x-data` components. The pattern:

```html
<div x-data="categoryChart()" x-init="init()">
  <canvas x-ref="chart" class="w-full" style="max-height: 400px;"></canvas>
</div>

<script>
function categoryChart() {
  return {
    chart: null,
    async init() {
      const res = await fetch('/admin/api/insights/category-breakdown?...');
      const data = await res.json();
      this.chart = new Chart(this.$refs.chart, {
        type: 'doughnut',
        data: { /* ... */ },
        options: { /* ... */ }
      });
    },
    destroy() {
      if (this.chart) this.chart.destroy();
    }
  };
}
</script>
```

Use `x-effect` or `$watch` to re-fetch and update charts when filter controls change, calling `this.chart.data = newData; this.chart.update()` instead of destroying and re-creating.

### 3.3 Dark Mode Support

Chart.js reads CSS custom properties at render time. Configure charts to respect DaisyUI theme colors:

```javascript
function getChartColors() {
  const style = getComputedStyle(document.documentElement);
  return {
    text: style.getPropertyValue('--bc') ? `oklch(${style.getPropertyValue('--bc')})` : '#1f2937',
    grid: style.getPropertyValue('--b3') ? `oklch(${style.getPropertyValue('--b3')})` : '#e5e7eb',
    // Category palette — fixed set of distinguishable colors
    palette: [
      '#4ade80', '#60a5fa', '#f472b6', '#facc15', '#a78bfa',
      '#fb923c', '#34d399', '#f87171', '#38bdf8', '#c084fc',
      '#fbbf24', '#2dd4bf', '#e879f9', '#818cf8', '#fb7185',
    ],
  };
}
```

Set `Chart.defaults.color` and `Chart.defaults.borderColor` in a global init block so all charts inherit theme-aware defaults. Re-read colors on `prefers-color-scheme` media query change:

```javascript
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  // Update Chart.defaults and call .update() on active charts
});
```

### 3.4 Responsive Behavior

All `<canvas>` elements use Chart.js `responsive: true` (the default) with `maintainAspectRatio: false` and a container with a fixed `max-height` (e.g., `max-h-[400px]`). On mobile (`< lg`), charts stack vertically and expand to full width.

### 3.5 Inline Script Organization

Chart initialization scripts live in the page template files (not external JS files), consistent with existing Alpine.js patterns in the codebase. Each page template defines its Alpine.js component functions in a `<script>` block at the bottom of the `{{define "content"}}` block.

---

## 4. Data API

New JSON endpoints under `/admin/api/insights/` serve aggregated data to the frontend. These are admin-authenticated endpoints (session cookie, same as existing `/admin/api/` routes) — not public REST API endpoints. The service layer does the aggregation via dynamic SQL queries against the `transactions` and `accounts` tables.

### 4.1 Service Layer Functions

Add a new file `internal/service/insights.go` with the following functions:

#### `GetCategoryBreakdown`

```go
type CategoryBreakdownParams struct {
    StartDate *time.Time
    EndDate   *time.Time
    AccountID *string
    UserID    *string
}

type CategoryBreakdownItem struct {
    CategoryPrimary string  `json:"category_primary"`
    CategoryDetailed *string `json:"category_detailed,omitempty"`
    TotalAmount     float64 `json:"total_amount"`
    Count           int     `json:"count"`
    Percentage      float64 `json:"percentage"`
}

func (s *Service) GetCategoryBreakdown(ctx context.Context, params CategoryBreakdownParams) ([]CategoryBreakdownItem, error)
```

SQL pattern (dynamic query builder, same style as `ListTransactions`):

```sql
SELECT category_primary, SUM(amount) AS total_amount, COUNT(*) AS count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.deleted_at IS NULL
  AND t.pending = false
  AND t.amount > 0  -- debits only (positive = money out in Plaid convention)
  [AND t.date >= $N]
  [AND t.date < $N]
  [AND t.account_id = $N]
  [AND bc.user_id = $N]
GROUP BY category_primary
ORDER BY total_amount DESC
```

For detailed drill-down, accept an optional `Primary` parameter and group by `category_detailed` instead.

**Amount convention:** In the Plaid convention used by Breadbox, positive amounts are debits (money out/spending) and negative amounts are credits (money in/income). The category breakdown shows spending, so it filters for `amount > 0`. The "Uncategorized" bucket captures transactions where `category_primary IS NULL`.

#### `GetSpendingOverTime`

```go
type SpendingOverTimeParams struct {
    StartDate   *time.Time
    EndDate     *time.Time
    Granularity string // "daily", "weekly", "monthly"
    AccountID   *string
    UserID      *string
    Category    *string
}

type SpendingOverTimePoint struct {
    Period      string  `json:"period"`       // "2026-03", "2026-W10", "2026-03-08"
    TotalSpent  float64 `json:"total_spent"`  // sum of positive amounts (debits)
    TotalIncome float64 `json:"total_income"` // abs(sum of negative amounts) (credits)
    NetFlow     float64 `json:"net_flow"`     // total_income - total_spent
    Count       int     `json:"count"`
}

func (s *Service) GetSpendingOverTime(ctx context.Context, params SpendingOverTimeParams) ([]SpendingOverTimePoint, error)
```

SQL uses `date_trunc` for monthly/weekly grouping, or raw `date` for daily:

```sql
SELECT
    TO_CHAR(date_trunc('month', t.date), 'YYYY-MM') AS period,
    COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0) AS total_spent,
    COALESCE(SUM(CASE WHEN t.amount < 0 THEN ABS(t.amount) ELSE 0 END), 0) AS total_income,
    COUNT(*) AS count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.deleted_at IS NULL AND t.pending = false
  [AND ...]
GROUP BY period
ORDER BY period ASC
```

#### `GetBalanceTrend`

```go
type BalanceTrendParams struct {
    AccountID string
    StartDate *time.Time
    EndDate   *time.Time
}

type BalanceTrendPoint struct {
    Date           string   `json:"date"`
    BalanceCurrent *float64 `json:"balance_current"`
}

func (s *Service) GetBalanceTrend(ctx context.Context, params BalanceTrendParams) ([]BalanceTrendPoint, error)
```

Balance history is not stored per-day in the current schema — only the latest `balance_current` is kept per account. Two approaches (choose one during implementation):

**Option A — Derive from transactions (recommended for Phase 27).** Start from the current balance and work backwards by subtracting daily net transaction amounts. This gives an approximation:

```sql
WITH daily_net AS (
    SELECT date, SUM(amount) AS net
    FROM transactions
    WHERE account_id = $1 AND deleted_at IS NULL AND pending = false
    GROUP BY date
)
SELECT date, SUM(net) OVER (ORDER BY date) AS cumulative_net
FROM daily_net
ORDER BY date
```

Then compute `balance_on_date = current_balance - (total_net_since_date)`. This is approximate but requires no schema changes.

**Option B — Balance snapshots table (future).** A `balance_snapshots` table recorded at each sync would give exact history. This is a Phase 27+ enhancement and is out of scope for the initial implementation.

#### `GetMonthlyComparison`

```go
type MonthlyComparisonParams struct {
    AccountID *string
    UserID    *string
}

type MonthlyComparisonResult struct {
    CurrentMonth  MonthSummary            `json:"current_month"`
    PreviousMonth MonthSummary            `json:"previous_month"`
    Average       MonthSummary            `json:"average"`
    ByCategory    []CategoryMonthCompare  `json:"by_category"`
}

type MonthSummary struct {
    Period      string  `json:"period"`       // "2026-03"
    TotalSpent  float64 `json:"total_spent"`
    TotalIncome float64 `json:"total_income"`
    NetFlow     float64 `json:"net_flow"`
    TxCount     int     `json:"tx_count"`
}

type CategoryMonthCompare struct {
    Category      string  `json:"category"`
    CurrentAmount float64 `json:"current_amount"`
    PreviousAmount float64 `json:"previous_amount"`
    AverageAmount float64 `json:"average_amount"`
    ChangePercent *float64 `json:"change_percent,omitempty"` // vs previous month
}

func (s *Service) GetMonthlyComparison(ctx context.Context, params MonthlyComparisonParams) (*MonthlyComparisonResult, error)
```

Computes three month summaries (current, previous, trailing 6-month average) in a single query using conditional aggregation:

```sql
SELECT
    TO_CHAR(date_trunc('month', t.date), 'YYYY-MM') AS month,
    category_primary,
    SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END) AS spent,
    SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END) AS income,
    COUNT(*) AS tx_count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.deleted_at IS NULL AND t.pending = false
  AND t.date >= date_trunc('month', CURRENT_DATE) - INTERVAL '6 months'
GROUP BY month, category_primary
ORDER BY month, spent DESC
```

The service layer groups results in Go to produce the three summaries and per-category comparison.

#### `GetDashboardSummary`

```go
type DashboardSummary struct {
    CurrentMonthSpent  float64 `json:"current_month_spent"`
    CurrentMonthIncome float64 `json:"current_month_income"`
    CurrentMonthNet    float64 `json:"current_month_net"`
    PreviousMonthSpent float64 `json:"previous_month_spent"`
    SpendingChange     *float64 `json:"spending_change,omitempty"` // percentage
    TopCategory        *string `json:"top_category,omitempty"`
    TopCategoryAmount  float64 `json:"top_category_amount"`
}

func (s *Service) GetDashboardSummary(ctx context.Context) (*DashboardSummary, error)
```

A lightweight query for the main dashboard cards — current month spending/income totals and comparison to last month.

### 4.2 Admin API Endpoints

Add to `internal/admin/insights.go`:

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/admin/api/insights/category-breakdown` | `CategoryBreakdownHandler` | Category spending breakdown |
| `GET` | `/admin/api/insights/spending-over-time` | `SpendingOverTimeHandler` | Spending/income time series |
| `GET` | `/admin/api/insights/balance-trend` | `BalanceTrendHandler` | Balance trend for one account |
| `GET` | `/admin/api/insights/monthly-comparison` | `MonthlyComparisonHandler` | Month vs month comparison |
| `GET` | `/admin/api/insights/dashboard-summary` | `DashboardSummaryHandler` | Summary stats for dashboard cards |

All endpoints:
- Return JSON with `Content-Type: application/json`
- Accept query parameters for filters (e.g., `?start_date=2026-01-01&end_date=2026-03-01&account_id=...&user_id=...`)
- Require admin session authentication (inside the existing `/admin/api` route group)
- Return `{"error": {"code": "...", "message": "..."}}` on failure, consistent with the REST API envelope pattern

Query parameter formats:
- Dates: `YYYY-MM-DD` (same as transaction list filters)
- `granularity`: `daily`, `weekly`, `monthly` (default: `monthly`)
- `category`: primary category string filter
- `account_id`, `user_id`: UUID strings

### 4.3 Public REST API (Optional, Deferred)

The insights data could also be exposed under `/api/v1/insights/` with API key auth. This is deferred to a future phase — for now, only the admin dashboard consumes these endpoints.

---

## 5. Category Breakdown

### 5.1 Page Design

A new page at `/admin/insights/categories` accessible from the sidebar navigation under a new "Insights" section.

**Layout:**
- **Filter bar** at top: date range (start/end), family member dropdown, account dropdown. Uses `.bb-filter-bar` pattern.
- **Primary chart**: doughnut chart showing top-level category spending. Each slice is a `category_primary` value. "Uncategorized" shown as a gray slice if present.
- **Category table** below chart: sorted by amount descending, showing category name, total amount, transaction count, and percentage of total.
- **Drill-down**: Clicking a slice or table row filters to that primary category and shows a second bar chart of its detailed sub-categories.

### 5.2 Chart Configuration

```javascript
{
  type: 'doughnut',
  data: {
    labels: categories.map(c => c.category_primary || 'Uncategorized'),
    datasets: [{
      data: categories.map(c => c.total_amount),
      backgroundColor: palette.slice(0, categories.length),
    }]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { position: 'right', labels: { color: chartColors.text } },
      tooltip: {
        callbacks: {
          label: (ctx) => `${ctx.label}: $${ctx.parsed.toFixed(2)} (${percentages[ctx.dataIndex]}%)`
        }
      }
    }
  }
}
```

### 5.3 Drill-Down Behavior

When a primary category is selected (click on slice or table row):
1. Update the filter bar to show the selected category
2. Fetch detailed breakdown: `GET /admin/api/insights/category-breakdown?category=FOOD_AND_DRINK&start_date=...`
3. Replace the doughnut with a horizontal bar chart showing detailed sub-categories
4. A "Back to all categories" link resets the view

### 5.4 Empty State

When there are no transactions in the selected date range:
- Hide the chart
- Show an empty state card: "No spending data for this period. Adjust the date range or sync more transactions."
- Use the existing `card bg-base-200` + centered content pattern

---

## 6. Spending Over Time

### 6.1 Chart Design

A new page at `/admin/insights/spending` or a tab/section within the insights page.

**Chart type:** Stacked bar chart with two datasets: spending (positive, colored by theme `error` or a warm color) and income (negative direction or overlaid, colored by `success`). Optionally, a line overlay showing net flow.

**X-axis:** Time periods. **Y-axis:** Dollar amounts.

### 6.2 Time Granularity Options

Three granularity options via a segmented control (DaisyUI `btn-group`):
- **Monthly** (default): Last 12 months. Good for trend analysis.
- **Weekly**: Last 12 weeks. Good for recent pattern detection.
- **Daily**: Last 30 days. Good for detailed recent view.

The selected granularity is sent as `?granularity=monthly` to the API. Default date ranges per granularity:
- Monthly: trailing 12 months from current date
- Weekly: trailing 12 weeks from current date
- Daily: trailing 30 days from current date

Users can override with explicit date range in the filter bar.

### 6.3 Filter Controls

Same filter bar as category breakdown: date range, family member, account. Additional filter:
- **Category**: dropdown to filter spending to a single primary category.

All filters update the chart via `fetch()` without page reload.

### 6.4 Chart Configuration

```javascript
{
  type: 'bar',
  data: {
    labels: points.map(p => p.period),
    datasets: [
      {
        label: 'Spending',
        data: points.map(p => p.total_spent),
        backgroundColor: '#f87171',
      },
      {
        label: 'Income',
        data: points.map(p => p.total_income),
        backgroundColor: '#4ade80',
      }
    ]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: { stacked: false, ticks: { color: chartColors.text }, grid: { color: chartColors.grid } },
      y: { ticks: { color: chartColors.text, callback: v => '$' + v.toLocaleString() }, grid: { color: chartColors.grid } }
    },
    plugins: {
      legend: { labels: { color: chartColors.text } },
      tooltip: {
        callbacks: {
          label: (ctx) => `${ctx.dataset.label}: $${ctx.parsed.y.toFixed(2)}`
        }
      }
    }
  }
}
```

---

## 7. Balance Trends

### 7.1 Per-Account Chart

Accessible from the account detail page (`/admin/accounts/{id}`) as an embedded chart section, and also from the insights page with an account selector.

**Chart type:** Line chart showing estimated daily balance over time.

**Data source:** `GetBalanceTrend` service function (derived from transactions, working backwards from current balance).

### 7.2 Time Range Selector

A segmented control with preset ranges:
- **1 month** (default)
- **3 months**
- **6 months**
- **1 year**
- **All time**

The selector adjusts `start_date` sent to the API. No end date needed (always "up to today").

### 7.3 Chart Configuration

```javascript
{
  type: 'line',
  data: {
    labels: points.map(p => p.date),
    datasets: [{
      label: accountName,
      data: points.map(p => p.balance_current),
      borderColor: '#60a5fa',
      backgroundColor: 'rgba(96, 165, 250, 0.1)',
      fill: true,
      tension: 0.3,
      pointRadius: 0,
      pointHitRadius: 10,
    }]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: { type: 'category', ticks: { color: chartColors.text, maxTicksLimit: 12 }, grid: { display: false } },
      y: { ticks: { color: chartColors.text, callback: v => '$' + v.toLocaleString() }, grid: { color: chartColors.grid } }
    },
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (ctx) => `Balance: $${ctx.parsed.y.toFixed(2)}`
        }
      }
    }
  }
}
```

### 7.4 Multi-Account View (Optional)

On the insights page, allow selecting multiple accounts to overlay their balance lines on the same chart. Each account gets a distinct color from the palette. Limited to 5 accounts max to avoid visual clutter.

---

## 8. Monthly Comparison

### 8.1 Comparison Logic

Compare three periods:
1. **Current month:** From 1st of current month to today.
2. **Previous month:** Full previous calendar month.
3. **Average:** Trailing 6-month average (excluding current month). If fewer than 2 months of data exist, omit the average.

Note: Current month is partial, so comparisons should be presented with appropriate context (e.g., "15 days into the month" or pro-rated projections).

### 8.2 Display Format

**Summary row** at top (three DaisyUI `stat` cards side by side):

| This Month (so far) | Last Month | 6-Month Average |
|---|---|---|
| $2,340.00 spent | $3,120.00 spent | $2,890.00/mo |
| $4,500.00 earned | $4,500.00 earned | $4,200.00/mo |
| +$2,160.00 net | +$1,380.00 net | +$1,310.00/mo |

**Category comparison table** below:

| Category | This Month | Last Month | Average | Change |
|---|---|---|---|---|
| Food & Drink | $450 | $520 | $490 | -13.5% |
| Transportation | $280 | $180 | $210 | +55.6% |
| ... | ... | ... | ... | ... |

Change percentage is current vs previous month. Color-coded: green (badge-success) if spending decreased, red (badge-error) if increased (lower spending = good).

### 8.3 Projected Spending

For the current (partial) month, calculate a projection:

```
projected = (current_spent / days_elapsed) * days_in_month
```

Show as a secondary line in the summary: "On track for ~$3,510 this month."

### 8.4 Filter Controls

Family member and account filters available (same as other insights views).

---

## 9. Dashboard Summary Cards

### 9.1 Net Income/Spending Cards

Add two new stat cards to the existing stats row on the main dashboard (`/admin/`), positioned after the existing four stats (Accounts Connected, Transactions Synced, Last Sync, Needs Attention):

**Card 1: This Month's Spending**
- `stat-title`: "Spent This Month"
- `stat-value`: "$2,340.00" (formatted with commas, 2 decimal places)
- `stat-desc`: "+12% vs last month" or "-8% vs last month" with appropriate color

**Card 2: Net Cash Flow**
- `stat-title`: "Net This Month"
- `stat-value`: "+$2,160.00" (green if positive, red if negative)
- `stat-desc`: "Income: $4,500 | Spending: $2,340"

### 9.2 Data Loading

The dashboard handler (`DashboardHandler` in `internal/admin/dashboard.go`) calls `svc.GetDashboardSummary(ctx)` and passes the result to the template data map. This is a synchronous server-side call (not async JS fetch), consistent with how the dashboard currently loads stats.

### 9.3 Handling No Data

If there are no transactions in the current month, show the cards with "$0.00" values and stat-desc "No transactions this month." Do not hide the cards — consistent presence helps users learn the layout.

### 9.4 Stats Row Layout

The existing stats row uses `stats stats-vertical lg:stats-horizontal`. Adding two more cards brings the total to six. On desktop (`lg+`), this may be too wide. Options:
- Wrap to two rows: use a grid `grid grid-cols-2 lg:grid-cols-3 gap-4` instead of the `stats` component for the full set.
- Or keep the `stats` component but accept horizontal scrolling at some breakpoints.

**Recommendation:** Split into two rows. Top row: Accounts, Transactions, Last Sync, Needs Attention (existing). Bottom row: Spent This Month, Net This Month (new). This keeps the existing layout stable and groups financial insights together.

---

## 10. Dashboard UI

### 10.1 Navigation

Add a new nav section to the sidebar in `internal/templates/partials/nav.html`:

```html
<li class="menu-title">Insights</li>
<li><a href="/admin/insights"><i data-lucide="bar-chart-3"></i> Spending</a></li>
```

A single "Insights" page with tabs or sections is preferable to multiple nav items, to keep the sidebar lean. The insights page contains:
1. Category breakdown (default view)
2. Spending over time
3. Monthly comparison

Balance trend lives on the account detail page (contextually relevant) and can optionally be linked from insights.

### 10.2 Insights Page Layout

`/admin/insights` — a new page template `internal/templates/pages/insights.html`.

**Tab navigation** using DaisyUI `tabs tabs-bordered`:

```html
<div class="tabs tabs-bordered mb-6" x-data="{ tab: 'categories' }">
  <a class="tab" :class="{ 'tab-active': tab === 'categories' }" @click="tab = 'categories'">Categories</a>
  <a class="tab" :class="{ 'tab-active': tab === 'spending' }" @click="tab = 'spending'">Spending Over Time</a>
  <a class="tab" :class="{ 'tab-active': tab === 'comparison' }" @click="tab = 'comparison'">Monthly Comparison</a>
</div>
```

Each tab panel is shown/hidden with Alpine.js `x-show`. Chart data is fetched lazily on first tab activation to avoid unnecessary API calls.

### 10.3 Responsive Design

- **Desktop (`lg+`):** Charts render at full width within `max-w-5xl` content area. Category breakdown shows chart + table side by side (2-column grid). Filter bar spans full width.
- **Tablet (`md`):** Charts stack vertically. Category chart above table.
- **Mobile (`sm`):** Full-width stacked layout. Filter controls wrap vertically. Doughnut legend moves below chart (`position: 'bottom'`). Stat comparison cards stack 1-per-row.

### 10.4 Page Routes

Add to `internal/admin/router.go` inside the authenticated admin route group:

```go
r.Get("/insights", InsightsPageHandler(a, tr))
```

Add to `internal/admin/router.go` inside the admin API route group:

```go
r.Get("/insights/category-breakdown", CategoryBreakdownHandler(svc))
r.Get("/insights/spending-over-time", SpendingOverTimeHandler(svc))
r.Get("/insights/balance-trend", BalanceTrendHandler(svc))
r.Get("/insights/monthly-comparison", MonthlyComparisonHandler(svc))
r.Get("/insights/dashboard-summary", DashboardSummaryHandler(svc))
```

### 10.5 Account Detail Enhancement

Add a balance trend chart section to the existing account detail page (`internal/templates/pages/account_detail.html`). This is embedded directly in the page, not on the insights page.

### 10.6 CSS Additions

Add to `input.css` inside `@layer components`:

```css
.bb-chart-container {
  @apply relative w-full;
  max-height: 400px;
}

.bb-chart-container canvas {
  @apply w-full;
}

.bb-insight-grid {
  @apply grid grid-cols-1 lg:grid-cols-2 gap-6;
}
```

---

## 11. Implementation Tasks

Ordered by dependency. Each task references the specific files to create or modify.

### 11.1 Service Layer — Insight Aggregation Functions

- Create `internal/service/insights.go`
- Implement `GetCategoryBreakdown`, `GetSpendingOverTime`, `GetBalanceTrend`, `GetMonthlyComparison`, `GetDashboardSummary`
- Add corresponding types to `internal/service/types.go` or inline in `insights.go`
- Follow existing dynamic query builder pattern from `internal/service/transactions.go`

### 11.2 Admin API — Insights Endpoints

- Create `internal/admin/insights.go`
- Implement `CategoryBreakdownHandler`, `SpendingOverTimeHandler`, `BalanceTrendHandler`, `MonthlyComparisonHandler`, `DashboardSummaryHandler`
- Register routes in `internal/admin/router.go`

### 11.3 Chart.js CDN Integration

- Add Chart.js `<script>` tag to `internal/templates/layout/base.html`
- Add global Chart.js defaults script block (dark mode colors, font)

### 11.4 Dashboard Summary Cards

- Modify `internal/admin/dashboard.go`: add `GetDashboardSummary` call
- Modify `internal/templates/pages/dashboard.html`: add summary stat cards row
- No new template file needed — extends existing dashboard

### 11.5 Insights Page — Template & Handler

- Create `internal/templates/pages/insights.html`
- Create `InsightsPageHandler` in `internal/admin/insights.go`
- Register base page in `internal/admin/templates.go` (add to `basePages` array)
- Add navigation item in `internal/templates/partials/nav.html`

### 11.6 Category Breakdown Tab

- Implement doughnut chart + category table in insights template
- Alpine.js component for fetch + render + drill-down
- Filter bar: date range, account, user

### 11.7 Spending Over Time Tab

- Implement bar chart in insights template
- Alpine.js component with granularity selector
- Filter bar: date range, account, user, category

### 11.8 Monthly Comparison Tab

- Implement stat cards + comparison table in insights template
- Alpine.js component for data fetch and display

### 11.9 Balance Trend — Account Detail

- Add balance chart section to `internal/templates/pages/account_detail.html`
- Alpine.js component with time range selector
- Uses existing `BalanceTrendHandler` endpoint

### 11.10 CSS Updates

- Add `.bb-chart-container` and `.bb-insight-grid` to `input.css`
- Run `make css` to regenerate `static/css/styles.css`

### 11.11 Sidebar Navigation

- Add "Insights" section to `internal/templates/partials/nav.html`
- Update `CurrentPage` handling in templates for active state

### Task Dependencies

```
11.1 (service layer) ──> 11.2 (API endpoints) ──> 11.4 (dashboard cards)
                                                ──> 11.5 (insights page) ──> 11.6 (categories tab)
                                                                          ──> 11.7 (spending tab)
                                                                          ──> 11.8 (comparison tab)
                                                                          ──> 11.9 (balance trend)
11.3 (Chart.js CDN) ──> 11.6, 11.7, 11.8, 11.9
11.10 (CSS) ── independent
11.11 (nav) ── independent
```

---

## 12. Dependencies

### 12.1 Phase 20B — Category System

Phase 27 works with the current raw `category_primary` / `category_detailed` strings on transactions. When Phase 20B (Category Mapping) is complete, the insights queries should be updated to:

- Join on the `categories` table instead of grouping by raw strings
- Use `categories.display_name` for chart labels instead of raw `FOOD_AND_DRINK`
- Use `categories.color` for chart slice colors instead of the hardcoded palette
- Use `categories.icon` for category table rows
- Respect `categories.hidden` — exclude hidden categories from breakdown charts
- Use `categories.slug` as the drill-down key instead of raw category strings

**Migration path:** The service layer functions should be designed with this in mind. Use the raw strings for now, but structure the response types so they can be enriched with `display_name`, `color`, `icon` fields later without breaking the frontend contract. The `CategoryBreakdownItem` struct includes a `category_primary` field that maps 1:1 to a future `slug` field.

### 12.2 No Schema Changes

Phase 27 requires no database migrations. All data comes from existing `transactions` and `accounts` tables. The `GetBalanceTrend` function derives balance history from transactions mathematically.

### 12.3 External Dependencies

- **Chart.js v4** via CDN — no build step, no npm. MIT license.
- No other new dependencies. Uses existing Alpine.js, DaisyUI, Tailwind, Lucide.

### 12.4 Currency Handling

All amount aggregations must respect `iso_currency_code`. The initial implementation assumes a single currency (most common for family use). Charts should display the currency symbol from the majority currency in the dataset. If mixed currencies are detected, show a warning: "Multiple currencies detected. Totals may not be accurate."

Future enhancement: group-by-currency aggregation and currency conversion are out of scope for Phase 27.
