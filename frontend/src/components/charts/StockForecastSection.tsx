import { useQueries } from '@tanstack/react-query'
import {
  LineChart, Line, BarChart, Bar, XAxis, YAxis,
  CartesianGrid, Tooltip, Legend, ResponsiveContainer, ReferenceLine, Cell,
} from 'recharts'
import { stocksApi } from '../../api/stocks'
import type { StockHolding } from '../../types'

interface Props {
  holdings: StockHolding[]
  usdToEur: (n: number) => number | null
}

interface ScenarioDef {
  bear: number
  base: number
  bull: number
  analystTarget: string
  catalyst: string
}

// Per-ticker 12-month scenario assumptions (fractional change from current price)
const TICKER_SCENARIOS: Record<string, ScenarioDef> = {
  ETON:    { bear: -0.48, base:  0.43, bull:  1.25, analystTarget: 'avg $39 · high $52',     catalyst: 'HEMANGEOL May launch, mid-2026 FDA, DESMODA ramp' },
  INTU:    { bear: -0.21, base:  0.18, bull:  0.49, analystTarget: 'avg $799 · high $813',    catalyst: 'TurboTax AI, QuickBooks expansion' },
  VNDA:    { bear: -0.43, base:  0.57, bull:  1.57, analystTarget: 'avg $15.50 · high $24',   catalyst: 'Imsidolimab BLA, NEREUS launch, pipeline' },
  TM:      { bear: -0.19, base:  0.13, bull:  0.27, analystTarget: 'avg $257 · buy',          catalyst: 'Tariff clarity, hybrid demand, $1B US investment' },
  MSFT:    { bear: -0.11, base:  0.20, bull:  0.38, analystTarget: 'avg $595 · strong buy',   catalyst: 'Azure +37%, Copilot monetisation' },
  OKLO:    { bear: -0.49, base:  0.56, bull:  1.75, analystTarget: 'avg $97–117 · buy',       catalyst: 'NRC milestones, Aurora plant progress' },
  VWCE:    { bear: -0.10, base:  0.07, bull:  0.16, analystTarget: 'index',                   catalyst: 'Global market recovery, DCA monthly' },
  'VWCE.AS': { bear: -0.10, base: 0.07, bull: 0.16, analystTarget: 'index',                  catalyst: 'Global market recovery, DCA monthly' },
  NVDA:    { bear: -0.30, base:  0.25, bull:  0.70, analystTarget: 'avg $165 · strong buy',   catalyst: 'Blackwell ramp, data-center AI demand' },
  AAPL:    { bear: -0.15, base:  0.12, bull:  0.28, analystTarget: 'avg $230 · buy',          catalyst: 'iPhone cycle, Apple Intelligence rollout' },
  GOOGL:   { bear: -0.20, base:  0.18, bull:  0.40, analystTarget: 'avg $210 · buy',          catalyst: 'Search AI, cloud growth, ad recovery' },
  AMZN:    { bear: -0.18, base:  0.22, bull:  0.45, analystTarget: 'avg $245 · buy',          catalyst: 'AWS acceleration, ad tier growth' },
  META:    { bear: -0.20, base:  0.20, bull:  0.50, analystTarget: 'avg $720 · buy',          catalyst: 'AI ad targeting, Llama monetisation' },
  TSLA:    { bear: -0.40, base:  0.10, bull:  0.80, analystTarget: 'avg $320 · mixed',        catalyst: 'FSD revenue, robotaxi rollout, Optimus' },
}

const DEFAULT_SCENARIO: ScenarioDef = {
  bear: -0.20, base: 0.10, bull: 0.30,
  analystTarget: '—',
  catalyst: 'Broader market conditions',
}

const VWCE_BENCHMARK: ScenarioDef = TICKER_SCENARIOS.VWCE

function fmt(n: number, ccy: string) {
  const locale = ccy === 'EUR' ? 'de-DE' : 'en-US'
  return new Intl.NumberFormat(locale, { style: 'currency', currency: ccy, maximumFractionDigits: 2 }).format(n)
}
function fmtEur(n: number) { return fmt(n, 'EUR') }
function fmtPct(n: number) { return `${n >= 0 ? '+' : ''}${(n * 100).toFixed(0)}%` }


