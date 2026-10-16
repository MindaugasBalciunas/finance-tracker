import { useEffect, useState } from 'react'
import { Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, Tabs } from '../components/ui'
import { Icon, IconTile } from '../components/Icon'
import { CashFlow } from './insights/CashFlow'
import { Spending } from './insights/Spending'
import { Trends } from './insights/Trends'
import { RecurringView } from './insights/Recurring'
import { FI } from './insights/FI'
import { ReviewTabs } from './insights/MonthReview'
import { SummaryView } from './insights/Summary'
import { TaxView } from './insights/Tax'

const TABS = [
  { value: 'summary', label: 'Summary' }, { value: 'cashflow', label: 'Cash flow' }, { value: 'spending', label: 'Spending' }, { value: 'trends', label: 'Trends' },
  { value: 'recurring', label: 'Recurring' }, { value: 'review', label: 'Review' }, { value: 'tax', label: 'Taxes' }, { value: 'fi', label: 'Independence' },
]
// Phones show the four most used and "More" — seven tabs scroll off-screen.
const PHONE = ['summary', 'cashflow', 'spending', 'recurring']
const MORE = [
  { value: 'trends', label: 'Trends', icon: 'chart', sub: 'Monthly spending by category over months or years' },
  { value: 'review', label: 'Review', icon: 'calendar', sub: 'A finished month or year: what happened and why' },
  { value: 'tax', label: 'Taxes', icon: 'percent', sub: 'The income declaration ahead: what you will pay or get back' },
  { value: 'fi', label: 'Independence', icon: 'trend', sub: 'Financial independence: target, progress, years to go' },
]

function useWide() {
  const q = '(min-width: 640px)'
  const [wide, setWide] = useState(() => (typeof window !== 'undefined' && window.matchMedia ? window.matchMedia(q).matches : true))
  useEffect(() => {
    if (!window.matchMedia) return
    const m = window.matchMedia(q)
    const on = () => setWide(m.matches)
    m.addEventListener?.('change', on)
    return () => m.removeEventListener?.('change', on)
  }, [])
  return wide
}

export default function Insights() {
  const loc = useLocation()
  const nav = useNavigate()
  const wide = useWide()
  const tab = loc.pathname.split('/')[2] || 'summary'
  // On a phone a tab picked from "More" takes the More slot, so the active tab is always visible.
  const extra = MORE.find((m) => m.value === tab)
  const tabs = wide ? TABS : [...TABS.filter((t) => PHONE.includes(t.value)), extra ? { value: extra.value, label: extra.label } : { value: 'more', label: 'More' }]
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
        <Route path="/more" element={<More />} />
      </Routes>
    </div>
  )
}

function More() {
  return (
    <div className="space-y-2">
      {MORE.map((m) => (
        <Link key={m.value} to={`/insights/${m.value}`} className="card flex items-center gap-3 p-3 hover:bg-sunken/40">
          <IconTile name={m.icon} color="var(--s1)" size={32} />
          <span className="min-w-0 flex-1"><span className="block font-medium">{m.label}</span><span className="block text-xs text-muted">{m.sub}</span></span>
          <Icon name="chevronR" size={16} className="text-muted" />
        </Link>
      ))}
    </div>
  )
}
