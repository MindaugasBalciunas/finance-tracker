import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { logoKey, logoReplaced, MerchantMark, probeLogo, useLogoIndex, useMissingLogos } from '../../components/MerchantLogo'
import { Card, Segmented, useToast } from '../../components/ui'

type MerchantCount = { merchant: string; count: number; category: string }
type Show = 'used' | 'none' | 'yours'

const idOf = (index: Record<string, string> | undefined, m: string) => index?.[logoKey(m)] ?? index?.[logoKey(m.split(/\s+/)[0] ?? '')]

/** Settings → Logos: what each merchant shows, and your own picture for any
 *  whose website won't share its icon (VMI, Bolt) or gave the wrong one. */
export function Logos() {
  const { data: index } = useLogoIndex()
  const { data: merchants } = useQuery({ queryKey: ['merchants', 'all'], queryFn: () => api.get<MerchantCount[]>('/merchants'), staleTime: 300_000 })
  const missing = useMissingLogos()
  const [q, setQ] = useState('')
  const [show, setShow] = useState<Show>('used')
  // The merchants people pay most: the ones a logo is worth adding for.
  const top = useMemo(() => [...(merchants ?? [])].sort((a, b) => b.count - a.count).slice(0, 100), [merchants])
  // Ask once whether each known site shares its icon, so "No logo" is
  // complete without scrolling through every row first.
  useEffect(() => { top.forEach((m) => { const id = idOf(index, m.merchant); if (id) probeLogo(id) }) }, [top, index])
  const none = (m: MerchantCount) => { const id = idOf(index, m.merchant); return !id || missing.has(id) }
  const yours = (m: MerchantCount) => !!idOf(index, m.merchant)?.startsWith('custom-')
  const noneCount = top.filter(none).length
  const list = useMemo(() => {
    const f = logoKey(q)
    // A search looks through every merchant, not only the top ones.
    const pool = f ? [...(merchants ?? [])].sort((a, b) => b.count - a.count).filter((m) => logoKey(m.merchant).includes(f)) : top
    return pool.filter(show === 'none' ? none : show === 'yours' ? yours : () => true).slice(0, 60)
  }, [merchants, top, q, show, index, missing.size])
  return (
    <Card title="Merchant logos" icon="image" color="var(--s1)">
      <p className="-mt-1 text-xs text-muted">
        Logos come from each merchant's own website. Where a site won't share one — or the picture is wrong — upload your own (PNG, JPEG, SVG, up to 256 KB); it's kept in your backups.
        {noneCount > 0 && <> {noneCount} of your {top.length} most-used merchants show a category icon instead.</>}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Segmented size="sm" value={show} onChange={setShow}
          options={[{ value: 'used', label: 'Most used' }, { value: 'none', label: `No logo${noneCount ? ` · ${noneCount}` : ''}` }, { value: 'yours', label: 'Yours' }]} />
        <input className="input w-full sm:w-auto sm:min-w-0 sm:flex-1" placeholder="Find any merchant" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Find a merchant" />
      </div>
      <ul className="mt-2 grid lg:grid-cols-2 lg:gap-x-8">
        {list.map((m) => <Row key={m.merchant} m={m} id={idOf(index, m.merchant)} missing={!!idOf(index, m.merchant) && missing.has(idOf(index, m.merchant)!)} />)}
      </ul>
      {list.length === 0 && <p className="py-6 text-center text-sm text-muted">{show === 'yours' ? 'No logos of your own yet.' : show === 'none' ? 'Every merchant here has a logo.' : 'No merchant matches.'}</p>}
    </Card>
  )
}

function Row({ m, id, missing }: { m: MerchantCount; id?: string; missing: boolean }) {
  const qc = useQueryClient()
  const toast = useToast()
  const file = useRef<HTMLInputElement>(null)
  const custom = id?.startsWith('custom-')
  const done = (newId: string) => { logoReplaced(newId); qc.invalidateQueries({ queryKey: ['logos'] }) }
  const upload = useMutation({
    mutationFn: async (f: File) => {
      if (f.size > 256 * 1024) throw new Error('Pick an image under 256 KB')
      const data = await new Promise<string>((res, rej) => {
        const r = new FileReader()
        r.onload = () => res(String(r.result).split(',')[1] ?? '')
        r.onerror = () => rej(new Error('Could not read the file'))
        r.readAsDataURL(f)
      })
      return api.put<{ id: string }>('/logos/custom', { merchant: m.merchant, data })
    },
    onSuccess: (r) => { done(r.id); toast(`${m.merchant}'s logo set`, 'good') },
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  const reset = useMutation({
    mutationFn: () => api.del(`/logos/custom/${encodeURIComponent(id!.slice('custom-'.length))}`),
    onSuccess: () => { done(id!); toast('Back to the automatic logo', 'good') },
  })
  return (
    <li className="flex items-center gap-3 border-b border-line py-2">
      <MerchantMark merchant={m.merchant} category={m.category} size={32} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{m.merchant}</div>
        <div className="text-xs text-muted">{custom ? 'your logo' : id && missing ? <span className="text-warn">its website won't share one — upload it</span> : id ? 'from its website' : 'no logo — the category icon'} · {m.count} payments</div>
      </div>
      <input ref={file} type="file" accept="image/png,image/jpeg,image/svg+xml,image/x-icon,image/webp,image/gif" hidden
        onChange={(e) => { const f = e.target.files?.[0]; if (f) upload.mutate(f); e.target.value = '' }} />
      {custom && <button className="btn-ghost h-8 px-2 text-xs" onClick={() => reset.mutate()} disabled={reset.isPending}>Reset</button>}
      <button className="btn-outline h-8 px-3 text-xs" onClick={() => file.current?.click()} disabled={upload.isPending} aria-label={`Upload a logo for ${m.merchant}`}>{custom ? 'Replace' : 'Upload'}</button>
    </li>
  )
}
