import { describe, it, expect, vi, beforeEach } from 'vitest'
import client from './client'
import { transactionsApi } from './transactions'
import type { Transaction } from '../types'

vi.mock('./client', () => ({
  default: { get: vi.fn() },
}))

const mockedGet = vi.mocked(client.get)

function tx(id: number, date: string): Partial<Transaction> {
  return { id, date, type: 'expense', category: 'Food', amount: { value: 10, currency: 'EUR' } }
}

function page(data: Partial<Transaction>[], pageNum: number, total: number, pageSize: number) {
  return {
    data: {
      data,
      total,
      page: pageNum,
      page_size: pageSize,
      total_pages: Math.ceil(total / pageSize),
    },
  }
}

describe('transactionsApi.listAll', () => {
  beforeEach(() => {
    mockedGet.mockReset()
  })

  it('returns the single page when everything fits', async () => {
    mockedGet.mockResolvedValueOnce(page([tx(1, '2026-01-05')], 1, 1, 1000))

    const result = await transactionsApi.listAll({ type: 'expense' })

    expect(mockedGet).toHaveBeenCalledTimes(1)
    expect(result.data).toHaveLength(1)
  })

  it('fetches and concatenates every page when results exceed one page', async () => {
    const total = 2500
    mockedGet.mockImplementation(async (_url, config) => {
      const p = config?.params?.page as number
      const size = p === 3 ? 500 : 1000
      const rows = Array.from({ length: size }, (_, i) =>
        tx((p - 1) * 1000 + i + 1, '2025-06-01')
      )
      return page(rows, p, total, 1000)
    })

    const result = await transactionsApi.listAll({ type: 'expense' })

    expect(mockedGet).toHaveBeenCalledTimes(3)
    expect(result.data).toHaveLength(total)
    expect(result.data[0].id).toBe(1)
    expect(result.data[total - 1].id).toBe(total)
  })

  it('passes the date filter through to every page request', async () => {
    mockedGet.mockImplementation(async (_url, config) => {
      const p = config?.params?.page as number
      return page([tx(p, '2024-03-01')], p, 1500, 1000)
    })

    await transactionsApi.listAll({ type: 'expense', date_from: '2024-01-01' })

    for (const call of mockedGet.mock.calls) {
      expect(call[1]?.params).toMatchObject({ type: 'expense', date_from: '2024-01-01' })
    }
  })
})
