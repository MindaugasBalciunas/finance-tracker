import clsx from 'clsx'

interface StatCardProps {
  title: string
  value: string
  subtitle?: string
  trend?: number
  color?: 'blue' | 'green' | 'red' | 'yellow' | 'purple'
}

const colorMap = {
  blue: 'bg-blue-50 text-blue-700 border-blue-200',
  green: 'bg-green-50 text-green-700 border-green-200',
  red: 'bg-red-50 text-red-700 border-red-200',
  yellow: 'bg-yellow-50 text-yellow-700 border-yellow-200',
  purple: 'bg-purple-50 text-purple-700 border-purple-200',
}

export default function StatCard({ title, value, subtitle, trend, color = 'blue' }: StatCardProps) {
  return (
    <div className={clsx('rounded-xl border p-5', colorMap[color])}>
      <p className="text-sm font-medium opacity-80">{title}</p>
      <p className="text-2xl font-bold mt-1">{value}</p>
      {subtitle && <p className="text-xs opacity-70 mt-1">{subtitle}</p>}
      {trend !== undefined && (
        <p className={clsx('text-xs font-medium mt-2', trend >= 0 ? 'text-green-600' : 'text-red-600')}>
          {trend >= 0 ? '▲' : '▼'} {Math.abs(trend).toFixed(1)}% vs last period
        </p>
      )}
    </div>
  )
}
