import { describe, expect, it } from 'vitest'
import type { Account } from '../types'
import { buildBankSections } from './accountLayout'

const acc = (key: string, label: string, group: Account['group'], institution: string, builtin = true, extra: Partial<Account> = {}): Account =>
  ({ id: 0, key, label, group, institution, builtin, archived: false, sort_order: builtin ? 0 : 100, ...extra })

const name = (k: string, f?: string) => f ?? k

describe('buildBankSections', () => {
  const accounts = [
    acc('seb', 'SEB', 'cash', 'SEB'), acc('seb_pen', 'SEB Pension', 'pensions', 'SEB'),
    acc('swed', 'Swedbank', 'cash', 'Swedbank'), acc('swed_etf', 'Swed ETF', 'investments', 'Swedbank'),
    acc('rev_r', 'Revolut ETF', 'investments', 'Revolut'), acc('cash', 'Cash', 'cash', 'Cash'),
    acc('acc_seb_savings', 'SEB savings', 'cash', 'SEB', false),
    acc('acc_wallet', 'Wallet', 'cash', 'Paysera', false),
    acc('acc_old', 'Old', 'cash', '', false, { archived: true }),
  ]
  const sections = buildBankSections(accounts, name)
  const banks = sections.map((s) => s.institution)

  it('orders banks familiar-first, others alphabetically, cash and unassigned last', () => {
    expect(banks.slice(0, 3)).toEqual(['SEB', 'Swedbank', 'Revolut'])
    expect(banks.indexOf('Paysera')).toBeGreaterThan(banks.indexOf('Luminor'))
    expect(banks[banks.length - 2]).toBe('Cash')
    expect(banks[banks.length - 1]).toBe('')
  })

  it('puts added accounts under their bank, cash before pensions', () => {
    const seb = sections.find((s) => s.institution === 'SEB')!.fields.map((f) => f.key)
    expect(seb).toEqual(['seb', 'acc_seb_savings', 'seb_pen'])
  })

  it('BTC holdings sit under Revolut as crypto, after the EUR accounts', () => {
    const rev = sections.find((s) => s.institution === 'Revolut')!.fields
    expect(rev.map((f) => f.key).slice(-2)).toEqual(['r_btc', 'm_btc'])
    expect(rev[rev.length - 1].group).toBe('crypto')
  })

  it('built-ins missing a row still get their default bank', () => {
    expect(sections.find((s) => s.institution === 'IBKR')!.fields[0].key).toBe('ibkr_stocks')
  })

  it('keeps archived accounts (callers decide to hide them)', () => {
    expect(sections.flatMap((s) => s.fields).find((f) => f.key === 'acc_old')!.archived).toBe(true)
  })
})
