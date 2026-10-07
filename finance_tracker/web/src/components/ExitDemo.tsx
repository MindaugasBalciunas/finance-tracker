import { useState } from 'react'
import { useDemo } from '../lib/hooks'
import { PinInput, Sheet, Spinner, useToast } from './ui'

/** Leaving demo mode: straight away when no PIN is set, otherwise only with
 *  the owner's PIN — so a borrowed phone stays in the demo. */
export function useExitDemo() {
  const demo = useDemo()
  const toast = useToast()
  const [asking, setAsking] = useState(false)
  const exit = async () => {
    if (demo.protected) return setAsking(true)
    try { await demo.switchTo(false); toast('Back to your data', 'good') } catch (e) { toast((e as Error).message, 'bad') }
  }
  const sheet = asking ? <PinSheet onClose={() => setAsking(false)} /> : null
  return { exit, sheet }
}

function PinSheet({ onClose }: { onClose: () => void }) {
  const demo = useDemo()
  const toast = useToast()
  const [pin, setPin] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async () => {
    if (pin.length < 4 || busy) return
    setBusy(true); setErr('')
    try {
      await demo.switchTo(false, pin)
      toast('Back to your data', 'good')
      onClose()
    } catch (e) {
      setErr((e as Error).message); setPin('')
    } finally { setBusy(false) }
  }
  return (
    <Sheet open onClose={onClose} title="Leave demo mode" footer={<>
      <button className="btn-ghost" onClick={onClose}>Stay in demo</button>
      <button className="btn-primary" onClick={submit} disabled={pin.length < 4 || busy}>{busy && <Spinner className="h-4 w-4" />}Show my data</button>
    </>}>
      <p className="mb-3 text-sm text-ink2">Your real finances are behind your PIN while the demo is on.</p>
      <PinInput center className="text-center text-2xl tnum" autoFocus
        value={pin} onChange={setPin} onKeyDown={(e) => e.key === 'Enter' && submit()} aria-label="PIN" placeholder="PIN" />
      {err && <div className="mt-2 text-sm text-bad">{err}</div>}
    </Sheet>
  )
}
