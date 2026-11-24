# Decisions

Product, design and technical decisions, newest first, each with the reason. Add one when a choice would otherwise get re-argued in a later refactor. Format: what was decided, why, what it rules out.

---

## 2026-11-24 · 3.2.0 · Mortgage principal from the balance history; habits, not shops

**Decided:** (1) Old one-row mortgage payments (Sep 2022 – Apr 2026) are split by migration 22: principal = that month's drop in the loan balance (production's history on 2026-11-24), interest = the rest of the payment (the owner's rule, also for payments to Evelina that exceeded the bank's instalment). The fix replays on restoring a pre-22 JSON backup. (2) Insights → Optional judges habits only — paid in 6+ of the last 12 months — as Keep / Cut back; no one-off shops, no guessed "leak" labels. (3) Month review opens on the current month.
**Why:** the owner asked for the principal back to the loan's start, and found "Worth it or a leak?" meaningless: it graded shops like bigbox (one-off electronics) and called DIY runs leaks because they came late in a month. None of its 20 rows had been answered.
**Rules out:** inferring "leak" from timing; asking for a verdict on a single purchase.

## 2026-11-24 · Home: what you see, not what to move

**Decided:** the app is a visualization tool, not the place money is managed — the banks do that. Top-up suggestions are `info` notes, left out of Home's "Needs you" and shown only in the payday card; the "pay yourself first" reminder is a note too. Home gains: an Investments row with today's move in the net-worth panel, saved & invested against the plan and over-budget lines in the month card, the emergency fund under target as a to-do, and "New since you last looked" (rows added since Home was marked seen, `prefs.home_seen`) instead of a fixed six. The "budgets differ from your last 12 months" note shows only in the month's first week.
**Why:** the owner, 2026-11-24: transfer suggestions shouldn't be the focus. Usage: Ledger is the most-tapped exit from Home (rows annotated there), Investments the most hunted-for page, and Insights → Summary (where saving moved in 3.0) is barely opened.
**Rules out:** a "Move €X" strip or headline on Home; Record-a-transfer buttons in "Needs you".
**Also:** for "how much went out, investing included" Home shows Insights → Spending's "This month so far, with investing" chart (`OutPaceCard`) rather than a text line of its own — the owner preferred the chart. It is the one panel shown in two places, by request, like the month chart in 3.0.1.

## 2026-11-23 · 3.0.1 · The month chart stays on Home

**Decided:** Home's month card keeps the day-by-day chart (same chart as Plan → Budget). On Home its short-pay note is left to the card's headline.
**Why:** the owner missed it after 3.0.0 moved it to Plan only. Together with the full-size net-worth panel, it is part of what Home is opened for.
**Rules out:** moving Home's charts elsewhere to "simplify" without asking.

## 2026-11-23 · 3.0.0 · Home is a one-page review

**Decided:** Home reads top to bottom: the full-size net-worth panel with its chart (the owner's favourite part; a compact version was reverted), a **Today** card (verdict: cash risk first, then the month; then "Needs you": the bell's actions, most urgent first), this month, cash until payday, the last 6 transactions.
**Why:** Home is the most-used page (~1,100 views in 60 days, ~5 min each), but it gave a big chart and four cards equal weight, so nothing said what needed attention.
**Rules out:** adding panels to Home that don't answer "what needs me today" or "where do I stand".

## 2026-11-23 · 3.0.0 · Pages by job, not by feature

**Decided:** Home = today, Plan = this month and ahead, Insights = looking back, Wealth = where money ends up. Each panel lives in one place. Insights 9 → 5 tabs, Wealth 8 → 7, Plan gains Cash.
**Why:** 60 days of usage showed Home (1,116 views, ~5 min each) as the daily page, and Investments as the most hunted-for page (25 hunts, ~5 hops). The same panels (left to spend, free money, emergency fund, "plan ended over") appeared on 2–4 pages each.
**Rules out:** copying a panel to a second page. Link to it instead.

## 2026-11-23 · Sub-views look like Ledger's

**Decided:** views that share a tab use the small segmented control (`SubNav`), the same as Ledger → Bank inbox. Every page is also listed in the menu.
**Why:** a first version used pill chips; the owner couldn't find Stress test and the rows looked unlike Ledger's. Stress test went back to its own tab.
**Rules out:** filter-looking controls for navigation; renaming tabs the owner knows ("Recurring" stayed, "Bills" was reverted).

## 2026-11-23 · Salary counts in the month it arrives (Vialet)

**Decided:** rule `{from: 2026-11-01, paid_by_day: 10, in_month: true}`. The owner sets it on prod after 3.0.0.
**Why:** pay lands around the 5th–10th, so each month gets one pay. The standing orders (IBKR on the 1st, SEB on the 16th, Evelina on the 17th) land in the same month as the pay, and the month matches the bank. The old "month before" rule moved only the salary, not the saving done from it.
**Cost accepted:** October 2026 reads low (one pay), with a note on that month.
**Technical:** `paid_by_day` used to mean both "counts for the month before" and "payday". The `in_month` flag splits them, so the cash plan keeps its payday. It's JSON in the plan settings: no migration, ignored by 2.x on rollback.

## 2026-11-23 · A short month is measured against the pay that came

**Decided:** with pay counted in its own month, once this month's pay is in (≥ half the plan), left to spend uses it. The plan's fit (Rebalance, Summary, the day chart's line) is still judged by the **usual** month (`income_planned`).
**Why:** a partial first pay (€4,125) otherwise showed €6,050 to spend. Judging the plan by that one month made Rebalance say "the plan leaves −€477" and offered to cut budget lines.
**Rules out:** permanent budget changes driven by one short month.

## 2026-11-23 · One spend number

**Decided:** Left to spend leads; free money and budget lines left are its breakdown, folded, and they add up to it.
**Why:** one screen showed left to spend, per day, projected, committed, free money and income base at once.

## 2026-11-23 · Refunds are money back on spending

**Decided:** refunds reduce spending everywhere, including the Ledger totals (shown as "refunded"). They never count as income.
**Why:** the Ledger was the one view that counted them as income (+€244 in October 2026).

## 2026-11-23 · Every release is checked against the raw data

**Decided:** before a release, `regress_raw.py` recomputes every figure from the raw tables with independently written rules, across all months and both salary settings. `ui_regress.mjs` then checks every page.
**Why:** checks of the app against itself missed bugs (a chart bar that didn't match its label; "12 mo" tiles summing 10 months). Run against 2.87.0, the harness fails 46 checks on the bugs fixed in 3.0.0.

## 2026-11-23 · UX checks are part of the regression

**Decided:** the UI regression also measures layout: columns that end far apart, and text cut off outside lists. It fails on Home and on cut summary text, and notes the rest for a full-size review.
**Why:** checks that the app *works* (errors, NaN, numbers) passed while Home had a large empty right column and cut-off payday text, and Optional, Recurring and Investments had columns ending 650–1,600px apart. The visual review had looked at thumbnails.

## 2026-11-23 · Changelog in the add-on

**Decided:** `finance_tracker/CHANGELOG.md`, one entry per release, in user terms, including what to do after updating.
**Why:** Home Assistant shows it on the add-on's Changelog tab and in the update notice.
