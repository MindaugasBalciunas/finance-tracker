import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, SubNav, Tabs } from '../components/ui'
import { Overview } from './wealth/Overview'
import { Investments } from './wealth/Investments'
import { Loans } from './wealth/Loans'
import { BalanceHistory } from './wealth/History'
import { TaxView } from './insights/Tax'
import { FI } from './insights/FI'
import { Ownership } from './wealth/Ownership'
import { StressView } from './wealth/Stress'
import { Dips } from './wealth/Dips'
import { EmergencyFund } from './wealth/Emergency'
import { PlanSettings } from './plan/Settings'

// Route segment → the tab it sits under: the house and its mortgage on one
// page. The stress test keeps its own tab: behind a toggle under
// Independence the owner couldn't find it.
const PARENT: Record<string, string> = { loans: 'ownership', dips: 'investments', bets: 'investments', btc: 'investments' }

export default function Wealth() {
  const loc = useLocation()
  const nav = useNavigate()
  const seg = loc.pathname.split('/')[2] || 'overview'
  const tab = PARENT[seg] ?? seg
  return (
    <div>
      <PageHeader title="Wealth" actions={<AskCFO q="Review my balance sheet: allocation across cash, investments, pension, crypto and property, against my framework. What should I change?" />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'overview' ? '/wealth' : `/wealth/${v}`)}
        tabs={[{ value: 'overview', label: 'Net worth' }, { value: 'investments', label: 'Investments' }, { value: 'ownership', label: 'Property & loans' }, { value: 'fi', label: 'Independence' }, { value: 'stress', label: 'Stress test' }, { value: 'tax', label: 'Taxes' }, { value: 'history', label: 'History' }]} />
      {tab === 'investments' && (
        // The dip plan is about the same funds: a view of Investments, not a
        // tab of its own (owner's call, 2026-11-25). Its path stays /wealth/dips
        // so the bell's and the phone's links keep working.
        <SubNav value={seg} options={[{ value: 'investments', label: 'Positions', to: '/wealth/investments' }, { value: 'dips', label: 'Dip plan', to: '/wealth/dips' }, { value: 'bets', label: 'Bets', to: '/wealth/bets' }, { value: 'btc', label: 'BTC', to: '/wealth/btc' }]} />
      )}
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/investments" element={<Investments />} />
        <Route path="/dips" element={<Dips />} />
        <Route path="/bets" element={<Dips plan="bets" />} />
        <Route path="/btc" element={<Dips plan="crypto" />} />
        <Route path="/ownership" element={<PropertyAndLoans />} />
        <Route path="/loans" element={<PropertyAndLoans />} />
        <Route path="/tax" element={<TaxView />} />
        <Route path="/fi" element={<Independence />} />
        <Route path="/stress" element={<StressView />} />
        <Route path="/history" element={<BalanceHistory />} />
      </Routes>
    </div>
  )
}

function PropertyAndLoans() {
  return (
    <div className="space-y-6">
      <Ownership />
      <Loans />
    </div>
  )
}

/** Independence, then the emergency fund the plan holds the wish list to,
 *  then the inputs both are worked out from (moved here from Plan). */
function Independence() {
  return (
    <div className="space-y-4">
      <FI />
      <EmergencyFund />
      <PlanSettings section="fi" />
    </div>
  )
}
