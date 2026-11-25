# Changelog

## 3.3.0 — Dip plans: when to buy and how much, for ETFs, bets and BTC

### Wealth → Investments → Dip plan (new)
- **Your written crash plan for VWCE, VALL, SPYL, SXRV and BCFP.** It sits next to Positions under Investments. Each fund gets a ladder of limit prices from 10 years of daily prices, recomputed every day:
  - **−10% under the 200-day average:** nothing extra; such dips usually mend.
  - **2-year average:** first third of the fund's reserve.
  - **4-year average:** second third.
  - **−44% from the high:** last third.
  The thirds go top down by price.
- When an **unfilled gap** (a range the price jumped over and never came back to) sits by an average, the limit goes to the gap's top. On QQQ that gives 602 and 501, the plan's own numbers.
- **What to do now** answers first:
  - "Buy €1,000 now" when a rung is reached, with shares at today's price.
  - Otherwise "Nothing extra to buy" and your **limit orders to keep**: one line per fund, as shares × price = total, coloured green (buy now), amber (within 5%) or plain.
  - **Copy all** puts the orders on the clipboard.
- **Bought from your trades, nothing to tick.** A buy of the fund in the current fall, at or under a rung's limit and worth at least half its third, counts as that rung bought. The plan stops asking until the next fall. Trades arrive by the IBKR sync or when you add them.
- **The reserve is savings above your safety net.** The card shows the cash above the emergency fund's target and caps "Buy now" at it. Its link opens the emergency fund card.
- **The chart comes first and is bigger:** three years, full width at the top, with the averages, the rungs as dashed lines in their average's colour, and **dots for your own buys**.
- VALL, SPYL and BCFP are too young for a 4-year average, so they borrow VWCE's, SXR8's and SXRV's history (same index), scaled to their own price. Prices in dollars are converted, so share counts are right.
- **Plan broken** shows if a fund stays under its 4-year average for over half a year: hold the reserve and rewrite the plan.

### Investments → Bets and → BTC (new)
- **Bets** puts your single stocks at Revolut on the same ladder: ABIVAX, Intuit, Oklo and Vanda to start, each with its own reserve. A note says the rules were written for index funds.
- **BTC** puts bitcoin (BTC-EUR) on it too. Its 2- and 4-year averages count calendar days, since bitcoin trades every day.
- The cash above the safety net goes to the funds first, then the bets, then BTC.

### Home and phone
- The **Investments** row shows the total gain on what you hold (value against what you paid, e.g. "total −€2,676 (−10.0%)") under today's move.
- When a rung is reached and not bought, the bell shows it as a note with the amount and how much free cash covers it.
- The phone gets one notification per rung and fall, without amounts.

### After updating
- Set each plan's reserve under **Funds & reserve**, **Stocks & reserve** and **Coins & reserve** (they start at €0).
- Ask CFO and connected AI apps read the plans with **get_dip_plan** (plan = funds, bets or crypto).

## 3.2.0 — Mortgage principal back to 2022, habits to keep or cut

### Your data (runs once when the add-on restarts)
- **Mortgage principal back to the start (Sep 2022).** Until April 2026 each mortgage payment, mostly paid to Evelina before the divorce, was one "house payment" expense, so the principal counted as spending instead of wealth built. Each of those 44 payments now keeps its interest and gets a linked split part for the principal, from the loan's monthly balance: €18,925 over Sep 2022 – Apr 2026. Net worth and every balance stay the same. Invested and loan principal go up, spending goes down by the same amount (Insights → Summary's "Invested · 12 mo" rises by about €2,000).
- 13 Jan 2023's €1,170 "Lizingas" payment is now January's mortgage payment, split the same way.
- A payment you edited since is left alone. Restoring an older backup applies the same split.

### Insights
- **Review** opens on the current month. Links to a finished month's review still open that month.
- **Optional spending:** "Worth it or a leak?" is now **Your regular optional spending**: only places and categories you paid in 6 or more of the last 12 months, each with what it costs a month and **Keep** / **Cut back**. One-off shops (bigbox, DIY stores) and guessed "looks like a leak" labels are gone. Earlier "Worth it" and "Cut" choices carry over as Keep and Cut back.

## 3.1.0 — Home shows what came in and where it went

### Home
- **New since you last looked:** the last card lists every row added since you last tapped **Mark seen** (bank syncs and your own entries), so notes, splits and must/optional can be done from Home. With nothing new it shows the last six rows, as before. The first visit after updating starts the clock, so the list begins empty.
- **This month so far, with investing** is on Home too (the same chart as Insights → Spending): spending plus investing day by day against last month and a typical month, with what was spent and invested so far. On desktop it sits under the month card, and cash until payday moves to the left column.
- **Investments** has its own row in the net-worth panel, with today's move, one tap from the Investments page.
- **This month's card** adds:
  - **Saved & invested** against the plan, e.g. "€2,727 · plan €1,200 · €1,527 more".
  - The budget lines that are over, named in one line.
- **Needs you:**
  - Lists the emergency fund when it is under its target.
  - Leaves suggested top-ups to the payday card. Moving money happens in your bank; the app shows where it stands.
- "Budgets differ from your last 12 months" shows only in the first week of the month.
- "pending" on bank rows is grey instead of orange, here and in the Ledger.

### Notifications
- Suggested top-ups and "Pay yourself first" are notes, never amber.

## 3.0.1 — The month chart is back on Home

- Home's "Left to spend" card shows the month day by day again: spending so far against the plan's pace, where this rhythm ends up, and bills due or paid. 3.0.0 had left it only on Plan.
- In a short-pay month the card's headline explains the short pay, so the note under the chart shows only on Plan's full-width chart, not three times on Home.

## 3.0.0 — New layout, salary in the month it arrives

