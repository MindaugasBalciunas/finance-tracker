import { describe, expect, it } from 'vitest'
import { logoName } from '../components/MerchantLogo'
import type { Account } from '../lib/types'

const acc = (id: string, institution: string) => ({ id, name: id, institution, kind: 'checking' }) as Account
const accounts = [acc('swed', 'Swedbank'), acc('seb', 'SEB'), acc('cash', '')]

describe('logoName', () => {
  it('keeps a merchant', () => {
    expect(logoName({ merchant: 'Maxima', note: '', category: 'finance.bank_fees', account_id: 'swed' }, accounts)).toBe('Maxima')
  })
  it('gives a bank charge the bank named in its note, else the account’s bank', () => {
    expect(logoName({ merchant: '', note: 'Seb bank fee', category: 'finance.bank_fees', account_id: 'swed' }, accounts)).toBe('SEB')
    expect(logoName({ merchant: '', note: 'Banko mokestis', category: 'finance.bank_fees', account_id: 'swed' }, accounts)).toBe('Swedbank')
    expect(logoName({ merchant: '', note: 'Loan interest', category: 'housing.mortgage_interest', account_id: 'seb' }, accounts)).toBe('SEB')
  })
  it('gives loan interest with no account the bank that holds the loan', () => {
    const withLoan = [...accounts, { id: 'mortgage', name: 'Mortgage', institution: 'SEB', kind: 'loan' } as Account]
    expect(logoName({ merchant: '', note: 'Loan interest', category: 'housing.mortgage_interest', account_id: '' }, withLoan)).toBe('SEB')
    expect(logoName({ merchant: '', note: 'Loan interest', category: 'housing.mortgage_interest', account_id: '' }, accounts)).toBe('')
  })
  it('leaves other rows without a merchant alone', () => {
    expect(logoName({ merchant: '', note: 'Seb loan notary', category: 'finance.legal', account_id: 'swed' }, accounts)).toBe('')
  })
})
