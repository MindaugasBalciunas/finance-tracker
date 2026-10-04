import { useState } from 'react'
import { useAccounts, useCreateAccount, useUpdateAccount } from '../../hooks/useAccounts'
import { ACCOUNT_GROUP_LABELS, type AccountGroup } from '../../types'

const GROUP_OPTIONS = Object.entries(ACCOUNT_GROUP_LABELS) as [AccountGroup, string][]

// Lists every balance-sheet account and lets the user add their own. Built-in
// accounts are fixed; added ones can be renamed, regrouped and archived —
// never deleted, because every past snapshot that held one still counts it.
export default function AccountsManager() {
  const { data: accounts } = useAccounts()
  const create = useCreateAccount()
  const update = useUpdateAccount()
  const [label, setLabel] = useState('')
  const [group, setGroup] = useState<AccountGroup>('cash')
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)

  const added = (accounts ?? []).filter((a) => !a.builtin)
  const builtin = (accounts ?? []).filter((a) => a.builtin)

  const add = (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    create.mutate({ label: label.trim(), group }, {
      onSuccess: () => setLabel(''),
      onError: (err: any) => setError(err?.response?.data?.error ?? 'Could not add the account'),
    })
  }

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center justify-between text-left"
      >
        <span>
          <span className="text-base font-semibold text-gray-900">Accounts</span>
          <span className="ml-2 text-xs text-gray-400">
            {builtin.length} built-in{added.length > 0 && ` · ${added.length} added`}
          </span>
        </span>
        <span className="text-gray-400 text-sm">{open ? '▲' : '▼'}</span>
      </button>

      {open && (
        <div className="mt-4 space-y-4">
          {added.length > 0 && (
            <ul className="divide-y divide-gray-100">
              {added.map((a) => (
                <li key={a.id} className="py-2 flex flex-wrap items-center gap-2">
                  <input
                    defaultValue={a.label}
                    onBlur={(e) => {
                      const v = e.target.value.trim()
                      if (v && v !== a.label) update.mutate({ id: a.id, input: { label: v } })
                    }}
                    className={`flex-1 min-w-0 border border-gray-200 rounded-lg px-2 py-1 text-sm ${a.archived ? 'text-gray-400' : ''}`}
                    aria-label="Account name"
                  />
                  <select
                    value={a.group}
                    onChange={(e) => update.mutate({ id: a.id, input: { group: e.target.value as AccountGroup } })}
                    className="border border-gray-200 rounded-lg px-2 py-1 text-sm"
                    aria-label="Group"
                  >
                    {GROUP_OPTIONS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}
                  </select>
                  <button
                    type="button"
                    onClick={() => update.mutate({ id: a.id, input: { archived: !a.archived } })}
                    className="text-xs text-gray-500 hover:text-gray-800 underline"
                  >
                    {a.archived ? 'Reopen' : 'Archive'}
                  </button>
                </li>
              ))}
            </ul>
          )}

          <form onSubmit={add} className="flex flex-wrap items-center gap-2">
            <input
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="New account, e.g. Paysera"
              className="flex-1 min-w-0 border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <select
              value={group}
              onChange={(e) => setGroup(e.target.value as AccountGroup)}
              className="border border-gray-300 rounded-lg px-2 py-1.5 text-sm"
              aria-label="Group for the new account"
            >
              {GROUP_OPTIONS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}
            </select>
            <button
              type="submit"
              disabled={!label.trim() || create.isPending}
              className="px-3 py-1.5 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
            >
              Add
            </button>
          </form>
          {error && <p className="text-xs text-red-600">{error}</p>}
          <p className="text-xs text-gray-400">
            An added account gets its own field in balance snapshots, counts in its group and the total, and can be
            picked for bank sync and transactions. Archiving keeps its history.
          </p>
          <p className="text-xs text-gray-400">
            Built-in: {builtin.map((a) => a.label).join(', ')}
          </p>
        </div>
      )}
    </div>
  )
}
