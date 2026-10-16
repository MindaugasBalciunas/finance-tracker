---
name: ft-live-verify
description: How to see a finance-tracker change working in the real app without touching the owner's data — isolated probe server on a VACUUM INTO copy, auth cleared on the copy, Vite on its own port, Playwright Firefox at 390px and desktop, and clean-up by port. Load when asked to run, screenshot or live-check the app, or before shipping a UI, schema or bank-flow change.
---

# Live verification on an isolated copy

The owner uses their local instance themselves (PIN + passkey enabled) and production is the Home Assistant add-on. **Never verify against either.** Spin up your own copy.

## 1. Pick free ports

Old sessions' probes may squat ports (some with auth on). Check before binding:

```bash
for p in 8091 8092 8093 5185 5186; do lsof -iTCP:$p -sTCP:LISTEN -n -P >/dev/null && echo "$p busy"; done
```

## 2. Copy the data

Use `VACUUM INTO` — a plain `cp` leaves the `-wal` file behind and gives silently stale rows.

```bash
S=<scratchpad>/probe && mkdir -p $S
sqlite3 /Users/mindaugas/finance-tracker/finance_tracker/backend/finance-v2.db "VACUUM INTO '$S/finance-v2.db'"
# clear auth on the COPY only
sqlite3 $S/finance-v2.db "DELETE FROM settings WHERE key='auth'; DELETE FROM webauthn_credentials;"
```

For a fresh, data-free run skip the copy and point `DB_PATH` at a file that doesn't exist yet (with `V1_DB_PATH=/nonexistent` it starts empty). For realistic fake data, turn on demo mode in the probe.

## 3. Run server + web

```bash
cd /Users/mindaugas/finance-tracker/finance_tracker/server
DB_PATH=$S/finance-v2.db V1_DB_PATH=/nonexistent PORT=8091 go run ./cmd/server > $S/server.log 2>&1 &   # run_in_background
cd ../web && API_URL=http://127.0.0.1:8091 npx vite --host 127.0.0.1 --port 5185 --strictPort > $S/vite.log 2>&1 &
curl -sf http://127.0.0.1:8091/api/health
```

Bind Vite to `127.0.0.1` explicitly — a second listener on the same port can catch IPv4 traffic. Confirm `lsof` shows *your* processes on those ports before trusting any result.

## 4. Look at it — Firefox, 390px and desktop

Firefox is what the owner uses; Chrome-only checks have missed real breakage. Browsers are cached under `~/Library/Caches/ms-playwright`; install the library into the scratchpad, not the repo:

```bash
cd $S && npm init -y >/dev/null && npm i playwright@latest >/dev/null
```

```js
// $S/shot.mjs — node shot.mjs '#/ledger' ledger
import { firefox } from 'playwright'
const [route = '#/', name = 'home'] = process.argv.slice(2)
const b = await firefox.launch()
for (const [w, h, scheme] of [[390, 844, 'light'], [390, 844, 'dark'], [1280, 900, 'dark']]) {
  const p = await b.newPage({ viewport: { width: w, height: h }, colorScheme: scheme, hasTouch: w < 500 })
  const errors = []; p.on('pageerror', (e) => errors.push(e.message)); p.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
  await p.goto(`http://127.0.0.1:5185/${route}`); await p.waitForLoadState('networkidle')
  await p.screenshot({ path: `${name}-${w}-${scheme}.png`, fullPage: true })
  const overflow = await p.evaluate(() => document.documentElement.scrollWidth > innerWidth)
  console.log(w, scheme, overflow ? 'HORIZONTAL OVERFLOW' : 'ok', errors)
  await p.close()
}
await b.close()
```

Routes use the hash (`#/plan`, `#/settings/data`). Read the screenshots; check paddings, sheet footers, tab bars, numbers not truncated, no horizontal scroll, no console errors. Exercise the flow you changed (open the sheet, save a transaction) with `p.click`/`p.fill`, not just the landing view.

## 5. Clean up

```bash
for p in 8091 5185; do kill $(lsof -t -iTCP:$p -sTCP:LISTEN) 2>/dev/null; done
```

Kill by port, not `pkill -f` (it has missed probes before, and stale servers on deleted DBs gave bogus results).

## Report

Say what you ran it against (copy of which DB, which commit), which viewports/schemes, what you clicked, and anything that looked wrong — with the screenshot paths.
