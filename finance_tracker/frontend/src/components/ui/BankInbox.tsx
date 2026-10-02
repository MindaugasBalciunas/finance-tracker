import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { SyncAllResult } from '../../api/banking'
import {
  useBankConnections,
  useBankSettings,
  useStagedTransactions,
  useSyncAllBanks,
} from '../../hooks/useBanking'
import BankStagingList from './BankStagingList'
import LoadingSpinner from './LoadingSpinner'

/**
 * Bank rows waiting to become transactions, on the page where transactions
 * are added.
 *
 * This used to live three taps deep — More → Bank connections → scroll —
 * which is the wrong place for the one thing about banking you do weekly.
 * Connecting a bank is settings and stays on the banking page; pulling what
 * is new and approving it is transaction entry, and belongs here next to
 * "+ Add".
 */
type Tab = 'staged' | 'dismissed' | 'imported'

const TAB_LABEL: Record<Tab, string> = {
  staged: 'To review',
  dismissed: 'Dismissed',
  imported: 'Added',
}

export default function BankInbox({
  open: openProp,
  onOpenChange,
}: {
  /** Controlled from the Add menu's "From your bank" entry. */
  open?: boolean
  onOpenChange?: (open: boolean) => void
} = {}) {
  const { data: settings } = useBankSettings()
  const configured = !!settings?.configured

  const connections = useBankConnections(configured)
  const [tab, setTab] = useState<Tab>('staged')
  // The queue drives the header count and the nav badge, so it is asked for
  // regardless of which tab is showing.
  const staged = useStagedTransactions({ state: 'staged' }, configured)
  const archive = useStagedTransactions({ state: tab }, configured && tab !== 'staged')
  const list = tab === 'staged' ? staged : archive
  const syncAll = useSyncAllBanks()
  const [report, setReport] = useState<SyncAllResult | null>(null)
  const [openState, setOpenState] = useState(false)
  const open = openProp ?? openState
  const setOpen = (next: boolean) => {
    setOpenState(next)
    onOpenChange?.(next)
  }

  const pending = staged.data?.total ?? 0
  // Nothing to do and nothing to connect: a user who never set this up gets
  // no bar at all.
  if (!configured) return null

  // Syncable accounts, so the bar can say what pressing Sync would actually
  // reach rather than failing silently when nothing is mapped.
  const mapped = (connections.data?.connections ?? []).flatMap((c) =>
    c.accounts.filter((a) => a.account_key)
  ).length

  const run = async (days?: number) => {
    setReport(null)
    try {
      const res = await syncAll.mutateAsync(days)
      setReport(res)
      if (res.totals.staged_new > 0) {
        setTab('staged')
        setOpen(true)
      }
    } catch {
      // The mutation's own error state renders below.
    }
  }

  const busy = syncAll.isPending

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm">
      {/* Two rows on a phone: the title and two buttons do not fit on one
          390px line without the heading wrapping mid-phrase. */}
      <div className="flex flex-wrap items-center gap-y-2 gap-x-2 p-3 sm:p-4">
        <button
          onClick={() => setOpen(!open)}
          className="w-full sm:flex-1 min-w-0 flex items-center gap-2 text-left"
        >
          <span className="text-lg shrink-0">🔗</span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2 text-sm font-medium text-gray-800">
              <span className="whitespace-nowrap">From your bank</span>
              {pending > 0 && (
                <span className="shrink-0 px-2 py-0.5 text-xs font-semibold rounded-full bg-indigo-100 text-indigo-700">
                  {pending} to review
                </span>
              )}
              {/* Next to the title, not floating at the far right of an
                  otherwise empty row. */}
              <span className="text-gray-300 text-xs">{open ? '▴' : '▾'}</span>
            </span>
            <span className="block text-xs text-gray-400 truncate">
              {open
                ? 'Tap to hide'
                : pending > 0
                  ? 'Tap to review and add them'
                  : mapped === 0
                    ? 'No account is mapped yet — set one up in Bank connections'
                    : 'Nothing waiting — tap for what was added or dismissed'}
            </span>
          </span>
        </button>

        <div className="flex items-center gap-2 shrink-0 ml-auto">
          {/* Deliberately two, mirroring the per-account pair: the cautious
              7-day look is the one to press first against a real bank, before
              pulling 90 days across weeks of existing rows. */}
          <button
            onClick={() => run(7)}
            disabled={busy || mapped === 0}
            className="px-2.5 py-1.5 text-xs rounded-lg border border-gray-200 text-gray-700 hover:bg-gray-50 disabled:opacity-40"
          >
            Last 7 days
          </button>
          <button
            onClick={() => run()}
            disabled={busy || mapped === 0}
            className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-40"
          >
            {busy ? 'Syncing…' : 'Sync all'}
          </button>
        </div>
      </div>

      {syncAll.isError && (
        <p className="px-3 sm:px-4 pb-3 text-xs text-red-600">
          {syncAll.error instanceof Error ? syncAll.error.message : 'Could not sync'}
        </p>
      )}

      {report && <SyncReport report={report} />}

      {open && (
        <div className="px-3 sm:px-4 pb-3 border-t border-gray-50 pt-3">
          {/* Dismissed and Added live here rather than on the settings page:
              putting a row back, or checking what a sync actually added, is
              part of reviewing, not part of connecting a bank. */}
          <div className="flex items-center gap-1 mb-2 overflow-x-auto">
            {(['staged', 'dismissed', 'imported'] as Tab[]).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`px-3 py-1.5 text-sm rounded-lg whitespace-nowrap ${
                  tab === t ? 'bg-gray-900 text-white' : 'text-gray-500 hover:bg-gray-100'
                }`}
              >
                {TAB_LABEL[t]}
                {t === 'staged' && pending > 0 ? ` (${pending})` : ''}
              </button>
            ))}
          </div>
          {list.isLoading && <LoadingSpinner />}
          {list.data && <BankStagingList rows={list.data.transactions} />}
        </div>
      )}
    </div>
  )
}

/**
 * What the sync actually did, per account.
 *
 * One bank failing while another works is the normal case, not the edge one —
 * consents expire on their own schedules — so a single "failed" would hide
 * which one needs attention.
 */
function SyncReport({ report }: { report: SyncAllResult }) {
  const { totals, accounts, synced, failed } = report

  if (accounts.length === 0) {
    return (
      <p className="px-3 sm:px-4 pb-3 text-xs text-gray-500">
        No account is mapped to sync yet —{' '}
        <Link to="/banking" className="underline">
          set one up
        </Link>
        .
      </p>
    )
  }

  const summary = [
    `${totals.fetched} fetched from ${synced} account${synced === 1 ? '' : 's'}`,
    `${totals.staged_new} new to review`,
  ]
  if (totals.unchanged) summary.push(`${totals.unchanged} already seen`)
  // auto_skipped counts one thing only: rows the bank has not booked yet.
  if (totals.auto_skipped) summary.push(`${totals.auto_skipped} still pending at the bank`)

  return (
    <div className="px-3 sm:px-4 pb-3 space-y-1">
      <p className={`text-xs ${failed ? 'text-gray-600' : 'text-emerald-600'}`}>{summary.join(' · ')}</p>
      {accounts
        .filter((a) => a.error || a.skipped)
        .map((a) => (
          <p key={a.link_id} className={`text-xs ${a.error ? 'text-red-600' : 'text-amber-700'}`}>
            {a.bank} · {a.name} — {a.error ?? a.skipped}
          </p>
        ))}
    </div>
  )
}
