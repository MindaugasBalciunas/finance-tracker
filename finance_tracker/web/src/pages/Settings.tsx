import { useState } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom'
import { PageHeader, Spinner, Tabs, useToast } from '../components/ui'
import { Icon } from '../components/Icon'
import { AccountsVisibility } from './settings/Accounts'
import { Categories } from './settings/Categories'
import { Rules } from './settings/Rules'
import { Tags } from './settings/Tags'
import { Banks } from './settings/Banks'
import { AISettings } from './settings/AI'
import { Security } from './settings/Security'
import { Appearance, Data } from './settings/Data'
import { UsageView } from './settings/Usage'

const SECTIONS = [
  { to: 'categories', label: 'Categories' }, { to: 'accounts', label: 'Accounts' }, { to: 'rules', label: 'Rules' }, { to: 'tags', label: 'Tags' }, { to: 'banks', label: 'Banks' },
  { to: 'ai', label: 'AI' }, { to: 'security', label: 'Security' }, { to: 'data', label: 'Data & backup' }, { to: 'usage', label: 'Usage' }, { to: 'appearance', label: 'Appearance' },
]

// A stray deep path (e.g. an old link that stacked sections) lands on its
// last known section instead of a blank page.
function UnknownSection() {
  const rest = useParams()['*'] || ''
  const last = rest.split('/').reverse().find((seg) => SECTIONS.some((s) => s.to === seg))
  return <Navigate to={`/settings/${last || 'categories'}`} replace />
}

const AI_SHARE_TEXT = 'The attached zip is my complete finances. Open PROMPT.md and follow it, using README.md for the file formats.'

/** The everyday hand-off: the whole dataset to any AI assistant in one tap. */
function ShareForAI() {
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const share = async () => {
    setBusy(true)
    try {
      const res = await fetch('api/export/ai.zip', { credentials: 'same-origin' })
      if (!res.ok) throw new Error(`Export failed (${res.status})`)
      const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'finance-for-ai.zip'
      const file = new File([await res.blob()], name, { type: 'application/zip' })
      // Phones: the share sheet goes straight to an AI app. Elsewhere: download.
      if (navigator.canShare?.({ files: [file] })) {
        try { await navigator.share({ files: [file], title: 'Finances for AI', text: AI_SHARE_TEXT }) } catch (e: any) { if (e?.name !== 'AbortError') throw e }
      } else {
        const url = URL.createObjectURL(file)
        const a = Object.assign(document.createElement('a'), { href: url, download: name })
        document.body.append(a); a.click(); a.remove()
        setTimeout(() => URL.revokeObjectURL(url), 10_000)
        toast('Saved — attach it in your AI assistant', 'good')
      }
    } catch (e: any) {
      toast(e?.message ?? 'Export failed', 'bad')
    } finally { setBusy(false) }
  }
  return (
    <section className="card mb-4 flex flex-wrap items-center gap-3 p-4">
      <span className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-accent/10 text-accent"><Icon name="spark" /></span>
      <div className="min-w-0 flex-1 basis-48">
        <div className="text-sm font-medium">Export for AI</div>
        <div className="text-xs text-muted">Every transaction, balance, loan and plan as one zip with a start-here PROMPT.md — for any AI assistant.</div>
      </div>
      <div className="flex w-full sm:w-auto">
        <button className="btn-primary flex-1 sm:flex-none" onClick={share} disabled={busy}>{busy ? <Spinner /> : <Icon name="share" size={16} />}Export zip</button>
      </div>
    </section>
  )
}

export default function Settings() {
  const nav = useNavigate()
  const seg = useLocation().pathname.split('/')[2] || 'categories'
  const tab = SECTIONS.some((s) => s.to === seg) ? seg : 'categories'
  return (
    <div>
      <PageHeader title="Settings" />
      <ShareForAI />
      <Tabs value={tab} onChange={(v) => nav(`/settings/${v}`)} tabs={SECTIONS.map((s) => ({ value: s.to, label: s.label }))} />
      <Routes>
        <Route path="/" element={<Categories />} />
        <Route path="categories" element={<Categories />} />
        <Route path="accounts" element={<AccountsVisibility />} />
        <Route path="rules" element={<Rules />} />
        <Route path="tags" element={<Tags />} />
        <Route path="banks" element={<Banks />} />
        <Route path="ai" element={<AISettings />} />
        <Route path="security" element={<Security />} />
        <Route path="data" element={<Data />} />
        <Route path="usage" element={<UsageView />} />
        <Route path="appearance" element={<Appearance />} />
        <Route path="*" element={<UnknownSection />} />
      </Routes>
    </div>
  )
}
