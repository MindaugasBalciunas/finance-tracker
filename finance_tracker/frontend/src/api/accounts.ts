import client from './client'
import type { Account, AccountGroup } from '../types'

export type AccountInput = { label?: string; group?: AccountGroup; archived?: boolean }

export const accountsApi = {
  list: () => client.get<Account[]>('/accounts').then((r) => r.data),
  create: (input: { label: string; group: AccountGroup }) =>
    client.post<Account>('/accounts', input).then((r) => r.data),
  update: (id: number, input: AccountInput) =>
    client.put<Account>(`/accounts/${id}`, input).then((r) => r.data),
}
