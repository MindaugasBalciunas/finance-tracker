import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, Tabs } from '../components/ui'
import { Overview } from './wealth/Overview'
import { Investments } from './wealth/Investments'
import { Loans } from './wealth/Loans'
import { BalanceHistory } from './wealth/History'
import { TaxView } from './insights/Tax'
import { FI } from './insights/FI'
import { Ownership } from './wealth/Ownership'
import { StressView } from './wealth/Stress'
import { EmergencyFund } from './wealth/Emergency'
import { PlanSettings } from './plan/Settings'

// Route segment → the tab it sits under: the house and its mortgage on one
// page. The stress test keeps its own tab: behind a toggle under
// Independence the owner couldn't find it.
const PARENT: Record<string, string> = { loans: 'ownership' }

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
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/investments" element={<Investments />} />
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
