import { useQuery } from '@tanstack/react-query'

// Fixed BTC holdings — update these when holdings change
export const BTC_HOLDINGS = {
  rev_m: 0.0091,
  rev_r: 0.017,
}

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
    staleTime: 5 * 60 * 1000,   // refresh every 5 min
    retry: 1,
  })
}

export function useBtcEur() {
  const { data: price } = useBtcPrice()
  if (!price) return { m_btc: null, r_btc: null, total: null, price: null }
  return {
    price,
    m_btc: BTC_HOLDINGS.rev_m * price,
    r_btc: BTC_HOLDINGS.rev_r * price,
    total: (BTC_HOLDINGS.rev_m + BTC_HOLDINGS.rev_r) * price,
  }
}
