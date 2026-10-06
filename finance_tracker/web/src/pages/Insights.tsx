import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, Tabs } from '../components/ui'
import { CashFlow } from './insights/CashFlow'
import { Spending } from './insights/Spending'
import { Trends } from './insights/Trends'
import { RecurringView } from './insights/Recurring'
import { FI } from './insights/FI'
import { ReviewTabs } from './insights/MonthReview'


const TABS = [
  { value: 'cashflow', label: 'Cash flow' }, { value: 'spending', label: 'Spending' }, { value: 'trends', label: 'Trends' },
  { value: 'recurring', label: 'Recurring' }, { value: 'review', label: 'Review' }, { value: 'fi', label: 'Independence' },
]

export default function Insights() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.split('/')[2] || 'cashflow'
  return (
    <div>
      <PageHeader title="Insights" actions={<AskCFO q={`Review my ${TABS.find((t) => t.value === tab)?.label.toLowerCase() ?? 'finances'}: what stands out, what changed, and one or two concrete actions.`} />} />
      <Tabs value={tab} onChange={(v) => nav(`/insights/${v}`)} tabs={TABS} />
      <Routes>
        <Route path="/" element={<CashFlow />} />
        <Route path="/cashflow" element={<CashFlow />} />
        <Route path="/spending" element={<Spending />} />
        <Route path="/trends" element={<Trends />} />
        <Route path="/recurring" element={<RecurringView />} />
        <Route path="/fi" element={<FI />} />
        <Route path="/review" element={<ReviewTabs />} />
      </Routes>
    </div>
  )
}