export default function StockForecastSection({ holdings, usdToEur }: Props) {
  // Fetch live prices for all active holdings
  const priceResults = useQueries({
    queries: holdings.map((h) => ({
      queryKey: ['stock-price', h.ticker],
      queryFn: () => stocksApi.getPrice(h.ticker),
      staleTime: 5 * 60 * 1000,
    })),
  })

  // Build per-holding current market value in EUR
  interface HoldingData {
    ticker: string
    currency: string
    shares: number
    currentPrice: number | null
    currentValueEur: number | null
    scenario: ScenarioDef
  }

  const holdingData: HoldingData[] = holdings.map((h, i) => {
    const price = priceResults[i].data?.price ?? null
    const valueInNative = price != null ? h.shares * price : null
    const valueEur = valueInNative != null
      ? h.currency === 'EUR' ? valueInNative : usdToEur(valueInNative)
      : null
    return {
      ticker: h.ticker,
      currency: h.currency,
      shares: h.shares,
      currentPrice: price,
      currentValueEur: valueEur,
      scenario: TICKER_SCENARIOS[h.ticker] ?? DEFAULT_SCENARIO,
    }
  })

  const pricesReady = holdingData.every((h) => h.currentValueEur != null)
  const totalEur = pricesReady
    ? holdingData.reduce((s, h) => s + (h.currentValueEur ?? 0), 0)
    : null

  // Portfolio scenario projected values (EUR)
  const portfolioBear = pricesReady
    ? holdingData.reduce((s, h) => s + (h.currentValueEur ?? 0) * (1 + h.scenario.bear), 0)
    : null
  const portfolioBase = pricesReady
    ? holdingData.reduce((s, h) => s + (h.currentValueEur ?? 0) * (1 + h.scenario.base), 0)
    : null
  const portfolioBull = pricesReady
    ? holdingData.reduce((s, h) => s + (h.currentValueEur ?? 0) * (1 + h.scenario.bull), 0)
    : null

  const portfolioBearPct = totalEur && portfolioBear ? (portfolioBear - totalEur) / totalEur : null
  const portfolioBasePct = totalEur && portfolioBase ? (portfolioBase - totalEur) / totalEur : null
  const portfolioBullPct = totalEur && portfolioBull ? (portfolioBull - totalEur) / totalEur : null

  // Timeline chart data
  const timelineData = totalEur && portfolioBear != null && portfolioBase != null && portfolioBull != null
    ? (() => {
        const months = [0, 2, 4, 6, 8, 10, 12]
        const now = new Date()
        return months.map((m) => {
          const t = m / 12
          const date = new Date(now.getFullYear(), now.getMonth() + m, 1)
          const label = m === 0
            ? 'Now'
            : date.toLocaleDateString('en', { month: 'short', ...(m === 12 ? { year: '2-digit' } : {}) })
          return {
            label,
            Bear:  Math.round(totalEur + (portfolioBear  - totalEur) * t),
            Base:  Math.round(totalEur + (portfolioBase  - totalEur) * t),
            Bull:  Math.round(totalEur + (portfolioBull  - totalEur) * t),
            VWCE:  Math.round(totalEur + totalEur * VWCE_BENCHMARK.base * t),
          }
        })
      })()
    : null

  // Return % comparison for bar chart
  const returnData = portfolioBearPct != null && portfolioBasePct != null && portfolioBullPct != null
    ? [
        { scenario: 'Bear', VWCE: VWCE_BENCHMARK.bear * 100, Portfolio: portfolioBearPct * 100 },
        { scenario: 'Base', VWCE: VWCE_BENCHMARK.base * 100, Portfolio: portfolioBasePct * 100 },
        { scenario: 'Bull', VWCE: VWCE_BENCHMARK.bull * 100, Portfolio: portfolioBullPct * 100 },
      ]
    : null

  if (holdings.length === 0) return null

  const CcyTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs space-y-1">
        <p className="font-semibold text-gray-700 mb-1">{label}</p>
        {payload.map((p: any) => (
          <div key={p.name} className="flex justify-between gap-4">
            <span style={{ color: p.color }}>{p.name}</span>
            <span className="font-medium">{fmtEur(p.value)}</span>
          </div>
        ))}
      </div>
    )
  }

  const PctTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs space-y-1">
        <p className="font-semibold text-gray-700 mb-1">{label} scenario</p>
        {payload.map((p: any) => (
          <div key={p.name} className="flex justify-between gap-4">
            <span style={{ color: p.fill }}>{p.name}</span>
            <span className={`font-medium ${p.value >= 0 ? 'text-green-600' : 'text-red-600'}`}>
              {p.value >= 0 ? '+' : ''}{p.value.toFixed(1)}%
            </span>
          </div>
        ))}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Section header */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 bg-gray-50 flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-gray-700">1-Year Scenario Forecast — Open Positions vs VWCE</h3>
            <p className="text-xs text-gray-400 mt-0.5">
              Bear / Base / Bull scenarios vs VWCE (global equity ETF benchmark) · Prices include live FX
            </p>
          </div>
          {totalEur != null && (
            <div className="text-right">
              <p className="text-xs text-gray-400">Portfolio base</p>
              <p className="text-sm font-bold text-blue-700">{fmtEur(totalEur)}</p>
            </div>
          )}
        </div>

        {/* Per-ticker scenario table */}
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200 text-xs">
              <tr>
                <th className="text-left px-4 py-2 font-semibold text-gray-600">Ticker</th>
                <th className="text-right px-4 py-2 font-semibold text-gray-600">Current</th>
                <th className="text-right px-4 py-2 font-semibold text-gray-600">
                  <span className="inline-flex items-center gap-1">Bear <span className="text-red-500">●</span></span>
                </th>
                <th className="text-right px-4 py-2 font-semibold text-gray-600">
                  <span className="inline-flex items-center gap-1">Base <span className="text-yellow-400">●</span></span>
                </th>
                <th className="text-right px-4 py-2 font-semibold text-gray-600">
                  <span className="inline-flex items-center gap-1">Bull <span className="text-green-500">●</span></span>
                </th>
                <th className="text-right px-4 py-2 font-semibold text-gray-600">Analyst target</th>
                <th className="text-left px-4 py-2 font-semibold text-gray-600">Key catalyst</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {holdingData.map((h) => {
                const ccy = h.currency
                const cp = h.currentPrice
                const bear = cp != null ? cp * (1 + h.scenario.bear) : null
                const base = cp != null ? cp * (1 + h.scenario.base) : null
                const bull = cp != null ? cp * (1 + h.scenario.bull) : null
                return (
                  <tr key={h.ticker} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-bold text-gray-900">{h.ticker}</td>
                    <td className="px-4 py-3 text-right text-gray-700">
                      {cp != null ? fmt(cp, ccy) : <span className="text-gray-300 text-xs">loading…</span>}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {bear != null ? (
                        <div>
                          <div className="text-red-500 font-semibold">{fmt(bear, ccy)}</div>
                          <div className="text-xs text-red-400">{fmtPct(h.scenario.bear)}</div>
                        </div>
                      ) : '—'}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {base != null ? (
                        <div>
                          <div className="text-yellow-600 font-semibold">{fmt(base, ccy)}</div>
                          <div className="text-xs text-yellow-500">{fmtPct(h.scenario.base)}</div>
                        </div>
                      ) : '—'}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {bull != null ? (
                        <div>
                          <div className="text-green-600 font-semibold">{fmt(bull, ccy)}</div>
                          <div className="text-xs text-green-500">{fmtPct(h.scenario.bull)}</div>
                        </div>
                      ) : '—'}
                    </td>
                    <td className="px-4 py-3 text-right text-gray-500 text-xs">{h.scenario.analystTarget}</td>
                    <td className="px-4 py-3 text-gray-500 text-xs max-w-xs">{h.scenario.catalyst}</td>
                  </tr>
                )
              })}

              {/* VWCE benchmark row */}
              <tr className="bg-blue-50/50 border-t-2 border-blue-100">
                <td className="px-4 py-3 font-bold text-blue-700">VWCE benchmark</td>
                <td className="px-4 py-3 text-right text-blue-600 text-xs">all-in equivalent</td>
                <td className="px-4 py-3 text-right">
                  <div className="text-red-500 font-semibold">{fmtPct(VWCE_BENCHMARK.bear)}</div>
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="text-yellow-600 font-semibold">{fmtPct(VWCE_BENCHMARK.base)}</div>
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="text-green-600 font-semibold">{fmtPct(VWCE_BENCHMARK.bull)}</div>
                </td>
                <td className="px-4 py-3 text-right text-gray-400 text-xs">index</td>
                <td className="px-4 py-3 text-gray-400 text-xs">{VWCE_BENCHMARK.catalyst}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      {/* Charts row */}
      {timelineData && returnData && totalEur != null && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Timeline line chart */}
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Portfolio Scenarios vs VWCE — 12 Months</h3>
            <p className="text-xs text-gray-400 mb-4">
              {fmtEur(totalEur)} base · linear projection per scenario
            </p>
            <ResponsiveContainer width="100%" height={280}>
              <LineChart data={timelineData} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                <YAxis
                  tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`}
                  tick={{ fontSize: 11 }}
                  width={55}
                  domain={['auto', 'auto']}
                />
                <Tooltip content={<CcyTooltip />} />
                <Legend wrapperStyle={{ fontSize: 11 }} />
                <ReferenceLine y={totalEur} stroke="#9ca3af" strokeDasharray="3 3" strokeWidth={1} />
                <Line type="monotone" dataKey="Bear" stroke="#ef4444" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="Base" stroke="#f59e0b" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="Bull" stroke="#10b981" strokeWidth={2} dot={false} />
                <Line type="monotone" dataKey="VWCE" stroke="#3b82f6" strokeWidth={1.5} strokeDasharray="6 3" dot={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>

          {/* Return % comparison bar chart */}
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Return % — Portfolio vs VWCE Benchmark</h3>
            <p className="text-xs text-gray-400 mb-4">12-month projected return per scenario · positive = outperforms VWCE</p>
            <ResponsiveContainer width="100%" height={280}>
              <BarChart data={returnData} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="scenario" tick={{ fontSize: 12 }} />
                <YAxis tickFormatter={(v) => `${v.toFixed(0)}%`} tick={{ fontSize: 11 }} />
                <Tooltip content={<PctTooltip />} />
                <Legend wrapperStyle={{ fontSize: 11 }} />
                <ReferenceLine y={0} stroke="#9ca3af" strokeWidth={1} />
                <Bar dataKey="VWCE" name="VWCE only" radius={[4, 4, 0, 0]}>
                  {returnData.map((d, i) => (
                    <Cell key={i} fill={d.VWCE >= 0 ? '#93c5fd' : '#fca5a5'} />
                  ))}
                </Bar>
                <Bar dataKey="Portfolio" name="Your portfolio" radius={[4, 4, 0, 0]}>
                  {returnData.map((d, i) => (
                    <Cell key={i} fill={d.Portfolio >= 0 ? '#10b981' : '#ef4444'} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Scenario outcome summary */}
      {portfolioBearPct != null && portfolioBasePct != null && portfolioBullPct != null && totalEur != null && (
        <div className="grid grid-cols-3 gap-4">
          {[
            { label: 'Bear case', pct: portfolioBearPct, value: portfolioBear, vwce: VWCE_BENCHMARK.bear, color: 'red' },
            { label: 'Base case', pct: portfolioBasePct, value: portfolioBase, vwce: VWCE_BENCHMARK.base, color: 'yellow' },
            { label: 'Bull case', pct: portfolioBullPct, value: portfolioBull, vwce: VWCE_BENCHMARK.bull, color: 'green' },
          ].map(({ label, pct, value, vwce, color }) => {
            const beats = pct > vwce
            return (
              <div key={label} className={`bg-white rounded-xl border p-4 ${
                color === 'red' ? 'border-red-200' : color === 'yellow' ? 'border-yellow-200' : 'border-green-200'
              }`}>
                <p className="text-xs font-medium text-gray-500 mb-1">{label}</p>
                <p className={`text-2xl font-bold ${
                  pct >= 0 ? 'text-green-600' : 'text-red-600'
                }`}>
                  {pct >= 0 ? '+' : ''}{(pct * 100).toFixed(1)}%
                </p>
                <p className="text-sm text-gray-600 mt-0.5">{value != null ? fmtEur(value) : '—'}</p>
                <div className={`mt-2 text-xs font-medium px-2 py-0.5 rounded-full inline-block ${
                  beats ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                }`}>
                  {beats ? `+${((pct - vwce) * 100).toFixed(1)}% vs VWCE` : `${((pct - vwce) * 100).toFixed(1)}% vs VWCE`}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
