import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, Tabs } from '../components/ui'
import { CashFlow } from './insights/CashFlow'
import { Spending } from './insights/Spending'
import { Trends } from './insights/Trends'
import { RecurringView } from './insights/Recurring'
import { FI } from './insights/FI'
import { ReviewTabs } from './insights/MonthReview'
import { SummaryView } from './insights/Summary'
import { TaxView } from './insights/Tax'
import { FunView } from './insights/Fun'

const TABS = [
  { value: 'summary', label: 'Summary' }, { value: 'cashflow', label: 'Cash flow' }, { value: 'spending', label: 'Spending' }, { value: 'trends', label: 'Trends' },
  { value: 'recurring', label: 'Recurring' }, { value: 'review', label: 'Review' }, { value: 'tax', label: 'Taxes' }, { value: 'fi', label: 'Independence' }, { value: 'fun', label: 'Fun facts' },
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
        <Route path="/recurring" element={<RecurringView />} />
        <Route path="/fi" element={<FI />} />
        <Route path="/review" element={<ReviewTabs />} />
        <Route path="/tax" element={<TaxView />} />
        <Route path="/fun" element={<FunView />} />
      </Routes>
    </div>
  )
}
