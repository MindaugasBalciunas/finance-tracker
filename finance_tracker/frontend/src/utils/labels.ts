import type { Transaction } from '../types'

// Labels marking fixed obligations and counterparty transfers — money that
// isn't a spending decision, so it shouldn't dominate spending stories
// ("most money went to Evelina — 44k across 39 payments" is a loan, not news).
// Single source of truth: insights, cards and Reports all import from here.
export const FIXED_LABELS = ['loan', 'alimony', 'leasing', 'evelina']

// Parses the comma-separated labels field into clean tokens.
export function txLabels(tx: Pick<Transaction, 'labels'>): string[] {
  return (tx.labels ?? '')
    .split(',')
    .map((l) => l.trim())
    .filter(Boolean)
}

export function hasAnyLabel(tx: Pick<Transaction, 'labels'>, labels: string[]): boolean {
  const own = txLabels(tx)
  return labels.some((l) => own.includes(l))
}

// A committed transaction carries one of the fixed-obligation labels.
export function isCommitted(tx: Pick<Transaction, 'labels'>): boolean {
  return hasAnyLabel(tx, FIXED_LABELS)
}

// Discretionary spending only — fixed obligations excluded via labels.
export function discretionary<T extends Pick<Transaction, 'labels'>>(txs: T[]): T[] {
  return txs.filter((tx) => !isCommitted(tx))
}
