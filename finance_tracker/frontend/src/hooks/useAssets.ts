import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { assetsApi } from '../api/assets'
import type { CreateAssetInput } from '../types'

const ASSETS_KEY = ['assets']
const SUMMARY_KEY = ['assets-summary']

export function useAssets() {
  return useQuery({ queryKey: ASSETS_KEY, queryFn: assetsApi.listAll })
}

export function useAssetSummary() {
  return useQuery({ queryKey: SUMMARY_KEY, queryFn: assetsApi.getSummary })
}

function useInvalidateAssets() {
  const qc = useQueryClient()
  return () => {
    qc.invalidateQueries({ queryKey: ASSETS_KEY })
    qc.invalidateQueries({ queryKey: SUMMARY_KEY })
  }
}

export function useCreateAsset() {
  const invalidate = useInvalidateAssets()
  return useMutation({
    mutationFn: (input: CreateAssetInput) => assetsApi.create(input),
    onSuccess: invalidate,
  })
}

export function useUpdateAsset() {
  const invalidate = useInvalidateAssets()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: CreateAssetInput }) =>
      assetsApi.update(id, input),
    onSuccess: invalidate,
  })
}

export function useDeleteAsset() {
  const invalidate = useInvalidateAssets()
  return useMutation({
    mutationFn: (id: number) => assetsApi.delete(id),
    onSuccess: invalidate,
  })
}
