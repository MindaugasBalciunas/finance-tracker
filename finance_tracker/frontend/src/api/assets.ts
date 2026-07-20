import client from './client'
import type { Asset, CreateAssetInput, AssetSummary } from '../types'

export const assetsApi = {
  listAll: () =>
    client.get<Asset[]>('/assets').then((r) => r.data),

  create: (input: CreateAssetInput) =>
    client.post<Asset>('/assets', input).then((r) => r.data),

  update: (id: number, input: CreateAssetInput) =>
    client.put<Asset>(`/assets/${id}`, input).then((r) => r.data),

  delete: (id: number) =>
    client.delete(`/assets/${id}`),

  getSummary: () =>
    client.get<AssetSummary>('/assets/summary').then((r) => r.data),
}
