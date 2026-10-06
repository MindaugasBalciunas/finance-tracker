import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import clsx from 'clsx'
import { useOverview } from '../lib/hooks'
import type { Action } from '../lib/types'
import { Sheet } from './ui'
import { Icon } from './Icon'

const ICON: Record<string, string> = { move: 'swap', short: 'alert', planned: 'calendar', inbox: 'inbox', review: 'chart', check: 'refresh', budgets: 'target' }
const KEY = 'ft-seen-notifications'
const keyOf = (a: Action) => `${a.kind}|${a.text}`

function loadSeen(): Set<string> {
  try { return new Set(JSON.parse(localStorage.getItem(KEY) || '[]')) } catch { return new Set() }
}

/** What needs the owner — money to move before a payment, rows to review, a
 *  month review — as notifications rather than a block on Home. "Seen" is
 *  remembered on this device only, so the badge counts what's new. */
export function useNotifications() {
  const { data } = useOverview()
  const actions = data?.actions ?? []
  const [seen, setSeen] = useState(loadSeen)
  const unseen = actions.filter((a) => !seen.has(keyOf(a)))
  const urgent = actions.some((a) => a.level !== 'info')
  const markSeen = () => {
    const next = new Set(actions.map(keyOf)) // forget what is no longer there
    setSeen(next)
    try { localStorage.setItem(KEY, JSON.stringify([...next])) } catch { /* private mode */ }
  }
  return { actions, unseen: unseen.length, urgent, markSeen, isNew: (a: Action) => !seen.has(keyOf(a)) }
}

export function NotificationsButton({ variant, collapsed = false }: { variant: 'header' | 'sidebar'; collapsed?: boolean }) {
  const n = useNotifications()
  const [open, setOpen] = useState(false)
  const [newOnOpen, setNewOnOpen] = useState<Set<string>>(new Set())
  const show = () => { setNewOnOpen(new Set(n.actions.filter(n.isNew).map(keyOf))); setOpen(true); n.markSeen() }
  const badge = n.unseen > 0 && (
    <span className={clsx('grid min-w-4 place-items-center rounded-full px-1 text-[10px] font-semibold leading-4 text-white', n.urgent ? 'bg-warn' : 'bg-accent')}>{n.unseen}</span>
  )
  return (
    <>
      {variant === 'header' ? (
        <button className="btn-ghost relative h-9 w-9 px-0" onClick={show} aria-label={`Notifications${n.unseen ? `, ${n.unseen} new` : ''}`}>
          <Icon name="bell" />
          {badge && <span className="absolute right-0.5 top-0.5">{badge}</span>}
        </button>
      ) : (
        <button onClick={show} title={collapsed ? 'Notifications' : undefined} aria-label={`Notifications${n.unseen ? `, ${n.unseen} new` : ''}`}
          className={clsx('relative flex h-10 items-center rounded-xl text-sm font-medium text-ink2 transition hover:bg-sunken hover:text-ink', collapsed ? 'w-10 justify-center' : 'gap-3 px-3')}>
          <Icon name="bell" size={20} />
          {!collapsed && <span className="flex-1 text-left">Notifications</span>}
          {badge && <span className={collapsed ? 'absolute -right-1 -top-1' : ''}>{badge}</span>}
        </button>
      )}
      {/* Portal: the mobile header's backdrop blur would otherwise trap the
          fixed-position sheet inside its 48px bar. */}
      {open && createPortal(<NotificationsPanel actions={n.actions} isNew={(a) => newOnOpen.has(keyOf(a))} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

function NotificationsPanel({ actions, isNew, onClose }: { actions: Action[]; isNew: (a: Action) => boolean; onClose: () => void }) {
  const nav = useNavigate()
  const go = (a: Action) => {
    onClose()
    if (a.link === 'cash') {
      nav('/')
      // After Home renders (the panel is gone by then, so not an effect here).
      setTimeout(() => document.getElementById('cash')?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 400)
    } else if (a.link) nav(a.link)
  }
  return (
    <Sheet open onClose={onClose} title="Notifications">
      {actions.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted"><Icon name="check" className="mx-auto mb-2 text-good" />Nothing needs you right now.</div>
      ) : (
        <div className="-mx-1 divide-y divide-line">
          {actions.map((a, i) => (
            <button key={i} onClick={() => go(a)} className="flex w-full items-center gap-3 rounded-lg px-1 py-2.5 text-left hover:bg-sunken/40">
              <span className={clsx('grid h-8 w-8 shrink-0 place-items-center rounded-full', a.level === 'bad' ? 'bg-bad/15 text-bad' : a.level === 'warn' ? 'bg-warn/15 text-warn' : 'bg-accent/10 text-accent')}>
                <Icon name={ICON[a.kind] ?? 'alert'} size={16} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm">{a.text}{isNew(a) && <span className="ml-1.5 rounded-full bg-accent/10 px-1.5 text-[10px] font-medium text-accent">new</span>}</span>
                {a.detail && <span className="block text-xs text-muted">{a.detail}</span>}
              </span>
              <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
            </button>
          ))}
        </div>
      )}
    </Sheet>
  )
}
