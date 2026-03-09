import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { insightsApi } from '../api/insights'

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
    mutationFn: insightsApi.generate,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [INSIGHTS_KEY] })
    },
  })
}
