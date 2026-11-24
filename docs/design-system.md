# Design system

The rules every page follows. Read this before building or moving a page, panel, chart or control. The hard rules are enforced by `web/src/test/design-rules.test.ts`. The rest is checked by `plugins/ft-dev/skills/ft-live-verify/scripts/ui_regress.mjs` and by review.

Product decisions behind these rules are in [decisions.md](decisions.md).

## 1. Information architecture

Each section has one job. A panel lives in one place; other places link to it.

| Section | Job | Tabs |
|---|---|---|
| **Home** | a one-page review of today | none: net worth panel (full size, with its chart, and an Investments row with today's move) · Today (verdict + needs you) · this month (with its day-by-day chart and saved & invested) · this month so far, with investing (Insights → Spending's chart, at the owner's ask) · cash until payday · new since you last looked (else recent) |
| **Ledger** | every transaction | Transactions · Bank inbox · Tidy up |
| **Plan** | this month and what's coming | Budget · Cash · Recurring · Forecast · Goals (Wish list / Trips) · Income & salary |
| **Wealth** | where the money ends up | Net worth · Investments · Property & loans · Independence · Stress test · Taxes · History |
| **Insights** | looking back | Summary · Review (Month / Year / Cash flow) · Spending (Categories / Merchants / Optional) · Habits (Trends / Drift) · Fun facts |

- **Before adding a panel**, ask which job it serves. If another section already shows the same figure, link there instead of copying the panel.
- **The menu is the site map.** `web/src/lib/sitemap.ts` lists every page; the phone menu shows all of them and the desktop sidebar shows the open section's. A new page goes there too. The design-rule test fails if a menu entry has no route.
- **Old URLs never break.** When a page moves, its old path keeps working: either it is still a route under its new tab, or it gets a `<Navigate replace>`.
- **Home is a one-page review**, read top to bottom; the net-worth panel keeps its full size and chart. It says *what needs you* before it shows detail. The Today card leads with a verdict (cash risk first, then the month) and the actions due: the bell's list without the top-ups, which stay in the payday card. Slow numbers (savings rate, FI) live on Insights → Summary; this month's saved & invested against the plan is on Home's month card. **The app shows money; the banks move it.** Transfer advice (top-ups, "pay yourself first" still to move) is always an `info` note, kept out of Home's "Needs you" and left to the payday card — never a headline.

## 2. Page anatomy

```
PageHeader   title · AskCFO on the right
Tabs         the section's tabs (underlined, scroll sideways on a phone)
SubNav       views that share a tab — small Segmented, like Ledger → Bank inbox
Filters      period chips / selects for the view (optional)
Content      cards, most important first
```

- **Views vs filters.** A choice that changes *which view* you see (Merchants vs Categories, Wish list vs Trips, Month vs Year) uses `SubNav`, or `Segmented size="sm"` inside a view. A choice that *narrows the same view* (This month · Last month · 12 months) uses chips (`chip`/`chip-on`). Never hand-made pill rows for navigation.
- Each sub-view is a route, so it has its own link (notifications, bookmarks, AI answers).
- Desktop: use the width. Two columns where two cards answer related questions (income and salary timing; planned trips and suggestions). Never a single narrow column with half the page empty.
- Phone first: 390px, no sideways scroll, tap targets ≥ 40px for main navigation.
- **Columns end together.** Side-by-side columns on desktop end within about 320px of each other. Otherwise move a card across, or use balanced CSS columns (`columns-2`, each card its own block), as Insights → Summary and Optional do. A short column beside a long one leaves a large empty area; that was the main layout complaint about 3.0's first Home.
- **Only list rows may cut text.** A row in a list (`divide-y`, table) may truncate its secondary line, because the full row is one tap away. Summaries, tiles, alerts, actions and "Needs you" items wrap instead.

## 3. Components (`web/src/components/ui.tsx`)

| Need | Use |
|---|---|
| Page title + Ask CFO | `PageHeader` |
| Section tabs | `Tabs` |
| Sub-views | `SubNav` (routes) / `Segmented size="sm"` (state) |
| A panel | `Card` (title, icon, colour by meaning, action on the right) |
| One headline figure | `Stat` |
| Change with colour by meaning | `Delta` (`goodWhenUp`) |
| Progress toward a target | `Meter` (`pace` tick, `goal`) |
| Form in a bottom sheet | `Sheet` with `footer` |
| Nothing to show | `Empty` |
| Long breakdown under a headline | a one-line fold that opens it (e.g. Plan → Free money) |

Reuse before writing new. A new primitive goes in `ui.tsx` and in this table.

## 4. Colour

- Only theme tokens: `bg-page/surface/raised/sunken`, `text-ink/ink2/muted`, `text-good/warn/bad`, `--s1…--s8` for series, `--hue-*` for meaning (income, spending, saving, fixed, invest…). No hex in components. The exceptions are the logo artwork and the banks' brand colours (`lib/brand.ts`).
- Each section has its hue (`--hue-home`, `--hue-plan`…). Cards take it unless the content has a meaning of its own: good/warn/bad, income/spending.
- Red means over or short; amber means heading there; green means done or on track. Never decoration.

## 5. Numbers

- **One spend number.** Left to spend leads on Home and Plan. Other framings (free money, budget lines left) are its breakdown, one tap away, and must add up to it.
- Money: `eur()` for headline figures (whole euros, cents under €1,000), `eurc` for list amounts, `eurk` for axes, `signed` for deltas. Negative values show a minus.
- A figure shown in two places is the same figure from the same endpoint. If two numbers differ by definition, the labels must say how: "net of money taken out", "after what leaves before payday".
- Rounding never contradicts a state: while short of a target, months round **down** (2.95 shows 2.9, not "3.0 of 3").
- "12 mo" means the last 12 complete months, whatever range a chart next to it shows.
- Dates are local days (`todayISO`, `daysAgo`), never `toISOString` (UTC is a day off after midnight).

## 6. Charts (Recharts)

- `isAnimationActive={false}`, series colours `--s1…`, axes from tokens.
- **Shapes match labels.** A bar's length is the number on its label; a line called "on plan" is the plan the card talks about. Put the arithmetic in a pure function and unit-test it (`timelineModel` in `MonthChart.tsx`).
- Axes never go negative unless the value is negative, and then they show the minus.
- When a chart can't say something with geometry (a short pay, no spending money), say it in a one-line note under the chart, with the sum that adds up to the card's figure.
- Tooltips use the same names as the legend.

## 7. Copy

- Short, plain, second person, money first: "€120 left this month".
- Provider-neutral: "AI", not a vendor.
- Explain the unusual month: a pay-schedule change, a short pay or a payout gets a note, so it doesn't read as overspending.

## 8. Checking a change

1. `make test`, which includes `design-rules.test.ts`.
2. `regress_raw.py`: every figure recomputed from the raw tables, every view, all months, under both salary settings.
3. `ui_regress.mjs`: every page at 390px and 1440px, light and dark. It **fails** on errors, NaN, sideways scroll, a missing shared header or sub-tab control, headline figures that differ from the API, text cut off outside a list, and Home's columns not ending together. It **notes** uneven columns on other pages for review.
4. Review every noted page **and every changed page at full size**, not as thumbnails. Check layout as well as numbers: empty areas, what draws the eye first, cut text, and each chart's bar, line and tooltip against its labels.
