import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { insightsApi, aiApi } from '../api/insights'

const INSIGHTS_KEY = 'insights'

export function useLatestInsight() {
  return useQuery({
    queryKey: [INSIGHTS_KEY, 'latest'],
    queryFn: insightsApi.getLatest,
    retry: false,
  })
}

export function useGenerateInsight() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (range?: { date_from?: string; date_to?: string }) => insightsApi.generate(range),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [INSIGHTS_KEY] })
    },
  })
}

export function useAISettings() {
  return useQuery({
    queryKey: ['ai-settings'],
    queryFn: aiApi.getSettings,
  })
}

export function useSaveAISettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: aiApi.saveSettings,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['ai-settings'] }),
  })
}

// useAIAvailable is the single question every AI surface asks before
// rendering: is AI switched on AND configured? Keeping it in one place is
// what stops a new AI feature from quietly ignoring the master switch.
//
// While the settings are still loading it answers false — a nav item or card
// that flashes in and then disappears is worse than one that appears a beat
// late.
export function useAIAvailable(): boolean {
  const { data } = useAISettings()
  return !!data?.enabled && !!data?.has_key && !!data?.model
}

// Why AI isn't available, when it isn't — the two cases need different next
// steps from the user ("turn it on" vs "add a key"), so surfaces that explain
// themselves ask this instead of useAIAvailable.
export function useAIUnavailableReason(): 'off' | 'unconfigured' | null {
  const { data } = useAISettings()
  if (!data) return 'unconfigured'
  if (!data.enabled) return 'off'
  if (!data.has_key || !data.model) return 'unconfigured'
  return null
}
