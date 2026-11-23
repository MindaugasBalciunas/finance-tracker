// UI regression: every page at 390px and 1440px, light and dark, in Firefox.
// Fails on console errors, NaN/undefined in text, sideways scroll, a page
// without the shared header/tab bar, a sub-view row that isn't the shared
// segmented control, and headline figures that differ from the API (which
// regress_raw.py checks against the raw data).
//
//   node ui_regress.mjs http://127.0.0.1:5185 http://127.0.0.1:8091/api [out-dir]
//
// Needs playwright installed where it runs (see ft-live-verify).
import { firefox } from 'playwright'

const [web = 'http://127.0.0.1:5185', apiBase = 'http://127.0.0.1:8091/api', out = ''] = process.argv.slice(2)
const ROUTES = ['', 'ledger', 'ledger/inbox', 'ledger/tidy', 'plan', 'plan/cash', 'plan/recurring', 'plan/forecast', 'plan/wishlist', 'plan/trips', 'plan/settings',
  'wealth', 'wealth/investments', 'wealth/ownership', 'wealth/loans', 'wealth/fi', 'wealth/stress', 'wealth/tax', 'wealth/history',
  'insights', 'insights/review', 'insights/cashflow', 'insights/spending', 'insights/merchants', 'insights/optional', 'insights/trends', 'insights/drift', 'insights/fun',
  'ai', 'settings', 'settings/accounts', 'settings/data']
// Pages that share a tab show this row of views.
const SUBNAV = { 'insights/spending': 3, 'insights/merchants': 3, 'insights/optional': 3, 'insights/trends': 2, 'insights/drift': 2, 'plan/wishlist': 2, 'plan/trips': 2, 'insights/review': 3, 'insights/cashflow': 3, 'ledger/inbox': 3 }
const get = async (p) => (await fetch(apiBase + p)).json()
const eur = (v) => (v < 0 ? '-' : '') + '€' + Math.round(Math.abs(v)).toLocaleString('en-US')

const fails = []
const ux = []
let passed = 0
const check = (name, ok, detail = '') => { if (ok) passed++; else fails.push(`${name} ${detail}`) }

const ov = await get('/overview')
const cf = await get(`/insights/cashflow?from=${(() => { const d = new Date(ov.date); d.setMonth(d.getMonth() - 12); return d.toISOString().slice(0, 7) })()}-01&to=${ov.date}&granularity=month`)
const last12 = cf.filter((f) => f.period < ov.date.slice(0, 7)).slice(-12)
const inc12 = last12.reduce((a, f) => a + f.income, 0)
// What each page must show, rounded the way the page does.
const EXPECT = {
  '': [eur(ov.liquid), eur(ov.plan.safe_to_spend).replace('-', '-')],
  plan: [eur(ov.plan.safe_to_spend)],
  'insights/cashflow': [eur(inc12), eur(inc12 / 12)],
}

