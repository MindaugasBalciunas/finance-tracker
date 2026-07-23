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

// One-line helper shown under the category picker.
export const CATEGORY_HINTS: Record<string, string> = {
  'Clothing': 'Clothes and shoes',
  'Dating': 'Dates — restaurants, flowers, outings together',
  'Divorce': 'Legal, notary and settlement costs',
  'Entertainment': 'Bars, events, hobbies, one-off fun purchases',
  'Finance': 'Loan payments, bank fees, taxes, insurance, transfers',
  'Food': 'Groceries and eating out',
  'Gifts': 'Presents and donations',
  'Health': 'Medicine, doctors, dentist, sports',
  'Housing': 'Rent, furniture, home improvement and repairs',
  'Kids - Education': 'School, kindergarten, courses',
  'Kids - Entertainment': 'Activities, toys, outings with kids',
  'Kids - Food': 'Food bought specifically for the kids',
  'Kids - General': 'Alimony, clothes, everything else for the kids',
  'Subscriptions': 'Recurring digital services — YouTube, Patreon, Netflix…',
  'Transport': 'Fuel, parking, public transport, car upkeep',
  'Utilities': 'Electricity, heating, water, internet, security',
  'Vacation': 'Trips and holidays',
  'Salary': 'Net employment pay',
  'Freelance': 'Side income and other inflows',
  'Reimbursement': 'Money returned to you',
  'Stocks & ETF': 'Brokerage top-ups and ETF purchases (VWCE…)',
  'Pension': '2nd/3rd pillar contributions (Artea…)',
  'Crypto': 'Bitcoin and other crypto purchases',
  'Real Estate': 'Property purchases and capital improvements',
  'Vehicle': 'Car purchase, leasing payments and buyouts',
}
