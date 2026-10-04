import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { ReviewTotals } from '../../api/review'
import { formatEuro } from '../../utils/format'
import { GRID, INCOME, SPENDING } from './colors'

type Row = ReviewTotals & { month: string }

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const short = (ym: string) => MONTHS[Number(ym.slice(5, 7)) - 1] ?? ym
const compact = (v: number) => (Math.abs(v) >= 1000 ? `${Math.round(v / 100) / 10}k` : String(Math.round(v)))

function Tip({ active, payload }: any) {
  if (!active || !payload?.length) return null
  const r: Row = payload[0].payload
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-sm px-3 py-2 text-xs space-y-0.5">
      <p className="font-semibold text-gray-700 mb-1">{short(r.month)} {r.month.slice(0, 4)}</p>
      <p className="flex items-center gap-2"><span className="w-3 h-0.5" style={{ backgroundColor: INCOME }} /><span className="font-semibold text-gray-900">{formatEuro(r.income)}</span><span className="text-gray-400">income</span></p>
      <p className="flex items-center gap-2"><span className="w-3 h-0.5" style={{ backgroundColor: SPENDING }} /><span className="font-semibold text-gray-900">{formatEuro(r.spending)}</span><span className="text-gray-400">spending</span></p>
      <p className="text-gray-500 pt-0.5">Net saved <span className={`font-semibold ${r.net_saved < 0 ? 'text-red-600' : 'text-gray-900'}`}>{formatEuro(r.net_saved)}</span></p>
    </div>
  )
}

// Income and spending per month for a year. The reviewed month is in full
// color and the rest recede (emphasis), so the eye lands on "this month in
// context". Clicking a month opens its review.
export default function TwelveMonthChart({ rows, current, onPick }: { rows: Row[]; current: string; onPick: (month: string) => void }) {
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2 mb-2">
        <h3 className="text-sm font-semibold text-gray-900">Last 12 months</h3>
        <div className="flex items-center gap-3 text-xs text-gray-600">
          <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: INCOME }} />Income</span>
          <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: SPENDING }} />Spending</span>
        </div>
      </div>
      <div className="h-56">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart
            data={rows}
            margin={{ top: 4, right: 0, left: -12, bottom: 0 }}
            barGap={2}
            barCategoryGap="22%"
            onClick={(e: any) => e?.activeLabel && onPick(e.activeLabel)}
          >
            <CartesianGrid vertical={false} stroke={GRID} />
            <XAxis dataKey="month" tickFormatter={short} tick={{ fontSize: 11, fill: '#6b7280' }} tickLine={false} axisLine={{ stroke: '#e5e7eb' }} interval={0} />
            <YAxis tickFormatter={compact} tick={{ fontSize: 11, fill: '#9ca3af' }} tickLine={false} axisLine={false} width={44} />
            <Tooltip content={<Tip />} cursor={{ fill: '#f8fafc' }} />
            <Bar dataKey="income" maxBarSize={12} radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer">
              {rows.map((r) => <Cell key={r.month} fill={INCOME} fillOpacity={r.month === current ? 1 : 0.3} />)}
            </Bar>
            <Bar dataKey="spending" maxBarSize={12} radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer">
              {rows.map((r) => <Cell key={r.month} fill={SPENDING} fillOpacity={r.month === current ? 1 : 0.35} />)}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      <details className="mt-2">
        <summary className="text-xs text-blue-600 cursor-pointer select-none">Show as table</summary>
        <table className="w-full text-xs mt-2">
          <thead className="text-gray-500">
            <tr><th className="text-left font-medium py-1">Month</th><th className="text-right font-medium">Income</th><th className="text-right font-medium">Spending</th><th className="text-right font-medium">Net</th></tr>
          </thead>
          <tbody className="divide-y divide-gray-50 tabular-nums">
            {[...rows].reverse().map((r) => (
              <tr key={r.month} className={r.month === current ? 'font-semibold text-gray-900' : 'text-gray-600'}>
                <td className="py-1">{short(r.month)} {r.month.slice(0, 4)}</td>
                <td className="text-right">{formatEuro(r.income)}</td>
                <td className="text-right">{formatEuro(r.spending)}</td>
                <td className={`text-right ${r.net_saved < 0 ? 'text-red-600' : ''}`}>{formatEuro(r.net_saved)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </div>
  )
}
