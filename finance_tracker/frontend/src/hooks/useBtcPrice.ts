import { useQuery } from '@tanstack/react-query'

async function fetchBtcEurPrice(): Promise<number | null> {
  const res = await fetch(
    'https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=eur',
    { signal: AbortSignal.timeout(10_000) }
  )
  if (!res.ok) throw new Error('Failed to fetch BTC price')
  const data = await res.json()
  const price = data?.bitcoin?.eur
  // Guard the response shape — a malformed payload yields null, not a TypeError.
  return typeof price === 'number' && Number.isFinite(price) ? price : null
}

export function useBtcPrice() {
  return useQuery({
    queryKey: ['btc-price'],
    queryFn: fetchBtcEurPrice,
    staleTime: 5 * 60 * 1000, // refresh every 5 min
    retry: 1,
  })
}

/** Returns live EUR/BTC price (null when unavailable or still loading). */
export function useBtcEur(): { price: number | null } {
  const { data: price } = useBtcPrice()
  return { price: price ?? null }
}
