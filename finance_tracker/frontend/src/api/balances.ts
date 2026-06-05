import client from './client'
import type {
  Balance,
  CreateBalanceInput,
  BalanceTrend,
  AccountAllocation,
  BalanceFilter,
} from '../types'

export const balancesApi = {
  list: async (filter: BalanceFilter = {}, btcPrice?: number): Promise<Balance[]> => {
    const params = btcPrice ? { ...filter, btc_price: btcPrice } : filter
    const { data } = await client.get<Balance[]>('/balances', { params })
    return data
  },

  getById: async (id: number): Promise<Balance> => {
    const { data } = await client.get<Balance>(`/balances/${id}`)
    return data
  },

  create: async (input: CreateBalanceInput): Promise<Balance> => {
    const { data } = await client.post<Balance>('/balances', input)
    return data
  },

  update: async (id: number, input: CreateBalanceInput): Promise<Balance> => {
    const { data } = await client.put<Balance>(`/balances/${id}`, input)
    return data
  },

  delete: async (id: number): Promise<void> => {
    await client.delete(`/balances/${id}`)
  },

  getLatest: async (btcPrice?: number): Promise<Balance> => {
    const params = btcPrice ? { btc_price: btcPrice } : undefined
    const { data } = await client.get<Balance>('/balances/latest', { params })
    return data
  },

  getTrend: async (filter: BalanceFilter = {}): Promise<BalanceTrend> => {
    const { data } = await client.get<BalanceTrend>('/balances/trend', { params: filter })
    return data
  },

  getAllocation: async (): Promise<AccountAllocation[]> => {
    const { data } = await client.get<AccountAllocation[]>('/balances/allocation')
    return data
  },

  getProjected: async (btcPrice?: number): Promise<Balance> => {
    const params = btcPrice ? { btc_price: btcPrice } : undefined
    const { data } = await client.get<Balance>('/balances/projected', { params })
    return data
  },
}
