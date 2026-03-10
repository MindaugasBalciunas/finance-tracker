import client from './client'
import type { StockTrade, CreateStockTradeInput, StockPortfolio } from '../types'

export const stocksApi = {
  listAll: () =>
    client.get<StockTrade[]>('/stocks').then((r) => r.data),

  create: (input: CreateStockTradeInput) =>
    client.post<StockTrade>('/stocks', input).then((r) => r.data),

  update: (id: number, input: Partial<CreateStockTradeInput>) =>
    client.put<StockTrade>(`/stocks/${id}`, input).then((r) => r.data),

  delete: (id: number) =>
    client.delete(`/stocks/${id}`),

  getPortfolio: () =>
    client.get<StockPortfolio>('/stocks/portfolio').then((r) => r.data),

  getPrice: (ticker: string) =>
    client.get<{ ticker: string; price: number }>(`/stocks/price/${ticker}`).then((r) => r.data),

  getHistory: (ticker: string, range: string) =>
    client.get<{ ticker: string; points: { date: string; close: number }[] }>(
      `/stocks/history/${ticker}?range=${range}`
    ).then((r) => r.data),
}
