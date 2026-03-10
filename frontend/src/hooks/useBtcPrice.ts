import { useQuery } from '@tanstack/react-query'

async function fetchBtcEurPrice(): Promise<number> {
  const res = await fetch(
    'https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=eur'
  )
  if (!res.ok) throw new Error('Failed to fetch BTC price')
  const data = await res.json()
  return data.bitcoin.eur as number
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
