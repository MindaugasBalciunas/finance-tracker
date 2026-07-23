// Lithuanian employee net-salary calculation (2026 parameters, standard
// employment contract, no additional pension accumulation).
// Approximate: the progressive 32% GPM band above 60 VDU/year and the Sodra
// ceiling are not modeled — irrelevant below ~€8.6k/month gross.

export interface LtNetBreakdown {
  gross: number
  sodra: number // VSD 12.52% + PSD 6.98% = 19.5%
  npd: number // non-taxable income amount
  gpm: number // income tax 20% of (gross − NPD)
  net: number
  netAfterDeductions: number
}

const SODRA_RATE = 0.1252 + 0.0698
const GPM_RATE = 0.2
const MMA = 1038 // minimum monthly salary the NPD formula anchors on
const NPD_MAX = 747
const NPD_SLOPE = 0.49

export function ltNetSalary(gross: number, deductions = 0): LtNetBreakdown {
  const sodra = gross * SODRA_RATE
  const npd = gross <= MMA ? NPD_MAX : Math.max(0, NPD_MAX - NPD_SLOPE * (gross - MMA))
  const gpm = Math.max(0, (gross - npd) * GPM_RATE)
  const net = gross - sodra - gpm
  return {
    gross,
    sodra: round2(sodra),
    npd: round2(npd),
    gpm: round2(gpm),
    net: round2(net),
    netAfterDeductions: round2(net - deductions),
  }
}

function round2(v: number): number {
  return Math.round(v * 100) / 100
}
