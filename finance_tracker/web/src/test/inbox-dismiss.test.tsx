import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { ToastProvider } from '../components/ui'
import { Inbox } from '../pages/ledger/Inbox'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

// The shape of the four Revolut rows the owner couldn't dismiss on prod
// (Nov 2026): booked, verdict "new", no match, category not a guess and no
// verdict note — so the row showed no Dismiss at all.
const row = (id: number, amount: number) => ({
  id, bank_account_id: 6, external_id: `eb:6:${id}`, raw_payee: '', raw_details: 'To investment account', raw_currency: 'EUR',
  booking_date: '2026-11-21', pending: false, date: '2026-11-21', kind: 'expense', amount, account_id: 'revolut', to_account_id: '',
  category: 'finance.bank_fees', merchant: '', note: 'To investment account', tags: [], edited: false, guessed: false,
  verdict: 'new', verdict_note: '', state: 'open', first_seen_at: '2026-11-21T04:47:58Z',
})

function setup() {
  let open = [row(460, 0.07), row(461, 4)]
  const calls: { method: string; path: string; body?: { ids: number[] } }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = url.replace(/^.*?api/, '').split('?')[0]
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method, path, body })
    if (path === '/bank/inbox/dismiss') {
      open = open.filter((r) => !body.ids.includes(r.id))
      return new Response(JSON.stringify({ changed: body.ids.length }), { status: 200 })
    }
    const out = path === '/bank/inbox' ? (url.includes('state=') && !url.includes('state=open') ? [] : open)
      : path === '/bank/connections' ? [{ id: 12, status: 'authorized', accounts: [] }]
        : path === '/categories' ? [{ id: 'finance.bank_fees', parent: 'finance', name: 'Bank fees', kind: 'expense', essential: true, sort: 1, archived: false }]
          : ['/accounts', '/merchants', '/tags'].includes(path) ? [] : {}
    return new Response(JSON.stringify(out), { status: 200 })
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider><MemoryRouter><Inbox /></MemoryRouter></ToastProvider>
    </QueryClientProvider>)
  return calls
}

it('lets you dismiss a clean bank row that has no match, guess or note', async () => {
  const calls = setup()
  const buttons = await screen.findAllByRole('button', { name: 'Dismiss To investment account' }, { timeout: 5000 })
  expect(buttons).toHaveLength(2)
  fireEvent.click(buttons[0])
  await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.path === '/bank/inbox/dismiss')).toBe(true))
  expect(calls.find((c) => c.path === '/bank/inbox/dismiss')!.body).toEqual({ ids: [460] })
  await waitFor(() => expect(screen.getAllByRole('button', { name: 'Dismiss To investment account' })).toHaveLength(1))
})

it('lets you dismiss from the bank transaction sheet too', async () => {
  const calls = setup()
  await screen.findAllByRole('button', { name: 'Dismiss To investment account' }, { timeout: 5000 })
  fireEvent.click(screen.getAllByRole('button', { name: /To investment account.*4[.,]00/ })[0])
  await screen.findByText('Bank transaction')
  fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
  await waitFor(() => expect(calls.find((c) => c.path === '/bank/inbox/dismiss')?.body).toEqual({ ids: [461] }))
})
