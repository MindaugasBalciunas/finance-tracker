---
name: ft-web-ui
description: Front-end conventions for finance-tracker/web — api wrapper and TanStack Query hooks, query-key invalidation, ui.tsx primitives, pickers and account-kind rules, money/date formatting, Tailwind 4 theme tokens and the CSS-layer gotcha, Recharts settings, mobile-first 390px and Firefox requirements, and copy style. Load before building or changing any page, component, chart or form in finance_tracker/web.
---

# Web UI conventions

The owner uses the app mostly on a phone, in Firefox, as an installed PWA. Daily transaction entry is the most-used path and must stay fast and logical.

## Data

- `src/lib/api.ts`: `api.get/post/put/del<T>(path, …)` — same-origin `api/…`, JSON, throws `ApiError(status, message, data)`; a 401 `locked` dispatches `ft:locked` (app shows the lock). Don't call `fetch` directly.
- `src/lib/hooks.ts`: shared queries (`useOverview`, `useAccounts`, `useCategories`, `usePlan(month)`, `useInbox`, …). Add a hook there when data is read in more than one place. Reference data gets a `staleTime`.
- Mutations: `useMutation` + `qc.invalidateQueries({ queryKey: [...] })` for **every** view whose numbers change — a new transaction moves `overview`, `plan`, `transactions`, balances. Stale Home numbers are a bug.
- Server-side prefs (`usePrefs`, `usePeriod(key, …)`) for view choices that should follow the owner across devices; `localStorage` only for per-device trivia.
- Demo mode: switching resets every query except `['demo']` — never show one database's cache in the other.
- Types for payloads in `src/lib/types.ts`. Money arrives as euro numbers.

## Building blocks — reuse before writing new

`components/ui.tsx`: `Card`, `Stat`, `Delta` (colour by meaning, `goodWhenUp`), `Meter`, `Money`, `Spinner`/`Loading`, `Empty`, `ErrorBox`, `Segmented`, `Sheet` (bottom sheet with `footer`), `Field`, `Toggle`, `useToast`, `PageHeader`, `Tabs`, `AskCFO`, `NumberInput` (keeps what's typed, accepts `,`), `PinInput`.
`components/pickers.tsx`: `CategoryPicker`, `AccountSelect` with `kinds`, `TagInput`, `NoteInput`.
`components/TxEditor.tsx`, `QuickActions.tsx`, `charts.tsx`, `Icon.tsx`.

**Account kinds in forms** (mirror of `ledger.Validate`): expense/income from `SPEND_KINDS` (checking, savings, cash); transfer from `TRANSFER_FROM_KINDS` (+ brokerage, crypto); transfer to `TRANSFER_TO_KINDS` (+ pension, loan). Property/vehicle never appear as "paid from".

## Formatting (`src/lib/format.ts`)

- `eur(v)` overview figures (whole euros, but keeps cents under €1,000 — €11.46 must never read €11); `eurc` exact list amounts; `eurk` axis ticks; `signed` deltas; `pct`. `—` for missing values. Use the `tnum` class for numeric columns.
- Dates: `dayLabel`, `shortDate`, `monthLabel`, `todayISO`, `thisMonth`, `addMonths` — local-time ISO strings, never `toISOString()` for a day (UTC shift).

## Styling (Tailwind 4)

- Theme tokens from `index.css` `@theme`: `bg-page/surface/raised/sunken`, `text-ink/ink2/muted`, `border-line`, `text-accent`, `text-good/bad/warn` (and `bg-*`, with opacity like `bg-accent/10`). Both colour schemes come from the CSS variables — never hard-code hex in components.
- **Custom CSS goes inside `@layer base` / `@layer components`.** Unlayered rules beat every Tailwind utility; that once broke paddings across Firefox and mobile.
- Sheet footers need bottom padding (safe area); horizontal tab bars must not show vertical scrollbars; `input[type=date]` needs `min-w-0` in flex rows.
- Charts: Recharts with `isAnimationActive={false}`, series colours `var(--s1)`…`var(--s8)`, `var(--s-other)`; axis/ink from tokens.

## Routing

`HashRouter` — routes are `/#/ledger`, `/#/plan`, …. Settings sub-pages under `pages/settings/`, insights under `pages/insights/`.

## Copy

Short, plain, second person, money-first ("€120 left this month"). Provider-neutral: say "AI", not a vendor. Errors from the server are already sentences — show them via `ErrorBox`/toast, don't wrap them in jargon.

## Done for a UI change

1. `npx tsc --noEmit && npx vitest run` green.
2. Live check on an isolated copy at **390px and desktop, in Firefox** (light and dark), plus the add/edit transaction and bank-inbox flows if touched — see `ft-live-verify`.
3. Accessible names on icon buttons (`aria-label`), visible focus, tap targets ≥ 40px.
