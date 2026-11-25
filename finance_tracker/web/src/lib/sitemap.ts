// Every page, by section: the sidebar lists the open section's pages under
// it and the phone menu lists them all, so a view that shares a tab with
// another (Merchants under Spending, Trips under Goals) is never more than
// one tap from the menu. Keep in step with each section's own tabs.
export type PageLink = { to: string; label: string }

export const SITEMAP: Record<string, PageLink[]> = {
  '/ledger': [
    { to: '/ledger', label: 'Transactions' }, { to: '/ledger/inbox', label: 'Bank inbox' }, { to: '/ledger/tidy', label: 'Tidy up' },
  ],
  '/plan': [
    { to: '/plan', label: 'Budget' }, { to: '/plan/cash', label: 'Cash until payday' }, { to: '/plan/recurring', label: 'Recurring' },
    { to: '/plan/forecast', label: 'Forecast' }, { to: '/plan/wishlist', label: 'Wish list' }, { to: '/plan/trips', label: 'Trips' },
    { to: '/plan/settings', label: 'Income & salary' },
  ],
  '/wealth': [
    { to: '/wealth', label: 'Net worth' }, { to: '/wealth/investments', label: 'Investments' }, { to: '/wealth/dips', label: 'Dip plan' }, { to: '/wealth/bets', label: 'Bets dip plan' }, { to: '/wealth/btc', label: 'BTC dip plan' },
    { to: '/wealth/ownership', label: 'Property & loans' },
    { to: '/wealth/fi', label: 'Independence' }, { to: '/wealth/stress', label: 'Stress test' }, { to: '/wealth/tax', label: 'Taxes' },
    { to: '/wealth/history', label: 'Balance history' },
  ],
  '/insights': [
    { to: '/insights', label: 'Summary' }, { to: '/insights/review', label: 'Month review' }, { to: '/insights/cashflow', label: 'Cash flow' },
    { to: '/insights/spending', label: 'Spending' }, { to: '/insights/merchants', label: 'Merchants' }, { to: '/insights/optional', label: 'Optional spending' },
    { to: '/insights/trends', label: 'Trends' }, { to: '/insights/drift', label: 'Lifestyle drift' }, { to: '/insights/fun', label: 'Fun facts' },
  ],
}
