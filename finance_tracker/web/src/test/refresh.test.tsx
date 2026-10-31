import { describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useRefresh } from '../lib/hooks'

// After a transaction changes, every view whose numbers can move is marked
// stale — including ones added later — while settings, the chat and market
// data are left alone.
describe('useRefresh', () => {
  it('refreshes ledger views, not settings or market data', () => {
    const qc = new QueryClient()
    for (const k of [['tax', 2026], ['close', '2026-10'], ['balance-table', 1], ['overview'], ['some-future-page'], ['prefs'], ['chat'], ['hist', 'VWCE', '1y']]) {
      qc.setQueryData(k, { x: 1 })
    }
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    const { result } = renderHook(() => useRefresh(), { wrapper })
    result.current()
    const stale = (k: unknown[]) => qc.getQueryState(k)?.isInvalidated
    for (const k of [['tax', 2026], ['close', '2026-10'], ['balance-table', 1], ['overview'], ['some-future-page']]) expect(stale(k), String(k)).toBe(true)
    for (const k of [['prefs'], ['chat'], ['hist', 'VWCE', '1y']]) expect(stale(k), String(k)).toBe(false)
  })
})
