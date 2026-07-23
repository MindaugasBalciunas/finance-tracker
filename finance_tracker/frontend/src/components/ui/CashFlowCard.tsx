import { formatEuro } from '../../utils/format'

interface Props {
  periodLabel: string
  income: number
  expenses: number
  invested: number
  netSaved: number | null
  savingsRateValue: string
  savingsRateSubtitle: string
  savingsRateTone: 'green' | 'yellow' | 'red'
  avgMonthlySpend: number | null
  completeMonthsCount: number
  runwayMonths: number | null
}

const TONE_TEXT = {
  green: 'text-green-600',
  yellow: 'text-yellow-600',
  red: 'text-red-600',
}

const RUNWAY_TARGET = 6 // months of free cash considered a full emergency fund

function FlowBar({ label, value, max, color, note }: {
  label: string
  value: number
  max: number
  color: string
  note?: string
}) {
  const pct = max > 0 ? Math.min((value / max) * 100, 100) : 0
  return (
    <div className="flex items-center gap-3">
      <span className="w-16 sm:w-20 text-xs text-gray-500 flex-shrink-0">{label}</span>
      <div className="flex-1 h-5 bg-gray-50 rounded overflow-hidden">
        <div className={`h-full rounded ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="w-24 sm:w-28 text-right text-sm font-semibold text-gray-800 flex-shrink-0 whitespace-nowrap">
        {formatEuro(value)}
      </span>
      {note && <span className="hidden sm:inline w-20 text-xs text-gray-400">{note}</span>}
    </div>
  )
}

// One card replacing eight stat tiles: the money flow of the period as
// proportional bars, plus savings rate and the free-cash runway meter.
export default function CashFlowCard({
  periodLabel, income, expenses, invested, netSaved,
  savingsRateValue, savingsRateSubtitle, savingsRateTone,
  avgMonthlySpend, completeMonthsCount, runwayMonths,
}: Props) {
  const max = Math.max(income, expenses + invested)
  const spendPctOfIncome = income > 0 ? (expenses / income) * 100 : null
  const investedPctOfIncome = income > 0 ? (invested / income) * 100 : null

  const runwayPct = runwayMonths != null ? Math.min((runwayMonths / RUNWAY_TARGET) * 100, 100) : null
  const runwayColor = runwayMonths == null ? 'bg-gray-300'
    : runwayMonths >= RUNWAY_TARGET ? 'bg-green-500'
    : runwayMonths >= 3 ? 'bg-yellow-400'
    : 'bg-red-400'

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-baseline justify-between gap-2 mb-4">
        <h3 className="text-base font-semibold text-gray-900">Cash Flow</h3>
        <span className="text-xs text-gray-400">{periodLabel}</span>
      </div>

      <div className="space-y-2">
        <FlowBar label="Income" value={income} max={max} color="bg-green-500" />
        <FlowBar
          label="Spending"
          value={expenses}
          max={max}
          color="bg-red-400"
          note={spendPctOfIncome != null ? `${spendPctOfIncome.toFixed(0)}% of inc.` : undefined}
        />
        <FlowBar
          label="Invested"
          value={invested}
          max={max}
          color="bg-blue-500"
          note={investedPctOfIncome != null ? `${investedPctOfIncome.toFixed(0)}% of inc.` : undefined}
        />
      </div>

      <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1 mt-4 pt-4 border-t border-gray-100">
        <div>
          <span className="text-xs text-gray-500 mr-1.5">Net saved</span>
          <span className={`text-lg font-bold ${netSaved != null && netSaved >= 0 ? 'text-green-600' : 'text-red-600'}`}>
            {netSaved != null ? formatEuro(netSaved) : '—'}
          </span>
        </div>
        <div>
          <span className="text-xs text-gray-500 mr-1.5">Savings rate</span>
          <span className={`text-lg font-bold ${TONE_TEXT[savingsRateTone]}`}>{savingsRateValue}</span>
          <span className="text-xs text-gray-400 ml-1.5">{savingsRateSubtitle}</span>
        </div>
        {avgMonthlySpend != null && (
          <div>
            <span className="text-xs text-gray-500 mr-1.5">Avg spend</span>
            <span className="text-lg font-bold text-gray-800">{formatEuro(avgMonthlySpend)}</span>
            <span className="text-xs text-gray-400 ml-1.5">/mo over {completeMonthsCount} mo</span>
          </div>
        )}
      </div>

      {runwayMonths != null && (
        <div className="mt-4">
          <div className="flex items-baseline justify-between mb-1">
            <span className="text-xs text-gray-500">
              Savings runway — free cash ÷ avg monthly spend
            </span>
            <span className={`text-sm font-bold ${
              runwayMonths >= RUNWAY_TARGET ? 'text-green-600' : runwayMonths >= 3 ? 'text-yellow-600' : 'text-red-600'
            }`}>
              {runwayMonths.toFixed(1)} months
            </span>
          </div>
          <div className="relative h-2.5 bg-gray-100 rounded-full overflow-hidden">
            <div className={`h-full rounded-full ${runwayColor}`} style={{ width: `${runwayPct}%` }} />
          </div>
          <p className="text-xs text-gray-400 mt-1">
            {runwayMonths >= RUNWAY_TARGET
              ? `Full ${RUNWAY_TARGET}-month emergency fund covered`
              : `${(RUNWAY_TARGET - runwayMonths).toFixed(1)} months short of the ${RUNWAY_TARGET}-month emergency-fund target`}
          </p>
        </div>
      )}
    </div>
  )
}
