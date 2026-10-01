import { describe, it, expect } from 'vitest'
import { formatAICost } from './aiCost'

describe('formatAICost', () => {
  // A reported cost is what the provider billed; a computed one is not, and
  // the tilde is the entire difference the reader gets to see.
  it('marks an estimate and leaves a reported cost alone', () => {
    expect(formatAICost(0.0123, false)).toBe('$0.0123')
    expect(formatAICost(0.0123, true)).toBe('~$0.0123')
  })

  // Most single answers cost a fraction of a cent — two decimals would print
  // $0.00 and the badge would look broken.
  it('keeps sub-cent amounts legible', () => {
    expect(formatAICost(0.0009, true)).toBe('~$0.0009')
    expect(formatAICost(1.5, false)).toBe('$1.5')
  })

  // No cost is not the same as a free call: an unpriced model must render
  // nothing rather than "$0".
  it('renders nothing when there is no cost', () => {
    expect(formatAICost(0, true)).toBeNull()
    expect(formatAICost(undefined, false)).toBeNull()
    expect(formatAICost(-1, false)).toBeNull()
  })
})
