# PRD — Calmer navigation and pay-month salary

Status: released in 3.0.0 (see finance_tracker/CHANGELOG.md, docs/decisions.md) · 2026-11-23

## 1. Problem

The app has grown to 5 sections and 23 tabs. On a phone (390px), the pages are long:

| Page | Length today |
|---|---|
| Plan → Budget | 5.8 screens |
| Insights → Fun facts | 6.2 screens |
| Insights → Optional | 5.7 screens |
| Insights → Spending | 5.1 screens |
| Wealth → Investments | 5.0 screens |
| Home | 3.5 screens |

Most of that length comes from showing **the same panels in several places**:

| Panel | Shown on |
|---|---|
| Left to spend + Free money | Home and Plan → Budget |
| The month day by day | Home and Plan → Budget |
| "Plan ended over in N of 6 months" | Home, Plan → Budget, Insights → Summary |
| Emergency fund | Home tile, Plan → Budget, Wealth → Independence, Insights → Summary |
| Top merchants | Insights → Spending and Insights → Merchants |
| "This month so far" pace charts | Insights → Spending, on every period, even "12 months" |

Some content is also on a page that doesn't match its purpose. The cash runbook ("Until payday") sits on Home. Its 60-day calendar sits in Plan → Forecast, and its buffers sit in Plan settings. The FI inputs sit in Plan, while the FI results are in Wealth.

The salary month rule has its own problem. Since 1 Nov a salary paid by the 10th counts for the month before. Only the salary moves, though: the investing and savings transfers made from that pay stay in the month they happen. A pay arriving on 10 Dec therefore counts in November, while the €1,200 moved to investing on 10–12 Dec counts in December. November's savings rate looks better than it was and December's worse. Also, every month's "left to spend" depends on pay that hasn't arrived yet.

## 2. Goals

1. **One purpose per section, and each panel lives in one place.**
   - Home = today
   - Plan = this month and what's coming
   - Insights = looking back
   - Wealth = where the money ends up
2. **Home fits in about 1.5 phone screens**, with net worth first and add/scan one tap away, as the owner prefers.
3. **Fewer top-level tabs:** 23 → 17. Closely related views share a tab and switch with a small toggle.
4. **Nothing breaks.** Every old URL (notifications, AI answers, MCP links, bookmarks) opens the right view.
5. **Salary counts in the month it arrives** for the Vialet payroll (paid ~5th–10th), and the setting says plainly what each choice means.

Out of scope: new numbers or new server endpoints (apart from the salary-rule flag in §4), and redesigning individual cards.

## 3. New layout

### Home — today

| Order | Panel | Change |
|---|---|---|
| 1 | Quick actions | unchanged |
| 2 | Net worth / liquid hero | unchanged |
| 3 | **This month** (left to spend, month bar, outlook, "how it's worked out") | day-by-day chart and 6-month track record removed — both are on Plan → Budget |
| 4 | **Payday** strip: next salary date, top-ups to make, "Record" for the next one | replaces the full "Until payday" card, which moves to Plan → Cash |
| 5 | Recent + Scan / Add | unchanged |

Removed from Home:
- the Free money card (stays on Plan → Budget)
- the 4 progress tiles (moved to Insights → Summary)
- the fun-fact teaser (Fun facts tab)

### Home details (from the review)

- Each asset group in the net-worth bar opens its page, plus an **Investments ›** link: Investments was the most hunted-for page (25 hunts in 60 days, about 5 hops each).
- The payday strip leads with the cash risk: the day cash runs out or dips under the emergency floor in the next 60 days.

### Finding pages

Sub-views that share a tab (Spending → Categories · Merchants · Optional; Habits → Trends · Drift; Goals → Wish list · Trips; Review → Month · Year · Cash flow) show as a **second row of tabs**, not a filter-like toggle. The desktop sidebar lists the open section's pages under it, and the phone menu lists every page by section (`web/src/lib/sitemap.ts`). Every page is one tap from the menu.

### Plan — this month and what's coming

