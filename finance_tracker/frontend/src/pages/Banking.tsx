import { useState } from 'react'
import { Link } from 'react-router-dom'
import BankConnectionCard from '../components/ui/BankConnectionCard'
import BankSettingsModal from '../components/ui/BankSettingsModal'
import BankSetupGuide from '../components/ui/BankSetupGuide'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import {
  useASPSPs,
  useBankCallback,
  useBankConnections,
  useBankSettings,
  useConnectBank,
  useStagedTransactions,
} from '../hooks/useBanking'

// The whole review queue — to review, dismissed and added — lives on the
// transactions page now. Adding a bank row to the ledger IS transaction
// entry, and it was three taps deep here. What stays is connecting a bank,
// mapping its accounts and watching consent expiry: settings.

export default function Banking() {
  const { data: settings, isLoading: settingsLoading } = useBankSettings()
  const configured = !!settings?.configured
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [picker, setPicker] = useState(false)
  const [pasteOpen, setPasteOpen] = useState(false)
  const [pasted, setPasted] = useState('')

  const connections = useBankConnections(configured)
  // Counted only to point at where the queue actually is.
  const waiting = useStagedTransactions({ state: 'staged', page_size: 1 }, configured)
  const pending = waiting.data?.total ?? 0
  const aspsps = useASPSPs(picker && configured)
  const connect = useConnectBank()
  const callback = useBankCallback()

  const startConnect = async (name: string, country: string) => {
    const res = await connect.mutateAsync({ name, country })
    setPicker(false)
    // The bank authorises in its own page and sends the browser back to the
    // registered redirect. Nothing calls our server, so there is no inbound
    // endpoint to expose.
    window.location.href = res.url
  }

  if (settingsLoading) return <LoadingSpinner />

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-2xl mx-auto">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">🔗 Bank connections</h1>
          <p className="text-xs text-gray-400">
            Connect a bank; review and add what it sends on the Transactions page
          </p>
        </div>
        <button
          onClick={() => setSettingsOpen(true)}
          className="text-sm text-gray-400 hover:text-gray-700 shrink-0"
        >
          ⚙️ Settings
        </button>
      </div>

      {!configured && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-sm text-gray-700">Not set up yet.</p>
          <p className="text-xs text-gray-400 mt-1">
            This connects through Enable Banking, a licensed account-information provider —
            banks don't allow direct access without a licence. Add your application credentials
            to get started.
          </p>
          <button
            onClick={() => setSettingsOpen(true)}
            className="mt-3 px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700"
          >
            Add credentials
          </button>
        </div>
      )}

      {/* Open by default with nothing set up — the credentials do not exist
          until you have walked the control panel, so the form on its own is a
          dead end. Collapsed once configured, because by then it is reference. */}
      {!configured && <BankSetupGuide defaultOpen />}

      {configured && (
        <>
          {connections.isLoading && <LoadingSpinner />}
          {connections.isError && <QueryError error={connections.error} onRetry={() => connections.refetch()} />}

          {connections.data?.connections.map((conn) => (
            <BankConnectionCard
              key={conn.id}
              conn={conn}
              validAccountKeys={connections.data.valid_account_keys}
              onReconnect={startConnect}
            />
          ))}

          <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
            {!picker ? (
              <div className="flex flex-wrap items-center gap-2">
                <button
                  onClick={() => setPicker(true)}
                  className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700"
                >
                  + Connect a bank
                </button>
                <button
                  onClick={() => setPasteOpen((p) => !p)}
                  className="px-3 py-1.5 text-sm text-gray-500 hover:text-gray-800"
                >
                  Paste a return address
                </button>
              </div>
            ) : (
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h3 className="text-sm font-medium text-gray-800">Pick your bank</h3>
                  <button onClick={() => setPicker(false)} className="text-xs text-gray-400">
                    Cancel
                  </button>
                </div>
                {aspsps.isLoading && <LoadingSpinner />}
                {aspsps.isError && <QueryError error={aspsps.error} onRetry={() => aspsps.refetch()} />}
                <div className="divide-y divide-gray-50">
                  {aspsps.data?.map((b) => (
                    <button
                      key={`${b.name}-${b.country}`}
                      onClick={() => startConnect(b.name, b.country)}
                      disabled={connect.isPending}
                      className="w-full flex items-center gap-3 py-2.5 text-left hover:bg-gray-50 disabled:opacity-50"
                    >
                      {b.logo && <img src={b.logo} alt="" className="w-6 h-6 object-contain" />}
                      <span className="flex-1 min-w-0">
                        <span className="block text-sm text-gray-800 truncate">{b.name}</span>
                        <span className="block text-xs text-gray-400">
                          {b.country}
                          {b.beta && ' · beta'}
                          {b.sandbox && ' · sandbox'}
                        </span>
                      </span>
                      <span className="text-gray-300">›</span>
                    </button>
                  ))}
                </div>
                {connect.isError && (
                  <p className="text-xs text-red-600 mt-2">
                    {connect.error instanceof Error ? connect.error.message : 'Could not start'}
                  </p>
                )}
              </div>
            )}

            {/* The auto-redirect is the convenience; this is the path that
                always works — over Tailscale, on the LAN, or whenever the
                bank cannot reach this host. */}
            {pasteOpen && (
              <div className="mt-3 pt-3 border-t border-gray-50">
                <label className="block text-xs text-gray-500">
                  Paste the address your browser landed on after approving at the bank
                  <input
                    value={pasted}
                    onChange={(e) => setPasted(e.target.value)}
                    placeholder="https://…/?code=…&state=…"
                    autoComplete="off"
                    className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
                  />
                </label>
                <button
                  onClick={async () => {
                    await callback.mutateAsync({ url: pasted.trim() })
                    setPasted('')
                    setPasteOpen(false)
                  }}
                  disabled={!pasted.trim() || callback.isPending}
                  className="mt-2 px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
                >
                  {callback.isPending ? 'Finishing…' : 'Finish connecting'}
                </button>
                {callback.isError && (
                  <p className="text-xs text-red-600 mt-2">
                    {callback.error instanceof Error ? callback.error.message : 'Could not finish'}
                  </p>
                )}
              </div>
            )}
          </div>

          <BankSetupGuide />

          {/* Reviewing, putting back and checking what was added all happen
              on the transactions page now — this page is where a bank gets
              connected, mapped and watched for consent expiry. */}
          <Link
            to="/transactions"
            className="flex items-center gap-2 bg-indigo-50 border border-indigo-100 rounded-xl px-3 py-2.5 hover:bg-indigo-100"
          >
            <span className="text-lg">📥</span>
            <span className="flex-1 min-w-0 text-sm text-indigo-800">
              {pending > 0 ? (
                <>
                  <b>{pending}</b> row{pending === 1 ? '' : 's'} waiting to be reviewed
                </>
              ) : (
                'Bank rows, dismissed and added'
              )}
              <span className="block text-xs text-indigo-500">Review and add them on Transactions</span>
            </span>
            <span className="text-indigo-300">›</span>
          </Link>
        </>
      )}

      {settingsOpen && <BankSettingsModal onClose={() => setSettingsOpen(false)} />}
    </div>
  )
}