const b = await firefox.launch()
for (const [w, h, scheme] of [[390, 844, 'light'], [390, 844, 'dark'], [1440, 900, 'light'], [1440, 900, 'dark']]) {
  const p = await b.newPage({ viewport: { width: w, height: h }, colorScheme: scheme, hasTouch: w < 500 })
  const errs = []
  p.on('pageerror', (e) => errs.push(e.message))
  p.on('console', (m) => m.type() === 'error' && !/favicon|Failed to load resource/.test(m.text()) && errs.push(m.text().slice(0, 160)))
  for (const r of ROUTES) {
    const tag = `${w}/${scheme} #/${r}`
    await p.goto(`${web}/#/${r}`)
    await p.waitForLoadState('networkidle')
    await p.waitForTimeout(1600) // CountUp settles
    const s = await p.evaluate(() => {
      const main = document.querySelector('main')
      const text = main?.innerText ?? ''
      return {
        text,
        h1: !!main?.querySelector('h1'),
        overflow: document.documentElement.scrollWidth > innerWidth + 1,
        bad: (text.match(/NaN|undefined|Infinity|€-€|−−/g) ?? []).slice(0, 3),
        tabs: main?.querySelectorAll('[aria-current="page"]').length ?? 0,
        // the shared segmented control: a sunken rounded group of buttons
        segs: [...(main?.querySelectorAll('div.bg-sunken.rounded-xl') ?? [])].map((d) => d.querySelectorAll(':scope > button').length),
        pills: main?.querySelectorAll('nav[aria-label="Views"] button.rounded-full').length ?? 0,
        // UX: side-by-side columns whose ends differ a lot leave an empty
        // area beside the shorter one (Home's short right column).
        gaps: [...(main?.querySelectorAll('*') ?? [])].filter((el) => {
          const cs = getComputedStyle(el)
          return cs.display === 'grid' && cs.gridTemplateColumns.split(' ').length >= 2 && el.children.length >= 2
        }).flatMap((g) => {
          const kids = [...g.children].map((c) => c.getBoundingClientRect()).filter((r) => r.width > 0 && r.height > 0)
          const rows = new Map()
          for (const r of kids) { const k = Math.round(r.top / 8); rows.set(k, [...(rows.get(k) ?? []), r]) }
          return [...rows.values()].filter((rs) => rs.length >= 2).map((rs) => {
            const bottoms = rs.map((r) => r.bottom)
            return Math.round(Math.max(...bottoms) - Math.min(...bottoms))
          }).filter((d) => d > 320).map((d) => `${(g.className || g.tagName).toString().slice(0, 40)}: ${d}px`)
        }),
        // UX: the same for CSS columns (masonry): compare each column's end.
        mcols: [...(main?.querySelectorAll('*') ?? [])].filter((el) => {
          const cs = getComputedStyle(el)
          return cs.columnCount !== 'auto' && Number(cs.columnCount) > 1 && el.children.length >= 2
        }).flatMap((m) => {
          const ends = new Map()
          for (const c of m.children) { const r = c.getBoundingClientRect(); if (!r.height) continue; const k = Math.round(r.left / 20); ends.set(k, Math.max(ends.get(k) ?? 0, r.bottom)) }
          const v = [...ends.values()]
          return v.length >= 2 && Math.max(...v) - Math.min(...v) > 320 ? [`columns ${(m.className || '').toString().slice(0, 30)}: ${Math.round(Math.max(...v) - Math.min(...v))}px`] : []
        }),
        // UX: text cut off with an ellipsis.
        cut: [...(main?.querySelectorAll('*') ?? [])].filter((el) => {
          const cs = getComputedStyle(el)
          // Rows of a list (divide-y, table, li) may cut a secondary line: the
          // full row is one tap away. Summaries, tiles, alerts and actions may not.
          return cs.textOverflow === 'ellipsis' && el.scrollWidth > el.clientWidth + 1 && el.textContent.trim().length > 0 && !el.closest('.divide-y, table, li, [role="listitem"]')
        }).map((el) => el.textContent.trim().slice(0, 60)),
      }
    })
    check(`${tag} renders without errors`, errs.length === 0, errs.splice(0).join(' | '))
    check(`${tag} no NaN/undefined`, s.bad.length === 0, s.bad.join(','))
    check(`${tag} no sideways scroll`, !s.overflow)
    if (r && !r.startsWith('ai')) check(`${tag} has the page header`, s.h1)
    if (SUBNAV[r]) check(`${tag} sub-views use the shared control`, s.segs.includes(SUBNAV[r]) && s.pills === 0, JSON.stringify(s.segs))
    for (const want of EXPECT[r] ?? []) check(`${tag} shows ${want}`, s.text.replace(/−/g, '-').includes(want), '')
    // Layout and readability: reported for review with full-size screenshots;
    // Home (the daily page) must have none.
    for (const g of [...s.gaps, ...s.mcols]) ux.push(`${tag} uneven columns — ${g}`)
    check(`${tag} no summary text cut off`, s.cut.length === 0, s.cut.map((c) => `"${c}"`).join(' | '))
    if (r === '') check(`${tag} Home columns balanced`, s.gaps.length === 0, s.gaps.join('; '))
    if (out && w === 390 && scheme === 'light') await p.screenshot({ path: `${out}/m-${r.replace(/\//g, '_') || 'home'}.png`, fullPage: true })
    if (out && w === 1440 && scheme === 'dark') await p.screenshot({ path: `${out}/d-${r.replace(/\//g, '_') || 'home'}.png`, fullPage: true })
  }
  await p.close()
}
await b.close()
console.log(`${passed} UI checks passed, ${fails.length} failed, ${ux.length} layout notes to review`)
for (const u of ux) console.log('  ~', u)
for (const f of fails) console.log('  ✗', f)
process.exit(fails.length ? 1 : 0)
