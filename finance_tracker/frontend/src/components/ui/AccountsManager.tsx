import { useMemo, useState } from 'react'
import { useAccountLabels, useAccountName, useAccounts, useAddedAccounts, useCreateAccount, useUpdateAccount } from '../../hooks/useAccounts'
import { useBankConnections, useBankSettings, useMapBankAccount } from '../../hooks/useBanking'
import type { BankAccountLink, BankConnection } from '../../api/banking'
import { Link } from 'react-router-dom'
import { ACCOUNT_GROUP_LABELS, type Account, type AccountGroup, type Balance } from '../../types'
import { GROUPS, OTHER_COLOR, withAddedAccounts } from '../../utils/balanceGroups'
import { formatEuro } from '../../utils/format'

const GROUP_OPTIONS = Object.entries(ACCOUNT_GROUP_LABELS) as [AccountGroup, string][]

const GROUP_HINTS: Record<AccountGroup, string> = {
  cash: 'Spendable money — counts toward free cash and the savings runway.',
  investments: 'Brokerage, ETFs, funds.',
  pensions: 'Pension pillars and other locked-up savings.',
  crypto: 'Crypto held in EUR value.',
  other: 'Anything else — counted in the total only.',
}

const GROUP_COLOR: Record<AccountGroup, string> = {
  cash: GROUPS.find((g) => g.group === 'cash')!.color,
  investments: GROUPS.find((g) => g.group === 'investments')!.color,
  pensions: GROUPS.find((g) => g.group === 'pensions')!.color,
  crypto: GROUPS.find((g) => g.group === 'crypto')!.color,
  other: OTHER_COLOR,
}

const errorText = (e: any, fallback: string) => e?.response?.data?.error ?? fallback

