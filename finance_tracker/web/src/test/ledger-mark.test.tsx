import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ToastProvider } from '../components/ui'
import { TxEditorProvider, useTxEditor } from '../components/TxEditor'
import Ledger from '../pages/Ledger'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

const tx = (o: { id: number; need?: string; [k: string]: unknown }) => ({ date: '2026-11-06', kind: 'expense', amount: 45, account_id: 'swed', tags: [], note: '', to_account_id: '', source: 'manual', ...o })

/** The Ledger over a fake server that remembers marks. */
function setup() {
  const calls: { method: string; path: string; body?: { need: string } }[] = []
  const items = [
    tx({ id: 1, category: 'food.restaurants', merchant: 'Bistro' }),
    tx({ id: 2, category: 'food.groceries', merchant: 'Lidl', need: 'optional' }),
    tx({ id: 3, category: 'housing.loan', merchant: 'Bank', fixed: true }),
    tx({ id: 4, kind: 'income', category: 'salary', merchant: 'Employer' }),
  ]
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const path = url.replace(/^.*?api/, '').split('?')[0]
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method, path, body })
    const m = path.match(/^\/transactions\/(\d+)\/need$/)
    if (path === '/transactions' && method === 'POST') return new Response(JSON.stringify({ transaction: { ...body, id: 9, need: '' } }), { status: 200 })
    if (m && method === 'PUT') {
      if (m[1] === '9') return new Response(JSON.stringify({ id: 9, need: body.need }), { status: 200 })
      const row = items.find((t) => t.id === Number(m[1]))!
      row.need = body.need
      return new Response(JSON.stringify(row), { status: 200 })
    }
    const out = path === '/transactions' ? { items, total: items.length, income: 0, expense: 135, transfer: 0 }
      : path === '/categories' ? [{ id: 'food.groceries', parent: 'food', name: 'Groceries', kind: 'expense', essential: true, sort: 1, archived: false },
        { id: 'food.restaurants', parent: 'food', name: 'Restaurants', kind: 'expense', essential: false, sort: 2, archived: false }]
        : path === '/tags/suggest' ? { usual: [], frequent: [] }
          : path === '/categories/suggest' ? { merchant: [], recent: [] }
            : /^\/transactions\/\d+\/recurring$/.test(path) ? { recurring_id: 0, match_id: 0, marked: false, items: [] }
              : /^\/transactions\/\d+$/.test(path) ? (method === 'PUT' ? items.find((t) => t.id === Number(path.split('/')[2])) : { transaction: items.find((t) => t.id === Number(path.split('/')[2])), parts: [] })
                : ['/accounts', '/merchants', '/tags', '/notes/suggest', '/rules'].includes(path) ? [] : {}
    return new Response(JSON.stringify(out), { status: 200 })
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/ledger']}>
          <TxEditorProvider><AddLunch /><Routes><Route path="/ledger/*" element={<Ledger />} /></Routes></TxEditorProvider>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>)
  return calls
}

/** Opens the add form with a lunch filled in, as a quick action would. */
function AddLunch() {
  const editor = useTxEditor()
  return <button onClick={() => editor.open({ kind: 'expense', category: 'food.restaurants', amount: 12, merchant: 'Canteen' })}>add lunch</button>
}

const swipe = (el: Element, from: [number, number], to: [number, number]) => {
  fireEvent.touchStart(el, { touches: [{ clientX: from[0], clientY: from[1] }] })
  for (let i = 1; i <= 4; i++) fireEvent.touchMove(el, { touches: [{ clientX: from[0] + ((to[0] - from[0]) * i) / 4, clientY: from[1] + ((to[1] - from[1]) * i) / 4 }] })
  fireEvent.touchEnd(el)
}

// Each free expense shows where it counts: dashed from its category, solid
// when marked. Obligations and income carry no pill.
it('shows must or optional on each free expense and nothing on the rest', async () => {
  setup()
  // The Ledger's first render takes ~3 s when the suite runs on a busy machine; the default 1 s wait failed there.
  expect(await screen.findByRole('button', { name: 'Optional, from its category. Change' }, { timeout: 5000 })).toBeInTheDocument() // Bistro: restaurants aren't essential
  expect(screen.getByRole('button', { name: 'Optional. Change' })).toBeInTheDocument() // Lidl: marked, though groceries are essential
  expect(screen.getAllByRole('button', { name: /Change$/ })).toHaveLength(2)
})

