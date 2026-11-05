import type { Flow } from './types'

/** A month's savings rate in % for the chart: counting the pay still to come
 *  while its window is open (November on the 5th, salary due by 10 Dec, is
 *  not a −157% month); null with no income at all. */
export const barRate = (f: Flow) => f.expected_savings_rate != null ? f.expected_savings_rate * 100 : f.income > 0 ? f.savings_rate * 100 : null
