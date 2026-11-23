import { timelineModel } from '../components/MonthChart'

// The bar at today, its label and the note must add up to the month's own
// figure — a €995 bar once carried a "€1,472 over" label.
const days = (cum: number, today: number) => Array.from({ length: 30 }, (_, i) => ({ day: i + 1, cum: i < today ? cum : null })) as never

describe('month chart model', () => {
  it('a normal month: bar = left to spend', () => {
    const m = timelineModel({ free_spent: 600, fund_overdrawn: 0, safe_to_spend: 848, free_planned: 1448, days: days(600, 23) }, 23)
    expect(m.shortPay).toBe(0)
    expect(m.planFree).toBe(1448)
    expect(m.gap).toBeCloseTo(848)
  })
  it('a short pay: line = usual month, bar + short pay = the month', () => {
    // November: €995 spent, €4,125 pay (−€477 spending money), usual €1,448
    const m = timelineModel({ free_spent: 995.28, fund_overdrawn: 0, safe_to_spend: -1471.93, free_planned: 1448.34, days: days(995.28, 23) }, 23)
    expect(m.shortPay).toBeCloseTo(1925.0, 0)
    expect(m.planFree).toBeCloseTo(1448.34)
    expect(m.gap).toBeCloseTo(453.06)
    expect(m.gap - m.shortPay).toBeCloseTo(-1471.93) // the note's sum is the month's figure
  })
  it('commitments above a usual income: line at €0, never below', () => {
    const m = timelineModel({ free_spent: 300, fund_overdrawn: 0, safe_to_spend: -500, free_planned: -200, days: days(300, 10) }, 10)
    expect(m.shortPay).toBe(0)
    expect(m.planFree).toBe(0)
    expect(m.gap).toBe(-300)
  })
})