// Every balance-sheet account in one place, grouped the way the net-worth
// cards group them, with today's value. Built-in accounts are fixed; added
// ones can be renamed, regrouped and archived — never deleted, because past
// snapshots still count them.
// embedded: shown as its own tab — always open, no collapse header.
export default function AccountsManager({ balance, embedded = false }: { balance?: Balance; embedded?: boolean }) {
  const added = useAddedAccounts()
  const [expanded, setOpen] = useState(false)
  const open = embedded || expanded
  const [editing, setEditing] = useState<number | null>(null)
  const [adding, setAdding] = useState(false)
  const [showArchived, setShowArchived] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)

  // Which accounts a bank feeds — only asked for when banking is set up.
  const { data: bankSettings } = useBankSettings()
  const { data: bank } = useBankConnections(!!bankSettings?.configured)
  const bankFed = useMemo(() => {
    const keys = new Set<string>()
    for (const c of bank?.connections ?? []) for (const a of c.accounts ?? []) if (a.account_key) keys.add(a.account_key)
    return keys
  }, [bank])

  const active = added.filter((a) => !a.archived)
  const archived = added.filter((a) => a.archived)
  // Every account with a row — built-ins included — can be edited; the two
  // BTC lines are computed, not accounts, and have no row.
  const { data: allAccounts } = useAccounts()
  const byKey = useMemo(() => new Map((allAccounts ?? []).map((a) => [a.key, a])), [allAccounts])
  const name = useAccountName()
  const bankLinks = useMemo(
    () => (bank?.connections ?? []).flatMap((c) => (c.accounts ?? []).map((link) => ({ conn: c, link }))),
    [bank],
  )

  const sections = useMemo(() => {
    const { groups, other } = withAddedAccounts(active, name)
    return [
      ...groups.map((g) => ({ group: g.group, title: g.key, accounts: g.accounts })),
      { group: 'other' as AccountGroup, title: 'Other', accounts: other },
    ]
  }, [active, name])

  const count = sections.reduce((n, s) => n + s.accounts.length, 0)

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      {embedded ? (
        <div className="flex items-baseline justify-between">
          <h3 className="text-base font-semibold text-gray-900">Accounts</h3>
          <span className="text-xs text-gray-400">
            {count} active{archived.length > 0 && ` · ${archived.length} archived`}
          </span>
        </div>
      ) : (
      <button type="button" onClick={() => setOpen((o) => !o)} className="w-full flex items-center justify-between text-left" aria-expanded={open}>
        <span>
          <span className="text-base font-semibold text-gray-900">Accounts</span>
          <span className="ml-2 text-xs text-gray-400">
            {count} active{archived.length > 0 && ` · ${archived.length} archived`}
          </span>
        </span>
        <span className="text-gray-400 text-sm">{open ? '▲' : '▼'}</span>
      </button>
      )}

      {open && (
        <div className={embedded ? 'mt-3 space-y-5' : 'mt-4 space-y-5'}>
          {notice && (
            <div className="flex items-start justify-between gap-2 bg-green-50 border border-green-100 rounded-lg px-3 py-2">
              <p className="text-xs text-green-800">{notice}</p>
              <button onClick={() => setNotice(null)} className="text-green-700 text-xs" aria-label="Dismiss">✕</button>
            </div>
          )}

          {sections.map((s) => {
            const total = balance ? s.accounts.reduce((sum, a) => sum + a.value(balance), 0) : null
            return (
              <section key={s.group}>
                <div className="flex items-baseline justify-between border-b border-gray-100 pb-1 mb-1">
                  <h4 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    <span className="w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: GROUP_COLOR[s.group] }} />
                    {s.title}
                  </h4>
                  {total != null && <span className="text-xs font-semibold text-gray-500">{formatEuro(total)}</span>}
                </div>
                {s.accounts.length === 0 ? (
                  <p className="text-xs text-gray-400 py-1.5">No accounts.</p>
                ) : (
                  <ul className="divide-y divide-gray-50">
                    {s.accounts.map((a) => {
                      const acc = byKey.get(a.key)
                      if (acc && editing === acc.id) {
                        return (
                          <li key={a.key} className="py-2">
                            <AccountEditor
                              account={{ ...acc, label: a.label }}
                              bankLinks={bankLinks}
                              onDone={(msg) => { setEditing(null); if (msg) setNotice(msg) }}
                            />
                          </li>
                        )
                      }
                      return (
                        <li key={a.key} className="py-2 flex items-center gap-2 min-w-0">
                          <span className="flex-1 min-w-0">
                            <span className="block text-sm text-gray-800 truncate">{a.label}</span>
                            {(bankFed.has(a.key) || (acc && !acc.builtin)) && (
                              <span className="flex flex-wrap gap-1 mt-0.5">
                                {bankFed.has(a.key) && <Badge tone="blue">🔗 bank sync</Badge>}
                                {acc && !acc.builtin && <Badge tone="gray">added</Badge>}
                              </span>
                            )}
                          </span>
                          {balance && (
                            <span className="text-sm font-semibold text-gray-700 tabular-nums">{formatEuro(a.value(balance))}</span>
                          )}
                          {acc ? (
                            <button
                              onClick={() => { setEditing(acc.id); setAdding(false) }}
                              className="px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded-lg"
                            >
                              Edit
                            </button>
                          ) : (
                            <span className="w-[42px]" aria-hidden />
                          )}
                        </li>
                      )
                    })}
                  </ul>
                )}
              </section>
            )
          })}

          {bank && bank.connections.some((c) => (c.accounts ?? []).length > 0) && (
            <BankLinks connections={bank.connections} validKeys={bank.valid_account_keys} onNotice={setNotice} />
          )}

          {archived.length > 0 && (
            <section>
              <button onClick={() => setShowArchived((v) => !v)} className="text-xs font-semibold uppercase tracking-wide text-gray-400 hover:text-gray-600">
                {showArchived ? '▾' : '▸'} Archived ({archived.length})
              </button>
              {showArchived && <ArchivedList accounts={archived} onNotice={setNotice} />}
            </section>
          )}

          {adding ? (
            <AddAccountForm
              onDone={(msg) => { setAdding(false); if (msg) setNotice(msg) }}
            />
          ) : (
            <button
              onClick={() => { setAdding(true); setEditing(null) }}
              className="w-full py-2 text-sm font-medium text-blue-600 border border-dashed border-blue-200 rounded-lg hover:bg-blue-50"
            >
              + Add account
            </button>
          )}

          <p className="text-xs text-gray-400">
            Built-in accounts are fixed. Added accounts get their own field in balance snapshots, count in their group and
            the total, and can be picked for bank sync and transactions. Archiving hides an account from forms but keeps
            its history.
          </p>
        </div>
      )}
    </div>
  )
}

function Badge({ tone, children }: { tone: 'blue' | 'gray'; children: React.ReactNode }) {
  const cls = tone === 'blue' ? 'bg-blue-50 text-blue-700' : 'bg-gray-100 text-gray-500'
  return <span className={`text-[10px] font-medium rounded px-1.5 py-0.5 ${cls}`}>{children}</span>
}

