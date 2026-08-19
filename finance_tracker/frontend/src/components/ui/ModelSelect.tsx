import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi } from '../../api/insights'
import { useAISettings, useSaveAISettings } from '../../hooks/useInsights'

// Compact model switcher for the shared AI sub-nav bar. Reads the active model
// from the saved gateway settings and lets the user swap it in one click from
// any AI tab — the choice is persisted (URL kept as-is) and the ai-settings
// query invalidated so every AI page reflects the switch immediately.
export default function ModelSelect() {
  const qc = useQueryClient()
  const { data: settings } = useAISettings()
  const save = useSaveAISettings()
  // Cached catalogue — shared with GatewaySettings' own fetch conceptually,
  // but keyed here so tab-hopping doesn't refetch on every mount.
  const { data: models } = useQuery({
    queryKey: ['ai-models'],
    queryFn: aiApi.models,
    staleTime: 5 * 60 * 1000,
  })

  // Nothing to choose until the gateway is configured with a model.
  if (!settings?.gateway_url || !settings?.model) return null

  const list = models && models.ok ? models.models : []

  // No catalogue available — just surface which model is in use, no switcher.
  if (list.length === 0) {
    return (
      <span className="min-w-0 max-w-[8rem] truncate text-xs text-gray-400" title={settings.model}>
        🧠 {settings.model}
      </span>
    )
  }

  // Keep the active model selectable even if the gateway dropped it from its
  // catalogue (a custom/legacy value).
  const options = list.includes(settings.model) ? list : [settings.model, ...list]

  const onChange = (model: string) => {
    if (!model || model === settings.model) return
    save.mutate(
      { gateway_url: settings.gateway_url, model },
      { onSuccess: () => qc.invalidateQueries({ queryKey: ['ai-settings'] }) },
    )
  }

  return (
    <label className="min-w-0 flex items-center gap-1 text-xs text-gray-400" title="AI model">
      <span aria-hidden>🧠</span>
      <select
        value={settings.model}
        onChange={(e) => onChange(e.target.value)}
        disabled={save.isPending}
        aria-label="AI model"
        className="min-w-0 max-w-[7rem] sm:max-w-[12rem] truncate text-xs text-gray-600 bg-white border border-gray-200 rounded-lg px-2 py-1 hover:text-gray-800 disabled:opacity-50"
      >
        {options.map((m) => (
          <option key={m} value={m}>
            {m}
          </option>
        ))}
      </select>
      {save.isPending && (
        <span className="inline-block animate-spin" aria-hidden>
          ↻
        </span>
      )}
    </label>
  )
}