A major version: pages are reorganised by what they're for, and every figure is now checked against the raw data before release. Your data isn't migrated. Going back to 2.87.0 works; there, a salary rule's "month it arrives" choice is ignored, so pay counts for the month before again.

### After updating
- **Choose how salary counts:** Plan → Income & salary → Salary timing. For a payday around the 5th–10th, pick **Month it arrives** on the rule from 1 Nov 2026 (payday by the 10th). Nothing changes until you choose.
- If you switch, October 2026 will read low (one pay instead of two) and is marked as the job-change month. November is measured against the €4,125 that actually came in.

### Finding things
- The phone menu lists every page by section, and the desktop sidebar lists the open section's pages, so every page is one tap from the menu.
- Views that share a tab show as a second row of tabs, in the same small control as Ledger → Bank inbox (To review · Added · Dismissed):
  - Spending → Categories · Merchants · Optional
  - Habits → Trends · Drift
  - Goals → Wish list · Trips
  - Review → Month · Year · Cash flow
- Old links, notifications and bookmarks still open the right page.

### Home
- Home is a one-page review, read top to bottom:
  - **net worth**: the same full-size panel and chart as before
  - **Today**: one sentence on where you stand (cash risk first, then the month), then **Needs you**, the actions due (top-ups, inbox, budgets), each one tap away
  - **this month**: left to spend; in a short-pay month it says the pay was short, not that you overspent
  - **cash until payday**
  - the last 6 transactions, with Add and Scan
- The payday strip leads with cash risk (e.g. "under your €9,020 floor from 17 Dec"), then the next top-up with a Record button. The full plan is on Plan → Cash.
- Tap Investments (or any group) in the net-worth bar to open it directly.
- The savings rate, emergency fund, independence and invested tiles moved to Insights → Summary.

### Plan
- New **Cash** tab: everything until payday (top-ups, buffers, low points) and the next 60 days day by day, which used to be under Forecast.
- **Budget** leads with one number, left to spend:
  - Free money is one line that opens its breakdown.
  - This month shows only the spending lines that need a look ("Show 9 on track" for the rest).
- A past month links to its Month review, and Month review links back to the plan for that month.
- **Goals** holds the wish list and trips. **Income & salary** holds the income base and salary timing, side by side on wide screens.

### Insights
- 5 tabs instead of 9: Summary · Review · Spending · Habits · Fun facts.
- Spending shows the same month cards for **Last month** as for **This month**: the month day by day, with investing, and where the month's income went.
- "Plan ended over" appears once on Summary, as a line pointing to Plan.

### Wealth
- **Property & loans** shows the house, the car and the mortgage on one page.
- **Independence** now includes the emergency fund card and its inputs (birth year, target age, withdrawal rate, emergency months), moved from Plan.
- Stress test keeps its own tab.

### Salary month
- A salary rule can now count pay **in the month it arrives** and still know your payday, so Cash until payday keeps working.
- With that setting, once this month's pay is in, left to spend uses the pay that came, not the planned salary.

### Fixes (from a full QA against the raw data, see docs/qa-3.0.0.md)
- The Ledger's Income total no longer includes refunds. They show as "refunded" under spending, so the Ledger agrees with Cash flow, Review and Plan (October had read €244 more income there).
- Cash until payday plans your usual day's spending when the month is already over budget, instead of €0, so top-ups are big enough.
- Forecast no longer expects pay this month that has already arrived. The month-end and the "under your floor" month now agree with the 60-day calendar.
- Rebalance and Summary judge the plan by its usual month, not by a short one-off pay. A short month no longer offers to cut a budget line.
- The month day by day chart, in a short-pay month, draws the **usual** plan, so it shows whether your spending is on track. A note gives the sum: "€453 left at usual pay − €1,925 short pay = €1,472 over". Its bar and label always agree. (It had drawn a €995 bar labelled "€1,472 over", and "on plan by now €0".)
- "Under your floor" says **today** when cash is already under it.
- Review → Cash flow's "12 mo" tiles always use the last 12 complete months. On "This year" they had summed 10 months and divided by 12.
- The emergency fund rounds down while short: no more "3.0 of 3 months" with €148 short.
- The invested tiles say they are net of money taken out.
- Top-up notifications open Plan → Cash.
- Fun facts → Who paid you the most always lists your current employer. Vialet's first, partial pay ranked 7th and was cut off.
- Two dates ("idle for half a year" on Accounts, chart periods) were worked out in UTC, which is a day off after midnight. They now use your local day.

### Layout
- Desktop columns end together: Home (review column on the right), Optional, Recurring (transfers beside the subscription review) and Investments (closed positions under trades). Before, one column often ended far above the other, leaving large empty areas.
- Summary tiles, Home's "Needs you", the payday card and Wealth's "What moved" rows wrap instead of cutting text off.

### Behind the scenes
- A design system (docs/design-system.md) and a decision log (docs/decisions.md). A test fails when code breaks the core rules: theme colours only, local dates, typed data, the shared header and sub-tab control, and every menu page existing.
- A raw-data regression of about 1,700 checks across all months, under both salary settings, plus a UI regression of every page on phone and desktop in light and dark. Run against 2.87.0, it fails 46 checks on the bugs fixed here.

## 2.87.0
- Spending shows where this month's income went, by account.

## 2.86.1
- Every bank inbox row can be dismissed.

## 2.86.0
- Spending shows this month so far, with investing too.

## 2.85.0
- Revolut and the mortgage follow bank transfers by themselves.

## 2.84.1
- Insights → Optional figures agree with each other and with the Ledger.

## 2.84.0
- A transfer between two of your banks is one transaction.

## 2.83.0
- Mark each payment Must or Optional, by swipe or in the form.