function GroupSelect({ value, onChange, id }: { value: AccountGroup; onChange: (g: AccountGroup) => void; id?: string }) {
  return (
    <select
      id={id}
      value={value}
      onChange={(e) => onChange(e.target.value as AccountGroup)}
      className="w-full border border-gray-300 rounded-lg px-2 py-1.5 text-sm bg-white"
    >
      {GROUP_OPTIONS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}
    </select>
  )
}

function AddAccountForm({ onDone }: { onDone: (notice?: string) => void }) {
  const create = useCreateAccount()
  const [label, setLabel] = useState('')
  const [group, setGroup] = useState<AccountGroup>('cash')
  const [error, setError] = useState<string | null>(null)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const name = label.trim()
    if (!name) return
    setError(null)
    create.mutate({ label: name, group }, {
      onSuccess: (a) => onDone(`${a.label} added to ${ACCOUNT_GROUP_LABELS[a.group]}. Enter its balance in your next snapshot (+ Add on this page).`),
      onError: (err) => setError(errorText(err, 'Could not add the account')),
    })
  }

  return (
    <form onSubmit={submit} className="border border-blue-100 bg-blue-50/40 rounded-lg p-3 space-y-2.5">
      <h4 className="text-sm font-semibold text-gray-900">New account</h4>
      <div>
        <label htmlFor="new-account-name" className="block text-xs font-medium text-gray-600 mb-0.5">Name</label>
        <input
          id="new-account-name"
          autoFocus
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder="e.g. Paysera"
          maxLength={60}
          className="w-full border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>
      <div>
        <label htmlFor="new-account-group" className="block text-xs font-medium text-gray-600 mb-0.5">Counts as</label>
        <GroupSelect id="new-account-group" value={group} onChange={setGroup} />
        <p className="text-xs text-gray-400 mt-1">{GROUP_HINTS[group]}</p>
      </div>
      {error && <p className="text-xs text-red-600">{error}</p>}
      <div className="flex justify-end gap-2">
        <button type="button" onClick={() => onDone()} className="px-3 py-1.5 text-sm border border-gray-300 rounded-lg bg-white">Cancel</button>
        <button type="submit" disabled={!label.trim() || create.isPending} className="px-3 py-1.5 text-sm font-medium text-white bg-blue-600 rounded-lg disabled:opacity-50">
          {create.isPending ? 'Adding…' : 'Add account'}
        </button>
      </div>
    </form>
  )
}

type BankLinkRef = { conn: BankConnection; link: BankAccountLink }

function AccountEditor({ account, bankLinks, onDone }: {
  account: Account
  bankLinks: BankLinkRef[]
  onDone: (notice?: string) => void
}) {
  const update = useUpdateAccount()
  const [label, setLabel] = useState(account.label)
  const [group, setGroup] = useState<AccountGroup>(account.group)
  const [error, setError] = useState<string | null>(null)
  const dirty = label.trim() !== account.label || group !== account.group

  const save = (e: React.FormEvent) => {
    e.preventDefault()
    if (!label.trim()) return
    if (!dirty) { onDone(); return }
    setError(null)
    update.mutate({ id: account.id, input: account.builtin ? { label: label.trim() } : { label: label.trim(), group } }, {
      onSuccess: () => onDone(),
      onError: (err) => setError(errorText(err, 'Could not save')),
    })
  }

  const archive = () => {
    setError(null)
    update.mutate({ id: account.id, input: { archived: true } }, {
      onSuccess: () => onDone(`${account.label} archived. Its history stays in past snapshots.`),
      onError: (err) => setError(errorText(err, 'Could not archive')),
    })
  }

  return (
    <form onSubmit={save} className="border border-gray-200 rounded-lg p-3 space-y-3">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-0.5">Name</label>
          <input
            autoFocus
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            maxLength={60}
            aria-label="Account name"
            className="w-full border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-0.5">Counts as</label>
          {account.builtin ? (
            <p className="text-sm text-gray-500 py-1.5">{ACCOUNT_GROUP_LABELS[account.group]} <span className="text-xs text-gray-400">· fixed for built-in accounts</span></p>
          ) : (
            <GroupSelect value={group} onChange={setGroup} />
          )}
        </div>
      </div>

      {bankLinks.length > 0 && <BankFeedPicker account={account} bankLinks={bankLinks} />}

      {error && <p className="text-xs text-red-600">{error}</p>}
      <div className="flex items-center gap-2">
        {!account.builtin && (
          <button type="button" onClick={archive} disabled={update.isPending} className="text-xs text-gray-500 hover:text-red-600 underline">
            Archive
          </button>
        )}
        <span className="flex-1" />
        <button type="button" onClick={() => onDone()} className="px-3 py-1.5 text-sm border border-gray-300 rounded-lg">Close</button>
        <button type="submit" disabled={!dirty || !label.trim() || update.isPending} className="px-3 py-1.5 text-sm font-medium text-white bg-blue-600 rounded-lg disabled:opacity-50">
          Save name
        </button>
      </div>
    </form>
  )
}

