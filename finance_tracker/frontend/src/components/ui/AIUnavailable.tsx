import { useAIUnavailableReason } from '../../hooks/useInsights'

// The placeholder an AI page shows instead of its content when AI can't run.
// Switched off and never configured need different next steps, so it says
// which one it is — and in both cases the way out is the ⚙️ in the AI
// sub-nav directly above it.
export default function AIUnavailable() {
  const reason = useAIUnavailableReason()
  if (!reason) return null

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6 text-center">
      <div className="text-2xl mb-2">✦</div>
      <h2 className="text-base font-semibold text-gray-900">
        {reason === 'off' ? 'AI features are switched off' : 'AI is not set up yet'}
      </h2>
      <p className="text-sm text-gray-400 mt-1 max-w-sm mx-auto">
        {reason === 'off'
          ? 'Insights, chat, receipt scanning, auto-labelling and forecasts are hidden. Your provider settings are kept — turn AI back on with the ⚙️ button above.'
          : 'Choose a provider (nexos.ai gateway or the Claude API) and add your API key with the ⚙️ button above.'}
      </p>
    </div>
  )
}