| Tab | Content |
|---|---|
| Budget | Left to spend (the one spend number), actual so far, free money folded into one line that opens its breakdown, the month day by day, rebalance, budget lines (this month: only spending lines that need a look, the rest behind "Show N on track"), once a year, year at a glance. A past month links to its Month review. Emergency fund moves to Wealth. |
| **Cash** *(new)* | The full "Until payday" runbook (moves, buffers, per-account low points) + "Next 60 days, day by day" (from Forecast) |
| Recurring | recurring costs + subscription review (unchanged name — renaming it "Bills" broke the owner's habit) |
| Forecast | What-if and month by month (the 60-day calendar moved to Cash) |
| **Goals** | Wish list · Trips, as a second row of tabs |
| Income & salary | Income base and salary timing side by side on wide screens (FI inputs moved to Wealth → Independence) |

### Insights — looking back (9 → 5 tabs)

| Tab | Content |
|---|---|
| Summary | The 4 progress tiles from Home, then the findings feed. "Plan ended over in N months" is one line pointing to Plan, not a second copy. |
| Review | Month · Year · **Cash flow** (Cash flow was its own tab). A month links to Plan for that month. |
| Spending | **Categories · Merchants · Optional** toggle. The month-to-date pace cards appear only on the "This month" period. |
| **Habits** | **Trends · Drift** toggle |
| Fun facts | unchanged |

### Wealth — where the money ends up (8 → 7 tabs)

| Tab | Content |
|---|---|
| Net worth | unchanged |
| Investments | unchanged |
| **Property & loans** | Ownership (house, car) + loans on one page |
| Independence | Takes the full emergency fund card from Plan and the FI inputs from Plan settings. |
| Stress test | unchanged, own tab (behind a toggle the owner couldn't find it) |
| Taxes | unchanged |
| History | unchanged |

### Redirects (old URL → new view)

These paths keep working with no redirect needed:
- `/insights/cashflow`, `/insights/merchants`, `/insights/optional`, `/insights/trends`, `/insights/drift`
- `/wealth/loans`, `/wealth/stress`
- `/plan/recurring`, `/plan/wishlist`, `/plan/trips`, `/plan/settings`

Each still renders its view, now highlighted under its new parent tab. `/insights/habits` opens Trends. The redirects that already existed (`/insights/recurring`, `/insights/fi`, `/insights/tax`, `/plan/payday`, `/plan/decisions`) are kept.

## 4. Salary month

**Decision:** count pay in the month it arrives from the Vialet start (2026-11-01): rule `{from: 2026-11-01, paid_by_day: 10, in_month: true}`. Earlier jobs keep their rules.

**Why a new flag:** `paid_by_day` did two jobs. It set which month pay counts in, and it was the day the app expects payday. Setting it to 0 (the only way to count pay in its own month) made the app lose the payday, so Cash until payday, the Home payday strip and the emergency fund's "leaving before payday" all disappeared. The new `in_month` flag on a salary rule separates the two. With the flag set, salary counts in the month it arrives and `paid_by_day` only gives the payday. Rules without it behave exactly as before. The flag is stored in the plan settings JSON, so there is no migration, and backups carry it. The MCP and AI tool schemas and the advice text describe it.

| | Pay month (chosen) | Earned month (today) |
|---|---|---|
| Matches the bank | yes | no — the month's income arrives next month |
| Investing/saving moves land in the same month as the pay | yes | no — only salary is shifted |
| "Left to spend" rests on money in hand | yes | it rests on pay that hasn't arrived |
| Month closes | on the 1st | on the 10th of the next month |
| Risk | a payday slipping past month end leaves a gap (not possible with a ~5th–10th payday) | a pay arriving on the 11th lands in the wrong month |

Effect on history: October keeps Nexos' pay of 9 Oct. November gets Vialet's first pay (5 Nov, €4,125 — a partial month). From December, each month gets one full pay.

**UI change (Plan → Income → Salary timing):** each rule becomes a clear choice: **"Month it arrives"** (with "Payday, by day N") or **"Month before"** (with "If paid by day N"), plus a one-line explanation under each. "New job" now starts with "Month it arrives, payday by the 10th".

**This month's income (review fix):** with pay counted in the month it arrives, once this month's pay is in (at least half the plan), the month is measured against that pay. Before this fix, the partial first Vialet pay (€4,125) still left "€6,050 to spend", which overstated November by about €1,820. An advance under half the plan keeps the modelled figure, because the rest is still to come.

**The switch month:** the month before a change in how pay is counted gets no pay or two. Plan and Month review show a note on that month (`salary_note`) saying its income reads low or high as a one-off, not as overspending.

**Rollout:** a settings change the owner makes on prod in Plan → Income. It isn't applied by a migration, because a migration would rewrite the owner's own choice.

## 5. Acceptance

- At 390px in Firefox: Home is ≤ 2 screens, and no page scrolls sideways.
- Every old route listed above opens the right view.
- `make test` is green.
- Server changes: the `in_month` salary-rule flag, this month's income from the pay that arrived, and `salary_note` on the plan report and month review (all with tests). No API route changes.
- Every page is reachable from the phone menu and from the desktop sidebar.
