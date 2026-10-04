// The last month that has fully ended, as YYYY-MM — what a review defaults to.
export function lastCompleteMonth(now = new Date()): string {
  return new Date(Date.UTC(now.getFullYear(), now.getMonth() - 1, 1)).toISOString().slice(0, 7)
}

// The dashboard's "review is ready" nudge remembers, per device, the last
// review month the user closed or opened. A new month has a new key, so the
// nudge comes back on its own. Storage can be unavailable (private mode);
// then the nudge simply shows.
const SEEN_KEY = 'review-nudge-seen'

export function reviewNudgeSeen(month: string): boolean {
  try {
    return localStorage.getItem(SEEN_KEY) === month
  } catch {
    return false
  }
}

export function markReviewNudgeSeen(month: string): void {
  try {
    localStorage.setItem(SEEN_KEY, month)
  } catch {
    // not persisted — it shows again next visit, which is harmless
  }
}
