import AINav from '../components/ui/AINav'
import AIInsightCard from '../components/ui/AIInsightCard'
import AIUnavailable from '../components/ui/AIUnavailable'
import { useAIAvailable } from '../hooks/useInsights'

// AI Financial Overview: the per-section analysis (now with a detailed
// category review). The provider configuration is reachable from the ⚙️
// button in the shared AINav bar; the chat lives on its own tab.
export default function AIOverview() {
  const available = useAIAvailable()
  if (!available) {
    return (
      <div className="p-4 sm:p-6 space-y-4 max-w-4xl mx-auto">
        <AINav />
        <AIUnavailable />
      </div>
    )
  }

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-4xl mx-auto">
      <AINav />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">✦ AI Financial Overview</h1>
          <p className="text-xs text-gray-400">
            Per-section analysis with detailed category reviews
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
