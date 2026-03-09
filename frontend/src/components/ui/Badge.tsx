import clsx from 'clsx'
import type { TransactionType } from '../../types'

const typeColors: Record<TransactionType, string> = {
  expense: 'bg-red-100 text-red-700',
  income: 'bg-green-100 text-green-700',
  investment: 'bg-blue-100 text-blue-700',
}

interface BadgeProps {
  type: TransactionType
}

export default function Badge({ type }: BadgeProps) {
  return (
    <span className={clsx('inline-block px-2 py-0.5 text-xs font-semibold rounded-full capitalize', typeColors[type])}>
      {type}
    </span>
  )
}