// A swipe to the left reveals Must / Optional; choosing one saves the mark,
// the pill turns solid, and the list is asked again (so Insights and the
// card follow). The swipe never opens the editor.
it('marks a payment a must with a swipe', async () => {
  const calls = setup()
  const row = (await screen.findByText('Bistro')).closest('[style*="touch-action"]')!
  expect(screen.queryByRole('button', { name: 'Mark a must' })).not.toBeInTheDocument()
  swipe(row, [300, 100], [180, 104])
  fireEvent.click(screen.getByText('Bistro')) // the click a swipe may still send
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  const before = calls.filter((c) => c.path === '/transactions').length
  fireEvent.click(screen.getByRole('button', { name: 'Mark a must' }))
  await waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ path: '/transactions/1/need', body: { need: 'must' } }))
  expect(await screen.findByRole('button', { name: 'Must. Change' })).toBeInTheDocument()
  await waitFor(() => expect(calls.filter((c) => c.path === '/transactions').length).toBeGreaterThan(before))
  expect(screen.queryByRole('button', { name: 'Mark a must' })).not.toBeInTheDocument() // closed again
})

// Scrolling the page over a row, or a short nudge, reveals nothing.
it('leaves vertical scrolling and small nudges alone', async () => {
  setup()
  const row = (await screen.findByText('Bistro')).closest('[style*="touch-action"]')!
  swipe(row, [300, 100], [280, 220])
  expect(screen.queryByRole('button', { name: 'Mark a must' })).not.toBeInTheDocument()
  swipe(row, [300, 100], [270, 100])
  expect(screen.queryByRole('button', { name: 'Mark a must' })).not.toBeInTheDocument()
  swipe(row, [100, 100], [260, 100]) // to the right on a closed row
  expect(screen.queryByRole('button', { name: 'Mark a must' })).not.toBeInTheDocument()
})

// Without a touch screen: the pill opens the same actions; a marked row also
// offers Clear, which sends the payment back to its category.
it('opens the actions from the pill and clears a mark', async () => {
  const calls = setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Optional. Change' }))
  expect(screen.getByRole('button', { name: 'Mark optional' })).toHaveAttribute('aria-pressed', 'true')
  fireEvent.click(screen.getByRole('button', { name: 'Clear the mark, back to its category' }))
  await waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ path: '/transactions/2/need', body: { need: '' } }))
  expect(await screen.findByRole('button', { name: 'Must, from its category. Change' })).toBeInTheDocument()
  // Escape closes an open row.
  fireEvent.click(screen.getByRole('button', { name: 'Optional, from its category. Change' }))
  expect(screen.getByRole('button', { name: 'Mark a must' })).toBeInTheDocument()
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(screen.queryByRole('button', { name: 'Mark a must' })).not.toBeInTheDocument()
})

// The same choice in the edit form: By category (saying what that is), Must
// or Optional. Saving sends the edit, then the mark on its own.
it('marks a payment from the edit form', async () => {
  const calls = setup()
  fireEvent.click(await screen.findByText('Bistro'))
  expect(await screen.findByRole('button', { name: 'By category · Optional' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Must' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(calls.find((c) => c.method === 'PUT' && c.path === '/transactions/1/need')).toMatchObject({ body: { need: 'must' } }))
  const puts = calls.filter((c) => c.method === 'PUT').map((c) => c.path)
  expect(puts.indexOf('/transactions/1')).toBeLessThan(puts.indexOf('/transactions/1/need'))
})

// Saving without touching the choice sends no mark at all.
it('leaves the mark alone when the form did not change it', async () => {
  const calls = setup()
  fireEvent.click(await screen.findByText('Lidl'))
  fireEvent.click(await screen.findByRole('button', { name: 'Save' }))
  await waitFor(() => expect(calls.some((c) => c.method === 'PUT' && c.path === '/transactions/2')).toBe(true))
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument())
  expect(calls.some((c) => c.path.endsWith('/need'))).toBe(false)
})

// Deciding at entry: a new expense added as a Must is created, then marked.
it('marks a new payment a must when adding it', async () => {
  const calls = setup()
  await screen.findByText('Bistro')
  fireEvent.click(screen.getByText('add lunch'))
  fireEvent.click(await screen.findByRole('button', { name: 'Must' }))
  const adds = screen.getAllByRole('button', { name: 'Add' })
  fireEvent.click(adds[adds.length - 1]) // the form's own, after the Ledger's
  await waitFor(() => expect(calls.find((c) => c.path === '/transactions/9/need')).toMatchObject({ method: 'PUT', body: { need: 'must' } }))
  expect(calls.findIndex((c) => c.method === 'POST' && c.path === '/transactions')).toBeLessThan(calls.findIndex((c) => c.path === '/transactions/9/need'))
})
