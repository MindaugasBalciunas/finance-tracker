import { describe, expect, it } from 'vitest'
import type { Account, Balance } from '../types'
import { freeCash, investments, withAddedAccounts } from './balanceGroups'

const base = {
  id: 1, date: '2026-10-01', is_auto: false, total: 0, seb: 10, swed: 20, swed_etf: 5, seb_pen: 0, luminor: 0,
  art: 0, cash: 0, rev_m: 0, rev_r: 0, r_btc: 0, m_btc: 0, btc_price: 0, rev_stocks: 0, ibkr_stocks: 0,
  created_at: '', updated_at: '',
} as Balance

const paysera: Account = { id: 20, key: 'acc_paysera', label: 'Paysera', group: 'cash', institution: 'Paysera', builtin: false, archived: false, sort_order: 100 }
const house: Account = { id: 21, key: 'acc_house', label: 'House fund', group: 'other', institution: '', builtin: false, archived: false, sort_order: 100 }

describe('added accounts', () => {
  it('count in their group total via extra_groups', () => {
    const b = { ...base, extra: { acc_paysera: 7 }, extra_groups: { cash: 7 } }
    expect(freeCash(b)).toBe(37)
    expect(investments(b)).toBe(5)
  })

  it('a snapshot without extras reads as before', () => {
    expect(freeCash(base)).toBe(30)
  })

  it('appear in their group breakdown and in other', () => {
    const { groups, other } = withAddedAccounts([paysera, house])
    const cash = groups.find((g) => g.group === 'cash')!
    const row = cash.accounts.find((a) => a.key === 'acc_paysera')!
    expect(row.label).toBe('Paysera')
    expect(row.value({ ...base, extra: { acc_paysera: 7 } })).toBe(7)
    expect(row.value(base)).toBe(0)
    expect(other.map((a) => a.key)).toContain('acc_house')
  })

  it('server groups win once present — a moved built-in moves its money', () => {
    const b = { ...base, groups: { cash: 35, investments: 0 } }
    expect(freeCash(b)).toBe(35)
    expect(investments(b)).toBe(0)
  })

  it('a moved built-in shows under its new group', () => {
    const { groups } = withAddedAccounts([], undefined, (k) => (k === 'swed_etf' ? 'cash' : undefined))
    const cash = groups.find((g) => g.group === 'cash')!.accounts.map((a) => a.key)
    const inv = groups.find((g) => g.group === 'investments')!.accounts.map((a) => a.key)
    expect(cash).toContain('swed_etf')
    expect(inv).not.toContain('swed_etf')
  })
})
