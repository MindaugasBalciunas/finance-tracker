// The design system's hard rules (docs/design-system.md), checked on the
// source so a refactor can't quietly drift from them.
const raw = import.meta.glob(['../pages/**/*.{ts,tsx}', '../components/**/*.{ts,tsx}', '../lib/**/*.{ts,tsx}'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const src = Object.entries(raw).map(([p, s]) => ({ p: p.replace(/^\.\.\//, ''), s }))
const offenders = (re: RegExp, allow: string[] = []) => src.filter(({ p, s }) => !allow.includes(p) && re.test(s)).map(({ p }) => p)

describe('design rules', () => {
  it('colours come from theme tokens, not hex (the logo artwork and bank brand colours excepted)', () => {
    expect(offenders(/['"`]#[0-9a-fA-F]{3,8}['"`]/, ['components/Logo.tsx', 'components/charts.tsx', 'lib/brand.ts'])).toEqual([])
  })
  it('a day is a local date, never toISOString (UTC is a day off after midnight)', () => {
    expect(offenders(/toISOString\(\)\.(slice|split|substring)/)).toEqual([])
  })
  it('data goes through the api wrapper (file download and upload excepted)', () => {
    expect(offenders(/[^.\w]fetch\(/, ['pages/Settings.tsx', 'pages/settings/Data.tsx', 'lib/api.ts', 'lib/usage.ts', 'components/Logo.tsx', 'components/MerchantLogo.tsx'])).toEqual([])
  })
  it('payloads are typed: no api.get<any>', () => {
    expect(offenders(/api\.(get|post|put|patch|del)<any>/)).toEqual([])
  })
  it('a page with tabs has the shared page header', () => {
    expect(src.filter(({ p, s }) => p.startsWith('pages/') && s.includes('<Tabs ') && !s.includes('<PageHeader')).map(({ p }) => p)).toEqual([])
  })
  it('views that share a tab use SubNav or Segmented, never hand-made pill rows', () => {
    expect(offenders(/aria-label="Views"/, ['components/ui.tsx'])).toEqual([])
  })
  it('every page in the menu exists as a route', async () => {
    const { SITEMAP } = await import('../lib/sitemap')
    const routes = src.filter(({ p }) => /pages\/(Ledger|Plan|Wealth|Insights)\.tsx$/.test(p)).map(({ s }) => s).join('\n')
    for (const pages of Object.values(SITEMAP)) for (const { to } of pages) {
      const seg = to.split('/')[2]
      if (seg) expect(routes, to).toContain(`path="/${seg}"`)
    }
  })
})
