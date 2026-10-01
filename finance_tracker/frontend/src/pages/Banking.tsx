import { useState } from 'react'
import BankConnectionCard from '../components/ui/BankConnectionCard'
import BankSettingsModal from '../components/ui/BankSettingsModal'
import BankStagingList from '../components/ui/BankStagingList'
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

type Tab = 'staged' | 'dismissed' | 'imported'

export default function Banking() {
  const { data: settings, isLoading: settingsLoading } = useBankSettings()
  const configured = !!settings?.configured
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [picker, setPicker] = useState(false)
  const [tab, setTab] = useState<Tab>('staged')
  const [pasteOpen, setPasteOpen] = useState(false)
  const [pasted, setPasted] = useState('')

  const connections = useBankConnections(configured)
  const staged = useStagedTransactions({ state: tab }, configured)
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
          <h1 className="text-xl font-bold text-gray-900">🏦 Bank connections</h1>
          <p className="text-xs text-gray-400">
            Pull recent transactions from your bank and add them one at a time
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

          <div>
            <div className="flex items-center gap-1 mb-2 overflow-x-auto">
              {(['staged', 'dismissed', 'imported'] as Tab[]).map((t) => (
                <button
                  key={t}
                  onClick={() => setTab(t)}
                  className={`px-3 py-1.5 text-sm rounded-lg capitalize whitespace-nowrap ${
                    tab === t ? 'bg-gray-900 text-white' : 'text-gray-500 hover:bg-gray-100'
                  }`}
                >
                  {t === 'staged' ? 'To review' : t}
                  {t === 'staged' && staged.data?.total ? ` (${staged.data.total})` : ''}
                </button>
              ))}
            </div>

            {staged.isLoading && <LoadingSpinner />}
            {staged.isError && <QueryError error={staged.error} onRetry={() => staged.refetch()} />}
            {staged.data && (
              <BankStagingList
                rows={staged.data.transactions}
                validAccountKeys={connections.data?.valid_account_keys ?? []}
              />
            )}
          </div>
        </>
      )}

      {settingsOpen && <BankSettingsModal onClose={() => setSettingsOpen(false)} />}
    </div>
  )
}
