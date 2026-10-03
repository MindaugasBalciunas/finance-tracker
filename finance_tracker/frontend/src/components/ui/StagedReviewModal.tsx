import { useState } from 'react'
import type { StagedTx } from '../../api/banking'
import type { CreateTransactionInput } from '../../types'
import { useCommitStaged, useUpdateStaged } from '../../hooks/useBanking'
import TransactionForm from '../forms/TransactionForm'

/**
 * Reviewing one bank row before it becomes a transaction — in the same form
 * used to add or edit one by hand.
 *
 * It used to be its own inline editor inside the card: a different shape,
 * a different set of fields, and a second place every form improvement had to
 * be repeated. A bank row is a transaction the user did not type, so the only
 * honest difference is where the values came from — which the bank panel
 * above the form shows, and the form itself does not need to know about.
 */
export default function StagedReviewModal({
  row,
  onClose,
}: {
  row: StagedTx
  onClose: () => void
}) {
  const update = useUpdateStaged()
  const commit = useCommitStaged()
  const [error, setError] = useState<string | null>(null)
  const [showRaw, setShowRaw] = useState(false)

  // The bank narrative behind this row: what the classifier read, and the only
  // place a merchant it failed to recognise is still named.
  const bankContext = [row.raw_payee, row.raw_details].filter(Boolean).join(' · ')

  // Saving and adding are one action here. The queue is the draft — there is
  // no second place to keep an edit, and "save without adding" is what
  // Dismiss on the card already means.
  const submit = async (data: CreateTransactionInput) => {
    setError(null)
    try {
      await update.mutateAsync({
        id: row.id,
        edit: {
          date: data.date,
          type: data.type,
          amount: Number(data.amount),
          category: String(data.category ?? ''),
          comment: data.comment ?? '',
          labels: data.labels ?? '',
          debit_account: data.debit_account ?? '',
          credit_account: data.credit_account ?? '',
        },
      })
      const res = await commit.mutateAsync([row.id])
      if (res.imported === 0) {
        // The commit re-verifies against the ledger, and it can decline —
        // the row may have been added from another device while this was open.
        setError(res.notes?.join(' · ') || 'That row was not added — reopen the list to see why.')
        return
      }
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not add that transaction')
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-start sm:items-center justify-center z-50 overflow-y-auto py-8">
      <div className="bg-white rounded-xl shadow-xl p-5 sm:p-6 w-full max-w-lg mx-4">
        <div className="flex items-start justify-between gap-2 mb-4">
          <h3 className="text-lg font-semibold text-gray-900">Add from your bank</h3>
          <button
            onClick={onClose}
            aria-label="Close"
            className="text-gray-400 hover:text-gray-700 text-xl leading-none"
          >
            ×
          </button>
        </div>

        {/* Where the proposal came from. Everything below this line is the
            ordinary transaction form; everything in it is bank provenance. */}
        <div className="mb-4 space-y-1.5">
          {row.verdict === 'duplicate_content' && (
            <p className="text-xs text-amber-800 bg-amber-50 border border-amber-200 rounded-lg px-2.5 py-2">
              ⚠️ {row.verdict_note || 'Matches a transaction already in your ledger.'}
            </p>
          )}
          {row.verdict === 'needs_review' && row.verdict_note && (
            <p className="text-xs text-purple-800 bg-purple-50 border border-purple-200 rounded-lg px-2.5 py-2">
              {row.verdict_note}
            </p>
          )}
          {row.enrich_note && (
            <p className="text-xs text-gray-500 bg-gray-50 border border-gray-100 rounded-lg px-2.5 py-1.5">
              ✨ {row.enrich_note}
            </p>
          )}
          <button
            type="button"
            onClick={() => setShowRaw((v) => !v)}
            className="text-xs text-gray-400 hover:text-gray-600"
          >
            {showRaw ? 'Hide' : 'Show'} raw bank data
          </button>
          {showRaw && (
            <pre className="text-xs bg-gray-50 rounded-lg p-2 overflow-x-auto text-gray-600 whitespace-pre-wrap break-words">
{`booked:   ${row.booking_date.slice(0, 10)}
payee:    ${row.raw_payee || '(none)'}
details:  ${row.raw_details || '(none)'}
amount:   ${row.raw_amount} ${row.raw_currency} ${row.raw_dk}
ref:      ${row.external_id}`}
            </pre>
          )}
        </div>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{error}</p>
        )}

        <TransactionForm
          onSubmit={submit}
          onCancel={onClose}
          isSubmitting={update.isPending || commit.isPending}
          submitLabel="Add transaction"
          aiContext={bankContext}
          // A photo would overwrite the date and amount the bank stated,
          // which are the two fields this row exists to keep faithful. AI
          // assist already reads the bank narrative for the description.
          showScan={false}
          // The queue stores labels verbatim, so the ⚡ preview chips have to
          // be saved as well — nothing downstream re-applies the rules.
          includeAutoLabels
          defaultValues={{
            date: row.date.slice(0, 10),
            type: row.type,
            amount: row.amount,
            category: row.category as CreateTransactionInput['category'],
            comment: row.comment,
            labels: row.labels,
            debit_account: row.debit_account,
            credit_account: row.credit_account,
          }}
        />
      </div>
    </div>
  )
}
