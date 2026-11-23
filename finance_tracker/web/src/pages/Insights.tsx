import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, SubNav, Tabs } from '../components/ui'
import { Spending } from './insights/Spending'
import { Trends } from './insights/Trends'
import { ReviewTabs } from './insights/MonthReview'
import { SummaryView } from './insights/Summary'
import { FunView } from './insights/Fun'
import { DriftView } from './insights/Drift'
import { MerchantsView } from './insights/Merchants'
import { OptionalView } from './insights/Optional'

// Looking back. Recurring bills live in Plan (they are what's coming);
// taxes and independence in Wealth (where the money ends up). Views that
// answer the same question share a tab, switched with a toggle; each keeps
// its own route so old links and notifications still open it.
const TABS = [
  { value: 'summary', label: 'Summary' }, { value: 'review', label: 'Review' }, { value: 'spending', label: 'Spending' },
  { value: 'habits', label: 'Habits' }, { value: 'fun', label: 'Fun facts' },
]
// Route segment → the tab it sits under.
const PARENT: Record<string, string> = { cashflow: 'review', merchants: 'spending', optional: 'spending', trends: 'habits', drift: 'habits' }
const SPENDING = [{ value: 'spending', label: 'Categories', to: '/insights/spending' }, { value: 'merchants', label: 'Merchants', to: '/insights/merchants' }, { value: 'optional', label: 'Optional', to: '/insights/optional' }]
const HABITS = [{ value: 'trends', label: 'Trends', to: '/insights/trends' }, { value: 'drift', label: 'Drift', to: '/insights/drift' }]

export default function Insights() {
  const loc = useLocation()
  const nav = useNavigate()
  const seg = loc.pathname.split('/')[2] || 'summary'
  const tab = PARENT[seg] ?? seg
  const name = ({ cashflow: 'cash flow', merchants: 'merchants', trends: 'spending trends', drift: 'lifestyle creep' } as Record<string, string>)[seg] ?? TABS.find((t) => t.value === tab)?.label.toLowerCase() ?? 'finances'
  // Optional spending asks the question the page is for.
  const ask = seg === 'optional'
    ? 'Review my optional spending: where is money leaking, and which of it looks like quality of life rather than reckless spending? One or two concrete actions.'
    : `Review my ${name}: what stands out, what changed, and one or two concrete actions.`
  return (
    <div>
      <PageHeader title="Insights" actions={<AskCFO q={ask} />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'habits' ? '/insights/trends' : `/insights/${v}`)} tabs={TABS} />
      {tab === 'spending' && <SubNav value={seg} options={SPENDING} />}
      {tab === 'habits' && <SubNav value={seg === 'habits' ? 'trends' : seg} options={HABITS} />}
      <Routes>
        <Route path="/" element={<SummaryView />} />
        <Route path="/summary" element={<SummaryView />} />
        <Route path="/review" element={<ReviewTabs key="review" />} />
        <Route path="/cashflow" element={<ReviewTabs key="cashflow" mode="cashflow" />} />
        <Route path="/spending" element={<Spending />} />
        <Route path="/merchants" element={<MerchantsView />} />
        <Route path="/optional" element={<OptionalView />} />
        <Route path="/habits" element={<Navigate to="/insights/trends" replace />} />
        <Route path="/trends" element={<Trends />} />
        <Route path="/drift" element={<DriftView />} />
        <Route path="/fun" element={<FunView />} />
        {/* moved: old links and saved notifications */}
        <Route path="/recurring" element={<Navigate to="/plan/recurring" replace />} />
        <Route path="/fi" element={<Navigate to="/wealth/fi" replace />} />
        <Route path="/tax" element={<Navigate to="/wealth/tax" replace />} />
      </Routes>
    </div>
  )
}
