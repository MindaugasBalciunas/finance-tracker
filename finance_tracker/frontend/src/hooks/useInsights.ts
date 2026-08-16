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
