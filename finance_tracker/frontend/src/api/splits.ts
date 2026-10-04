import client from './client'
import type { Transaction } from '../types'

export type SplitPart = {
  amount: number
  category?: string
  labels?: string
  comment?: string
  // Files the part as money this person owes you.
  owed_by?: string
}

export type OwedPerson = {
  person: string
  name: string
  lent: number
  repaid: number
  outstanding: number
  rows: { id: number; date: string; type: string; amount: number; comment: string }[]
}

export const splitsApi = {
  split: (id: number, parts: SplitPart[]) =>
    client.post<{ parts: Transaction[] }>(`/transactions/${id}/split`, { parts }).then((r) => r.data),
  unsplit: (id: number) => client.post<Transaction>(`/transactions/${id}/unsplit`).then((r) => r.data),
  owed: () => client.get<OwedPerson[]>('/transactions/owed').then((r) => r.data),
  repayment: (id: number, person: string) =>
    client.post<Transaction>(`/transactions/${id}/repayment`, { person }).then((r) => r.data),
}