// Which PSD2 bank accounts keep this account up to date. Ticking one points
// it here (away from wherever it fed before); unticking stops syncing it.
// Applies at once — it is the same link as on the Bank connections page.
function BankFeedPicker({ account, bankLinks }: { account: Account; bankLinks: BankLinkRef[] }) {
  const map = useMapBankAccount()
  const labels = useAccountLabels()
  const [error, setError] = useState<string | null>(null)

  const toggle = (link: BankAccountLink, on: boolean) => {
    setError(null)
    map.mutate({ id: link.id, accountKey: on ? account.key : '' }, {
      onError: (e) => setError(errorText(e, 'Could not change the link')),
    })
  }

  return (
    <fieldset>
      <legend className="block text-xs font-medium text-gray-600 mb-1">Auto-update from bank account</legend>
      <ul className="space-y-1">
        {bankLinks.map(({ conn, link }) => {
          const here = link.account_key === account.key
          const elsewhere = !here && link.account_key ? labels[link.account_key] ?? link.account_key : null
          return (
            <li key={link.id}>
              <label className="flex items-start gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  className="mt-0.5"
                  checked={here}
                  disabled={map.isPending}
                  onChange={(e) => toggle(link, e.target.checked)}
                />
                <span className="min-w-0">
                  <span className="block text-gray-800 truncate">{link.display_name || link.iban}</span>
                  <span className="block text-xs text-gray-400">
                    {conn.aspsp_name}{link.iban && link.display_name ? ` · ${link.iban}` : ''}
                    {link.bank_balance != null && ` · bank says ${formatEuro(link.bank_balance)}`}
                    {elsewhere && <span className="text-amber-700"> · now feeds {elsewhere}</span>}
                  </span>
                </span>
              </label>
            </li>
          )
        })}
      </ul>
      <p className="text-xs text-gray-400 mt-1">
        Applies immediately. Each sync sets this account to the bank's balance — the sum, if you tick more than one.
      </p>
      {error && <p className="text-xs text-red-600 mt-1">{error}</p>}
    </fieldset>
  )
}

function ArchivedList({ accounts, onNotice }: { accounts: Account[]; onNotice: (msg: string) => void }) {
  const update = useUpdateAccount()
  return (
    <ul className="mt-1 divide-y divide-gray-50">
      {accounts.map((a) => (
        <li key={a.id} className="py-2 flex items-center gap-2">
          <span className="flex-1 min-w-0 text-sm text-gray-400 truncate">
            {a.label} <span className="text-xs">· {ACCOUNT_GROUP_LABELS[a.group]}</span>
          </span>
          <button
            onClick={() => update.mutate({ id: a.id, input: { archived: false } }, { onSuccess: () => onNotice(`${a.label} reopened.`) })}
            disabled={update.isPending}
            className="px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded-lg"
          >
            Reopen
          </button>
        </li>
      ))}
    </ul>
  )
}

const NEW_ACCOUNT = '__new__'

