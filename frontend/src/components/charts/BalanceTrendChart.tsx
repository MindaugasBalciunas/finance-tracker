import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import type { BalanceTrend } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  trend: BalanceTrend
}

const ACCOUNT_COLORS: Record<string, string> = {
  seb: '#3b82f6',
  swed: '#10b981',
  swed_etf: '#6366f1',
  swed_pen: '#8b5cf6',
  luminor: '#f59e0b',
  art: '#ec4899',
  cash: '#14b8a6',
  rev_m: '#f97316',
  rev_r: '#ef4444',
  r_btc: '#f59e0b',
  m_btc: '#d97706',
  rev_stocks: '#06b6d4',
}

const ACCOUNT_LABELS: Record<string, string> = {
  seb: 'SEB',
  swed: 'Swedbank',
  swed_etf: 'Swed ETF',
  swed_pen: 'Swed Pension',
  luminor: 'Luminor',
  art: 'Art',
  cash: 'Cash',
  rev_m: 'Revolut M',
  rev_r: 'Revolut R',
  r_btc: 'R BTC',
  m_btc: 'M BTC',
  rev_stocks: 'Rev Stocks',
}

export default function BalanceTrendChart({ trend }: Props) {
  const data = trend.dates.map((date, i) => {
    const row: Record<string, number | string> = { date }
    row['total'] = trend.totals[i]
    Object.keys(trend.accounts).forEach((acc) => {
      row[acc] = trend.accounts[acc][i] ?? 0
    })
    return row
  })

  const activeAccounts = Object.keys(trend.accounts).filter((acc) =>
    trend.accounts[acc].some((v) => v > 0)
  )

  return (
    <ResponsiveContainer width="100%" height={380}>
      <LineChart data={data} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="date" tick={{ fontSize: 11 }} />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} />
        <Tooltip formatter={(v: number) => formatEuro(v)} />
        <Legend />
        <Line
          type="monotone"
          dataKey="total"
          stroke="#1d4ed8"
          strokeWidth={2.5}
          dot={false}
          name="Total"
        />
        {activeAccounts.map((acc) => (
          <Line
            key={acc}
            type="monotone"
            dataKey={acc}
            stroke={ACCOUNT_COLORS[acc] ?? '#94a3b8'}
            strokeWidth={1.5}
            dot={false}
            name={ACCOUNT_LABELS[acc] ?? acc}
            strokeDasharray="4 2"
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
