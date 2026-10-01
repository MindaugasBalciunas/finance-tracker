import { NavLink } from 'react-router-dom'
import clsx from 'clsx'
import { useAIAvailable } from '../../hooks/useInsights'

// Shared chrome for the three label-management pages (Labels / Rules /
// AI tagging) so they behave as one section split for readability.

export type Banner = { kind: 'ok' | 'error'; text: string } | null

export function errText(err: unknown): string {
  const e = err as { response?: { data?: { error?: string } }; message?: string }
  return e.response?.data?.error ?? e.message ?? 'Something went wrong'
}

const tabs = [
  { to: '/labels', label: '🏷️ Labels' },
  { to: '/labels/rules', label: '⚡ Rules' },
  { to: '/labels/ai', label: '✦ AI tagging' },
]

export default function LabelsNav() {
  // AI tagging is a third of this section; with AI off it's just a dead tab.
  const aiOn = useAIAvailable()
  const visible = aiOn ? tabs : tabs.filter((t) => t.to !== '/labels/ai')
  return (
    <div className="flex gap-1 bg-gray-100 rounded-xl p-1 w-fit">
      {visible.map((t) => (
        <NavLink
          key={t.to}
          to={t.to}
          end
          className={({ isActive }) => clsx(
            'px-3 py-1.5 rounded-lg text-sm font-medium transition-colors whitespace-nowrap',
            isActive ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-800'
          )}
        >
          {t.label}
        </NavLink>
      ))}
    </div>
  )
}

export function BannerAlert({ banner, onClose }: { banner: Banner; onClose: () => void }) {
  if (!banner) return null
  return (
    <div className={`flex items-start justify-between gap-3 text-sm rounded-xl px-4 py-2.5 border ${
      banner.kind === 'ok'
        ? 'bg-emerald-50 border-emerald-200 text-emerald-800'
        : 'bg-red-50 border-red-200 text-red-800'
    }`}>
      <span>{banner.text}</span>
      <button onClick={onClose} className="opacity-60 hover:opacity-100">✕</button>
    </div>
  )
}
