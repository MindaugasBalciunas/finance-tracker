import { lazy, Suspense } from 'react'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/layout/Layout'
import LoadingSpinner from './components/ui/LoadingSpinner'
import AuthGate from './components/ui/AuthGate'
import { DateRangeProvider } from './context/DateRangeContext'

// Each page (and the charts only it uses) loads as its own chunk, so first
// paint doesn't wait for Recharts or the datepicker.
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Transactions = lazy(() => import('./pages/Transactions'))
const Balances = lazy(() => import('./pages/Balances'))
const Reports = lazy(() => import('./pages/Reports'))
const Stocks = lazy(() => import('./pages/Stocks'))
const Assets = lazy(() => import('./pages/Assets'))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Data only changes through mutations in this app, and those invalidate
      // explicitly — refetching on tab focus is pure network chatter.
      staleTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthGate>
      <DateRangeProvider>
        <HashRouter>
          <Routes>
            <Route path="/" element={<Layout />}>
              <Route index element={<Suspense fallback={<LoadingSpinner />}><Dashboard /></Suspense>} />
              <Route path="transactions" element={<Suspense fallback={<LoadingSpinner />}><Transactions /></Suspense>} />
              <Route path="balances" element={<Suspense fallback={<LoadingSpinner />}><Balances /></Suspense>} />
              <Route path="stocks" element={<Suspense fallback={<LoadingSpinner />}><Stocks /></Suspense>} />
              <Route path="assets" element={<Suspense fallback={<LoadingSpinner />}><Assets /></Suspense>} />
              <Route path="reports" element={<Suspense fallback={<LoadingSpinner />}><Reports /></Suspense>} />
            </Route>
          </Routes>
        </HashRouter>
      </DateRangeProvider>
      </AuthGate>
    </QueryClientProvider>
  )
}
