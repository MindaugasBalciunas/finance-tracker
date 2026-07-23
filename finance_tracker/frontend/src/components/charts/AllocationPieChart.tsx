import { memo } from 'react'
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import type { AccountAllocation } from '../../types'
import { formatEuro } from '../../utils/format'
import { useIsMobile } from '../../hooks/useIsMobile'

interface Props {
  allocations: AccountAllocation[]
}

const COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
  '#14b8a6', '#f97316', '#ec4899', '#6366f1', '#d97706',
  '#06b6d4', '#84cc16',
]

function CustomTooltip({ active, payload }: any) {
  if (!active || !payload?.length) return null
  const d = payload[0].payload
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
      <p className="font-semibold text-gray-800 mb-1">{d.account}</p>
      <p className="text-gray-700">{formatEuro(d.amount)}</p>
      <p className="text-gray-500">{d.percentage.toFixed(1)}%</p>
    </div>
  )
}

const AllocationPieChart = ({ allocations }: Props) => {
  // Long outside labels clip at phone widths — the table below carries the
  // detail there, so mobile shows compact percent-only labels.
  const isMobile = useIsMobile()
  const sorted = [...allocations].sort((a, b) => b.amount - a.amount)

  const main = sorted.filter((a) => a.percentage >= 5)
  const small = sorted.filter((a) => a.percentage < 5)
  const data = small.length > 0
    ? [
        ...main,
        {
          account: 'Other',
          amount: small.reduce((sum, a) => sum + a.amount, 0),
          percentage: small.reduce((sum, a) => sum + a.percentage, 0),
        },
      ]
    : main

  return (
    <div>
      <ResponsiveContainer width="100%" height={isMobile ? 240 : 320}>
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            outerRadius={isMobile ? 80 : 110}
            dataKey="amount"
            nameKey="account"
            label={isMobile
              ? ({ percentage }) => `${percentage.toFixed(0)}%`
              : ({ account, percentage, amount }) =>
                  `${account} ${formatEuro(amount)} (${percentage.toFixed(1)}%)`
            }
            labelLine={true}
            isAnimationActive={false}
          >
            {data.map((_, i) => (
              <Cell key={i} fill={COLORS[i % COLORS.length]} />
            ))}
          </Pie>
          <Tooltip content={<CustomTooltip />} />
        </PieChart>
      </ResponsiveContainer>

      <table className="w-full text-xs mt-3 border-collapse">
        <thead>
          <tr className="text-gray-400 border-b border-gray-100">
            <th className="text-left font-medium py-1.5 pr-2">Account</th>
            <th className="text-right font-medium py-1.5 pr-2">Amount</th>
            <th className="text-right font-medium py-1.5">%</th>
          </tr>
        </thead>
        <tbody>
          {data.map((d, i) => (
            <tr key={d.account} className="border-b border-gray-50 hover:bg-gray-50">
              <td className="py-1.5 pr-2">
                <span className="inline-flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm flex-shrink-0" style={{ backgroundColor: COLORS[i % COLORS.length] }} />
                  {d.account}
                </span>
              </td>
              <td className="text-right py-1.5 pr-2 font-medium text-gray-700">{formatEuro(d.amount)}</td>
              <td className="text-right py-1.5 text-gray-500">{d.percentage.toFixed(1)}%</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default memo(AllocationPieChart)
