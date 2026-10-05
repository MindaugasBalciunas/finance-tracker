import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AskCFO, PageHeader, Tabs } from '../components/ui'
import { Overview } from './wealth/Overview'
import { Investments } from './wealth/Investments'
import { Loans } from './wealth/Loans'
import { BalanceHistory } from './wealth/History'

export default function Wealth() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.split('/')[2] || 'overview'
  return (
    <div>
      <PageHeader title="Wealth" actions={<AskCFO q="Review my balance sheet: allocation across cash, investments, pension, crypto and property, against my framework. What should I change?" />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'overview' ? '/wealth' : `/wealth/${v}`)}
        tabs={[{ value: 'overview', label: 'Net worth' }, { value: 'investments', label: 'Investments' }, { value: 'loans', label: 'Loans' }, { value: 'history', label: 'History' }]} />
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/investments" element={<Investments />} />
        <Route path="/loans" element={<Loans />} />
        <Route path="/history" element={<BalanceHistory />} />
      </Routes>
    </div>
  )
}
