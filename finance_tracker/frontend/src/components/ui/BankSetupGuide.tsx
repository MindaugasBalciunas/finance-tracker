import { useState } from 'react'

// BankSetupGuide is the missing half of the credentials form. The form asks
// for an application id and a .pem; neither exists until you have walked
// Enable Banking's control panel, and the two steps people lose an evening to
// — the environment being unchangeable, and "linking" not being the same as
// "authorising" — are nowhere near the fields they bite.
//
// Everything here was checked against Enable Banking's own docs (control
// panel, linked accounts, markets/lt, sandbox credentials). Where their docs
// are silent the wording hedges rather than inventing a rule.

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="flex gap-3">
      <span className="shrink-0 w-6 h-6 rounded-full bg-indigo-600 text-white text-xs font-semibold flex items-center justify-center">
        {n}
      </span>
      <div className="min-w-0 flex-1 pb-4">
        <h4 className="text-sm font-medium text-gray-900">{title}</h4>
        <div className="mt-1 text-xs text-gray-500 space-y-1.5 leading-relaxed">{children}</div>
      </div>
    </li>
  )
}

function Copyable({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-stretch gap-1.5 mt-1">
      <code className="flex-1 min-w-0 px-2 py-1.5 bg-gray-50 border border-gray-200 rounded-lg text-xs text-gray-800 break-all">
        {value}
      </code>
      <button
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value)
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
          } catch {
            // Clipboard is blocked over plain http on some browsers — the
            // value is on screen and selectable, so this is not worth an error.
            setCopied(false)
          }
        }}
        className="shrink-0 px-2 text-xs rounded-lg border border-gray-200 text-gray-500 hover:bg-gray-50"
      >
        {copied ? 'Copied' : 'Copy'}
      </button>
    </div>
  )
}

export default function BankSetupGuide({ defaultOpen = false }: { defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen)
  // The address this app is actually being served from. The bank sends the
  // browser back here, so this is the string that has to be registered —
  // guessing it is the single most common way the connect step fails.
  const redirect = `${window.location.origin}/`

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center justify-between gap-2 p-4 text-left"
      >
        <span>
          <span className="block text-sm font-medium text-gray-900">
            How to connect Swedbank or SEB
          </span>
          <span className="block text-xs text-gray-400">
            Getting an application ID and private key from Enable Banking
          </span>
        </span>
        <span className="text-gray-300 shrink-0">{open ? '▴' : '▾'}</span>
      </button>

      {open && (
        <div className="px-4 pb-4">
          <ol className="space-y-0">
            <Step n={1} title="Register an application">
              <p>
                Sign in at{' '}
                <a
                  href="https://enablebanking.com/cp"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-indigo-600 underline"
                >
                  enablebanking.com/cp
                </a>{' '}
                — enter your email and follow the one-time link; the account is created on first
                sign-in. Open the applications page and press <b>Add a new application</b>.
              </p>
              <p>
                <b>Environment</b> — pick <b>Production</b> for your real accounts, or{' '}
                <b>Sandbox</b> to rehearse first. An application cannot be moved between the two
                later, so to do both you register two applications and swap the credentials here.
              </p>
              <p>
                <b>Private key generation</b> — leave the default,{' '}
                <i>Generate in the browser and export private key</i>.
              </p>
              <p>
                <b>Allowed redirect URLs</b> — this app's own address, exactly as written, trailing
                slash included:
              </p>
              <Copyable value={redirect} />
              <p>
                If you reach this app at more than one address (LAN, Tailscale, DuckDNS), add each
                one — the field takes a list — and set the matching one below before connecting.
                Production applications require <code>https://</code>.
              </p>
              <p>Press <b>Register</b>.</p>
            </Step>

            <Step n={2} title="Find the application ID and the key file">
              <p>
                A <code>.pem</code> file downloads as you register. <b>Its filename is the
                application ID</b> — a UUID like <code>aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.pem</code>,
                so the ID is that name without the <code>.pem</code>.
              </p>
              <p>
                Enable Banking never receives this key, which also means they cannot re-issue it.
                Keep the file.
              </p>
              <p>
                Open <b>⚙️ Settings</b> above, paste the ID into <b>Application ID</b>, choose the{' '}
                <code>.pem</code> for <b>Private key</b>, set the environment to match the
                application, and save.
              </p>
            </Step>

            <Step n={3} title="Production only — activate by linking your accounts">
              <p>
                A new production application sits at <b>Inactive</b> and can read nothing. Press{' '}
                <b>Activate by linking accounts</b> in the control panel and go through your bank.
                It flips to <b>Active</b> in restricted mode.
              </p>
              <p>
                No contract, no company details and no support ticket — restricted mode is a
                self-service button, and Enable Banking's terms cover production use for a private
                individual's own accounts.
              </p>
              <p className="text-amber-700 bg-amber-50 border border-amber-100 rounded-lg px-2 py-1.5">
                Link <b>every</b> account you want to import. Accounts you did not link are silently
                stripped from the API's responses — they do not error, they just never appear.
              </p>
              <p className="text-amber-700 bg-amber-50 border border-amber-100 rounded-lg px-2 py-1.5">
                Linking is <b>not</b> authorising. After activating you still have to come back here
                and press <b>+ Connect a bank</b>, even for the very same account.
              </p>
            </Step>

            <Step n={4} title="Connect the bank">
              <p>
                Press <b>+ Connect a bank</b> and pick <b>Swedbank</b> or <b>SEB</b> — the list is
                read live from Enable Banking, Lithuania first.
              </p>
              <p>
                <b>Swedbank LT</b> signs you in with Smart-ID or Mobile-ID. <b>SEB LT</b> also
                accepts the SEB mobile app or a Digipass. Lithuanian banks do not hand off to their
                phone apps automatically, so expect to type your personal code or phone number on
                the bank's page before the SCA prompt.
              </p>
              <p>
                The bank returns you here and the connection finishes by itself. If it cannot reach
                this app — over Tailscale, say — copy the address your browser landed on and use{' '}
                <b>Paste a return address</b> instead.
              </p>
            </Step>

            <Step n={5} title="Map the accounts, then sync small">
              <p>
                Each account gets a dropdown — tell it which of your accounts it is. Anything left
                at <i>Don't sync</i> is ignored.
              </p>
              <p>
                Press <b>Last 7 days</b> before <b>Sync now</b>. The short window lets you check
                that the dates and descriptions match what the CSV import produced for the same
                shops, while there is almost nothing to wade through. Only then pull the full 90
                days.
              </p>
              <p>
                Consent runs out on the bank's own schedule — the card counts the days down, and
                reconnecting is the same walk through step 4.
              </p>
            </Step>
          </ol>

          <p className="text-xs text-gray-400 border-t border-gray-50 pt-3">
            Rehearsing in the sandbox? Swedbank LT needs no credentials there — you pick a test user
            from a dropdown. SEB has no sandbox at all, so it can only be tried against a production
            application.
          </p>
        </div>
      )}
    </div>
  )
}
