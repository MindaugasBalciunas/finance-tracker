import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { ToastProvider } from '../components/ui'
import { TxEditorProvider } from '../components/TxEditor'
import { needsList, NewOrRecent } from '../pages/Home'
import type { Overview, Tx } from '../lib/types'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

const tx = (id: number, merchant: string): Tx => ({ id, date: '2026-11-23', kind: 'expense', amount: 7.9, account_id: 'swed', to_account_id: '', category: 'food.work_lunch', merchant, note: '', tags: [], source: 'bank' } as Tx)

const overview = (o: Partial<Overview>) => ({ actions: [], inbox_open: 0, recent: [], ...o }) as Overview

function wrap(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider><MemoryRouter><TxEditorProvider>{ui}</TxEditorProvider></MemoryRouter></ToastProvider>
    </QueryClientProvider>)
}

function stub(prefs: Record<string, unknown>, fresh: Tx[]) {
  const calls: { method: string; path: string; url: string; body?: Record<string, unknown> }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = url.replace(/^.*?api/, '').split('?')[0]
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method, path, url, body })
    const out = path === '/prefs' ? (method === 'PUT' ? { ...prefs, ...body } : prefs)
      : path === '/transactions' ? { items: fresh, total: fresh.length, income: 0, expense: 0, transfer: 0 }
        : ['/accounts', '/categories'].includes(path) ? [] : {}
    return new Response(JSON.stringify(out), { status: 200 })
  }))
  return calls
}

// The app shows where money stands; the banks move it. Top-ups stay in the
// payday card, so Home's "Needs you" leaves them out and keeps the rest in
// order, most urgent first.
it("keeps top-up suggestions out of Home's Needs you", () => {
  const list = needsList(overview({ actions: [
    { level: 'info', kind: 'emergency', text: 'Emergency fund at 2.7 of 3 months' },
    { level: 'info', kind: 'move', text: 'Move €320 from Swedbank savings to Swedbank', due: '2026-11-24' },
    { level: 'warn', kind: 'planned', text: "Artea €200 planned for 10 Nov hasn't happened", due: '2026-11-10' },
  ] }))
  expect(list.map((a) => a.kind)).toEqual(['planned', 'emergency'])
})

// Bank rows added since Home was last marked seen are listed so they can be
// annotated here; "Mark seen" moves the time forward.
it('lists rows added since you last looked and marks them seen', async () => {
  const calls = stub({ liquid_only: false, home_seen: '2026-11-23T06:00:00Z' }, [tx(2, 'Dinner House'), tx(3, 'Norfa')])
  wrap(<NewOrRecent o={overview({ recent: [tx(1, 'Old shop')] })} />)
  expect(await screen.findByText('New · 2')).toBeInTheDocument()
  expect(screen.getByText('Norfa')).toBeInTheDocument()
  expect(screen.queryByText('Old shop')).toBeNull()
  expect(calls.some((c) => c.path === '/transactions' && c.url.includes('created_after=2026-11-23T06%3A00%3A00Z'))).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: /Mark seen/ }))
  await waitFor(() => expect(calls.some((c) => c.method === 'PUT' && c.path === '/prefs' && c.body?.home_seen === 'now')).toBe(true))
})

// The first visit starts the clock instead of calling every row new.
it('starts the clock on the first visit and shows the recent rows', async () => {
  const calls = stub({ liquid_only: false }, [])
  wrap(<NewOrRecent o={overview({ recent: [tx(1, 'Old shop')] })} />)
  expect(await screen.findByText('Recent')).toBeInTheDocument()
  expect(screen.getByText('Old shop')).toBeInTheDocument()
  await waitFor(() => expect(calls.some((c) => c.method === 'PUT' && c.path === '/prefs' && c.body?.home_seen === 'now')).toBe(true))
  expect(calls.some((c) => c.url.includes('created_after=now'))).toBe(false)
})

// A save that keeps failing must not be retried in a loop.
it('tries to start the clock only once when saving fails', async () => {
  const calls: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = url.replace(/^.*?api/, '').split('?')[0]
    calls.push(`${init?.method ?? 'GET'} ${path}`)
    if (path === '/prefs' && init?.method === 'PUT') return new Response(JSON.stringify({ error: 'too many periods' }), { status: 400 })
    return new Response(JSON.stringify(path === '/prefs' ? { liquid_only: false } : ['/accounts', '/categories'].includes(path) ? [] : {}), { status: 200 })
  }))
  wrap(<NewOrRecent o={overview({ recent: [tx(1, 'Old shop')] })} />)
  await screen.findByText('Recent')
  await new Promise((r) => setTimeout(r, 300))
  expect(calls.filter((c) => c === 'PUT /prefs')).toHaveLength(1)
})
