import { Link } from 'react-router-dom'
import { useCats } from '../lib/categories'
import { eur } from '../lib/format'
import { ledgerLink } from '../lib/links'
import type { CommitmentTotal } from '../lib/types'

/** The fixed commitments a "biggest expenses" or "top merchants" list leaves
 *  out — alimony, the mortgage, family support, finance costs — as one line of
 *  totals under it, so €1,000 of alimony every month doesn't fill the list but
 *  isn't hidden either. Each total opens its rows in the Ledger. */
export function CommitmentsLine({ items, from, to }: { items: CommitmentTotal[] | undefined; from: string; to: string }) {
  const cats = useCats()
  const list = (items ?? []).filter((c) => c.total > 0.005)
  if (!list.length) return null
  const shown = list.slice(0, 4)
  const rest = list.slice(4).reduce((a, c) => a + c.total, 0)
  return (
    <p className="border-t border-line px-4 py-2.5 text-xs leading-relaxed text-muted">
      <span className="text-ink2">Fixed commitments, not listed:</span>{' '}
      {shown.map((c, i) => (
        <span key={c.category}>
          {i > 0 && ' · '}
          <Link to={ledgerLink({ category: c.category, commitments: true, from, to })} className="hover:text-ink">
            {cats.name(c.category)} <span className="tnum">{eur(c.total)}</span>
          </Link>
        </span>
      ))}
      {rest > 0.005 && <> · other <span className="tnum">{eur(rest)}</span></>}
    </p>
  )
}
