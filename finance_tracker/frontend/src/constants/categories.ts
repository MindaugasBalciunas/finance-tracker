import type { Category, TransactionType } from '../types'

export const CATEGORIES_BY_TYPE: Record<TransactionType, Category[]> = {
  expense: [
    'Clothing',
    'Dating',
    'Divorce',
    'Entertainment',
    'Finance',
    'Food',
    'Gifts',
    'Health',
    'Housing',
    'Kids - Education',
    'Kids - Entertainment',
    'Kids - Food',
    'Kids - General',
    'Subscriptions',
    'Transport',
    'Utilities',
    'Vacation',
  ],
  income: [
    'Salary',
    'Freelance',
    'Reimbursement',
  ],
  investment: [
    'Stocks & ETF',
    'Pension',
    'Crypto',
    'Real Estate',
    'Vehicle',
  ],
}

// Flat list for contexts that don't filter by type (e.g. filter dropdowns)
export const CATEGORIES: Category[] = [
  ...CATEGORIES_BY_TYPE.expense,
  ...CATEGORIES_BY_TYPE.income,
  ...CATEGORIES_BY_TYPE.investment,
]