// Which balance-sheet account each PSD2 bank account feeds. Every sync sets
// that account to the bank's balance; two bank accounts pointed at the same
// one are added together. Pointing one of them at its own account splits it.
function BankLinks({ connections, validKeys, onNotice }: {
  connections: BankConnection[]
  validKeys: string[]
  onNotice: (msg: string) => void
}) {
  const labels = useAccountLabels()
  const links = connections.flatMap((c) => (c.accounts ?? []).map((a) => ({ conn: c, link: a })))
  const shared = new Map<string, number>()
  for (const { link } of links) if (link.account_key) shared.set(link.account_key, (shared.get(link.account_key) ?? 0) + 1)

  return (
    <section>
      <div className="flex items-baseline justify-between border-b border-gray-100 pb-1 mb-1">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">🔗 Bank accounts (PSD2)</h4>
        <Link to="/banking" className="text-xs text-blue-600 hover:underline">Connections →</Link>
      </div>
      <ul className="divide-y divide-gray-50">
        {links.map(({ conn, link }) => (
          <BankLinkRow
            key={link.id}
            conn={conn}
            link={link}
            validKeys={validKeys}
            labels={labels}
            sharedWith={link.account_key ? (shared.get(link.account_key) ?? 1) - 1 : 0}
            onNotice={onNotice}
          />
        ))}
      </ul>
      <p className="text-xs text-gray-400 mt-1">
        Each sync sets the chosen account to the bank's own balance. Bank accounts pointed at the same account are added
        together — to track one separately, pick “＋ New account…”.
      </p>
    </section>
  )
}

function BankLinkRow({ conn, link, validKeys, labels, sharedWith, onNotice }: {
  conn: BankConnection
  link: BankAccountLink
  validKeys: string[]
  labels: Record<string, string>
  sharedWith: number
  onNotice: (msg: string) => void
}) {
  const map = useMapBankAccount()
  const create = useCreateAccount()
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState('')
  const [group, setGroup] = useState<AccountGroup>('cash')
  const [error, setError] = useState<string | null>(null)
  const title = link.display_name || link.iban || `Account ${link.id}`

  const point = (key: string, label?: string) => {
    setError(null)
    map.mutate({ id: link.id, accountKey: key }, {
      onSuccess: () => onNotice(key
        ? `${title} now feeds ${label ?? labels[key] ?? key}. The next sync sets it to the bank's balance.`
        : `${title} is no longer synced.`),
      onError: (e) => setError(errorText(e, 'Could not change the link')),
    })
  }

  const createAndPoint = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setError(null)
    create.mutate({ label: name.trim(), group }, {
      onSuccess: (a) => { setNaming(false); setName(''); point(a.key, a.label) },
      onError: (err) => setError(errorText(err, 'Could not add the account')),
    })
  }

  return (
    <li className="py-2 space-y-1.5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="flex-1 min-w-[10rem]">
          <span className="block text-sm text-gray-800 truncate">{title}</span>
          <span className="text-xs text-gray-400">
            {conn.aspsp_name}{link.iban && link.display_name ? ` · ${link.iban}` : ''}
            {link.bank_balance != null && ` · bank says ${formatEuro(link.bank_balance)}`}
          </span>
        </span>
        <label className="flex items-center gap-1.5 text-xs text-gray-500">
          feeds
          <select
            value={naming ? NEW_ACCOUNT : link.account_key}
            disabled={map.isPending}
            onChange={(e) => {
              if (e.target.value === NEW_ACCOUNT) { setNaming(true); return }
              setNaming(false)
              point(e.target.value)
            }}
            className="border border-gray-300 rounded-lg px-2 py-1 text-sm bg-white text-gray-800"
            aria-label={`Balance account fed by ${title}`}
          >
            <option value="">Don't sync</option>
            {validKeys.map((k) => <option key={k} value={k}>{labels[k] ?? k}</option>)}
            <option value={NEW_ACCOUNT}>＋ New account…</option>
          </select>
        </label>
      </div>
      {sharedWith > 0 && !naming && (
        <p className="text-xs text-amber-700">
          Added together with {sharedWith} other bank account{sharedWith === 1 ? '' : 's'} into {labels[link.account_key] ?? link.account_key}.
        </p>
      )}
      {naming && (
        <form onSubmit={createAndPoint} className="flex flex-wrap items-center gap-2 bg-blue-50/50 border border-blue-100 rounded-lg p-2">
          <input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={`e.g. ${conn.aspsp_name} savings`}
            maxLength={60}
            className="flex-1 min-w-[8rem] border border-gray-300 rounded-lg px-2 py-1 text-sm"
            aria-label="New account name"
          />
          <div className="w-36"><GroupSelect value={group} onChange={setGroup} /></div>
          <button type="button" onClick={() => setNaming(false)} className="px-2 py-1 text-xs text-gray-500">Cancel</button>
          <button type="submit" disabled={!name.trim() || create.isPending} className="px-3 py-1 text-sm text-white bg-blue-600 rounded-lg disabled:opacity-50">
            Create &amp; link
          </button>
        </form>
      )}
      {error && <p className="text-xs text-red-600">{error}</p>}
    </li>
  )
}
