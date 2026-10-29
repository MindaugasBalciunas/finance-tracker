import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, Tabs } from '../components/ui'
import { CashFlow } from './insights/CashFlow'
import { Spending } from './insights/Spending'
import { Trends } from './insights/Trends'
import { ReviewTabs } from './insights/MonthReview'
import { SummaryView } from './insights/Summary'
import { FunView } from './insights/Fun'
import { DriftView } from './insights/Drift'

// Looking back. Recurring bills live in Plan (they are what's coming);
// taxes and independence in Wealth (where the money ends up).
const TABS = [
  { value: 'summary', label: 'Summary' }, { value: 'review', label: 'Review' }, { value: 'cashflow', label: 'Cash flow' }, { value: 'spending', label: 'Spending' },
  { value: 'trends', label: 'Trends' }, { value: 'drift', label: 'Drift' }, { value: 'fun', label: 'Fun facts' },
]
export default function Insights() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.split('/')[2] || 'summary'
  // Every tab, scrolling sideways on a phone.
  const tabs = TABS
  const name = TABS.find((t) => t.value === tab)?.label.toLowerCase() ?? 'finances'
  return (
    <div>
      <PageHeader title="Insights" actions={<AskCFO q={`Review my ${name}: what stands out, what changed, and one or two concrete actions.`} />} />
      <Tabs value={tab} onChange={(v) => nav(`/insights/${v}`)} tabs={tabs} />
      <Routes>
        <Route path="/" element={<SummaryView />} />
        <Route path="/summary" element={<SummaryView />} />
        <Route path="/cashflow" element={<CashFlow />} />
        <Route path="/spending" element={<Spending />} />
        <Route path="/trends" element={<Trends />} />
        <Route path="/review" element={<ReviewTabs />} />
        {/* moved: old links and saved notifications */}
        <Route path="/recurring" element={<Navigate to="/plan/recurring" replace />} />
        <Route path="/fi" element={<Navigate to="/wealth/fi" replace />} />
        <Route path="/tax" element={<Navigate to="/wealth/tax" replace />} />
        <Route path="/drift" element={<DriftView />} />
        <Route path="/fun" element={<FunView />} />
      </Routes>
    </div>
  )
}
