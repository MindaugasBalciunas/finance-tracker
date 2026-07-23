import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import CategoryTransactionsModal from './CategoryTransactionsModal'
import { transactionsApi } from '../../api/transactions'
import type { Transaction } from '../../types'

vi.mock('../../api/transactions', () => ({
  transactionsApi: {
    listAll: vi.fn(),
  },
}))

const mockedApi = vi.mocked(transactionsApi)

function tx(id: number, date: string, value: number, comment: string): Transaction {
  return {
    id,
    date,
    type: 'expense',
    category: 'Food',
    amount: { value, currency: 'EUR' },
    comment,
    created_at: '',
    updated_at: '',
  } as Transaction
}

function renderModal(onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <CategoryTransactionsModal
        category="Food"
        type="expense"
        dateRange={{ date_from: '2026-01-01' }}
        onClose={onClose}
      />
    </QueryClientProvider>
  )
  return onClose
}

describe('CategoryTransactionsModal', () => {
  beforeEach(() => {
    mockedApi.listAll.mockReset()
  })

  it('lists the transactions with count and total', async () => {
    mockedApi.listAll.mockResolvedValue({
      data: [
        tx(1, '2026-02-10', 25.5, 'Maxima groceries'),
        tx(2, '2026-01-05', 74.5, 'Lidl weekly shop'),
      ],
      total: 2,
      page: 1,
      page_size: 1000,
      total_pages: 1,
    })

    renderModal()

    await waitFor(() => {
      expect(screen.getByText('Maxima groceries')).toBeInTheDocument()
    })
    expect(screen.getByText('Lidl weekly shop')).toBeInTheDocument()
    expect(screen.getByText('2 transactions')).toBeInTheDocument()
    expect(mockedApi.listAll).toHaveBeenCalledWith({
      category: 'Food',
      type: 'expense',
      date_from: '2026-01-01',
    })
  })

  it('shows an empty state when nothing matches', async () => {
    mockedApi.listAll.mockResolvedValue({
      data: [],
      total: 0,
      page: 1,
      page_size: 1000,
      total_pages: 0,
    })

    renderModal()

    await waitFor(() => {
      expect(screen.getByText('No transactions in this period.')).toBeInTheDocument()
    })
  })

  it('closes on close button and on Escape', async () => {
    mockedApi.listAll.mockResolvedValue({
      data: [tx(1, '2026-02-10', 10, 'Coffee')],
      total: 1,
      page: 1,
      page_size: 1000,
      total_pages: 1,
    })

    const onClose = renderModal()
    await waitFor(() => {
      expect(screen.getByText('Coffee')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByLabelText('Close'))
    expect(onClose).toHaveBeenCalledTimes(1)

    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(2)
  })
})
