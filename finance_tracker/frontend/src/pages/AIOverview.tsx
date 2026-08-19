import { useAISettings } from '../hooks/useInsights'
import AINav from '../components/ui/AINav'
import AIInsightCard from '../components/ui/AIInsightCard'

// AI Financial Overview: the per-section analysis (now with a detailed
// category review). The nexos.ai gateway configuration is reachable from the
// ⚙️ button in the shared AINav bar; the chat lives on its own tab.
export default function AIOverview() {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-4xl mx-auto">
      <AINav />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">✦ AI Financial Overview</h1>
          <p className="text-xs text-gray-400">
            Per-section analysis with detailed category reviews
            {configured && settings?.model && <> · <span className="text-indigo-500">{settings.model}</span></>}
          </p>
        </div>
      </div>

      <AIInsightCard />


      <p className="text-[11px] text-gray-400">
        Each analysis sends your aggregated financial summary (scoped to the app's date range) to the
        configured gateway. Conversations and analyses are stored in your own database.
      </p>
    </div>
  )
}
