import { describe, it, expect } from 'vitest'
import { ltNetSalary } from './ltSalary'

describe('ltNetSalary', () => {
  it('matches the real €8,400 gross → €5,082 net payslip', () => {
    const b = ltNetSalary(8400)
    expect(b.npd).toBe(0) // NPD fully phased out at this level
    expect(b.sodra).toBeCloseTo(1638)
    expect(b.gpm).toBeCloseTo(1680)
    expect(b.net).toBeCloseTo(5082)
  })

  it('subtracts fixed monthly deductions', () => {
    expect(ltNetSalary(8400, 30).netAfterDeductions).toBeCloseTo(5052)
  })

  it('applies full NPD at minimum wage', () => {
    const b = ltNetSalary(1038)
    expect(b.npd).toBe(747)
    expect(b.gpm).toBeCloseTo((1038 - 747) * 0.2)
    expect(b.net).toBeCloseTo(1038 - 1038 * 0.195 - 58.2)
  })

  it('phases NPD out linearly between MMA and the cutoff', () => {
    const b = ltNetSalary(2000)
    expect(b.npd).toBeCloseTo(747 - 0.49 * (2000 - 1038))
    expect(b.npd).toBeGreaterThan(0)
  })
})
