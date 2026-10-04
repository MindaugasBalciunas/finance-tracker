// The last month that has fully ended, as YYYY-MM — what a review defaults to.
export function lastCompleteMonth(now = new Date()): string {
  return new Date(Date.UTC(now.getFullYear(), now.getMonth() - 1, 1)).toISOString().slice(0, 7)
}
