# QA — 3.0.0 metrics and charts

Date: 2026-11-23. Data: a copy of local data (last prod sync 9 Nov, plus the copy's own bank syncs). Salary rule: "Month it arrives" from 1 Nov 2026, payday by the 10th.

## Regression (repeatable)

| Run | Checks | Failed |
|---|---|---|
| `regress_raw.py`, "Month it arrives" | 1,687 | 0 |
| `regress_raw.py`, "Month before, by the 10th" | 1,687 | 0 |
| `regress_raw.py` against 2.87.0 (proves it catches bugs) | 1,687 | **46**: Ledger refunds ×38, Last-month cards ×5, calendar today, usual spending money |
| `ui_regress.mjs`: 32 pages × phone/desktop × light/dark | 564 | 0 |
| `design-rules.test.ts` | 7 | 0 (after fixing 2 UTC-date bugs it found) |

Scripts: `plugins/ft-dev/skills/ft-live-verify/scripts/`.

## Method (first pass)

1. **Cross-view checks:** a script (95 checks) compared each figure's formula with every view that shows it: Home, Plan, Review (Month, Year, Cash flow), Spending, Ledger, Summary, the cash calendar and the emergency fund.
2. **Raw recomputation:** separate code read the raw `transactions`, `balances`, `accounts`, `categories` and `budgets` tables. It recomputed income, spending, refunds, invested, saved and savings rate per month, net worth by group, the 12-month averages, essentials, the emergency fund and the planned fixed and saving amounts. It used its own pay-month rule, not the app's code.
3. **Visual pass:** every page at 390px (light) and 1440px (dark) in Firefox, scanned for console errors, NaN, sideways scroll and broken charts, then read by eye.

## Raw data vs app

| Figure | Raw | App | |
|---|---|---|---|
| Sep income / spending / invested | €6,475.61 / €6,352.44 / €1,666.21 | same | ✓ |
| Oct income / spending / invested | €3,759.91 / €4,000.58 / €508.44 | same | ✓ |
| Nov income / spending / saved | €4,231.79 / €3,144.21 / €1,087.58 | same | ✓ |
| Nov invested | €2,725.86 | €2,726.86 | data note 2 |
| Cash, investments, pension, crypto, debt | €8,955.88 / €26,995.06 / €18,175.00 / €1,452.79 / −€261,630.22 | same | ✓ |
| Property & car | €440,000.00 | €444,551.50 | data note 1 |
| 12-month income / spending / saved per month | €6,009.63 / €4,658.17 / €1,351.46 | same | ✓ |
| 12-month invested per month | €462.55 (linked accounts only) | €681.71 | ✓ by the app's rule, see data note 3 |
| Essentials per month | €3,012.75 | same | ✓ |
| Plan fixed / saving per month | €2,361.66 / €1,200.00 | same | ✓ |
| November pay | €4,125.01 | same | ✓ |

## Bugs found and fixed

| # | Where | Was | Now |
|---|---|---|---|
| 1 | Plan → Cash (runbook) | planned €0/day of spending until payday once the month was over budget, so top-ups were too small | your usual day (€56.82) |
| 2 | Plan → Forecast | November still expected €1,818 of pay that was never coming, so the month-end was €1,818 too high and the forecast went under the floor in January instead of today | €0 still to come; under the floor in November, as the calendar says |
| 3 | Plan → Budget, Rebalance; Summary | "the plan leaves −€476.65, a gap of €3,211" judged the plan by this short month | judged by the plan's usual month: leaves €1,448 |
| 4 | Plan → Budget, hero | offered to cut a budget line to fit the short pay | says it's a short month and leaves the lines as they are |
| 5 | Month day by day chart | the plan line sloped down to −€477, the axis lost its minus signs, and the label ran into the days | the plan line stays at €0, with a note under the chart |
| 6 | Cash calendar, payday strip | "under your floor from Tue 24 Nov" when it was already under today | "today" |
| 7 | Review → Cash flow tiles | "12 mo" summed only the chart's range (YTD = 10 months) and divided by 12: €5,099/month, 29% | always the last 12 complete months: €6,010/month, 22% |
| 8 | Emergency fund card, Summary | "3.0 of 3 months" and "going well" while €82–148 short (rounded up) | rounds down while short: 2.9, warning |
| 9 | Ledger | Income included refunds (+€244.28 in October) | refunds shown as "refunded" under spending |
| 10 | Spending, Last month | the month cards were missing | same cards as This month |
| 11 | Month day by day chart (found by you after the first pass) | a €995 bar labelled "€1,472 over", and "on plan by now €0" | in a short-pay month the line is the usual plan; the bar = its label; a note gives the sum. The chart's arithmetic is unit-tested (`month-chart.test.ts`) |
| 12 | Accounts, chart periods | dates worked out in UTC (a day off after midnight) | local day |

Invested tiles now say "net of money taken out": the 12-month figure nets the €16,981 taken out in December 2025.

## Why the first pass missed #11

It compared numbers with the raw data and with each other, and looked at charts as thumbnails. It didn't check each chart's shapes against its own labels, and it didn't re-check the chart that one of its own fixes had changed. Both are now part of the process (design-system.md §6 and §8).

## Data notes (for you, not the code)

1. **Solar panels €4,551.50:** the account is archived and its note says it's part of the house, but it still counts in net worth on top of the €410k house appraisal. Wealth flags it as "hidden but still counted". If the appraisal includes the panels, set its balance to €0 (or close it in Settings → Accounts). Net worth is then €4,551.50 lower.
2. **"Transfer testing" €1 (9 Nov), IBKR → Swedbank, category "between own accounts":** money out of a broker is normally a payout. Delete the test row, or recategorise it.
3. **Investing transfers with no destination account:** €1,300 in March (OKLO, MU, VNDA), the €1,000 IBKR top-up on 30 June, and €330 of RAV4 leasing in November 2025. They count as invested, but the IBKR and Revolut balances in the ledger never received them. Link them to their accounts to tidy up.

## Not changed

- "Savings rate" means income − spending (investing counts as saved). November reads 25.7% "a strong month" even though €1,639 more went out than came in once investing is counted. Spending's "Where November's income went" card shows that −€1,639.
- The month review's 6-month average invested (€3,487/month) is real: it includes the €11.3k invested in June.
