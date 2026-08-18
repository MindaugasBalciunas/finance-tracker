// Compact error banner for a page's primary query: a failed load must look
// different from "no data yet", and offer a retry on the spot.
export default function QueryError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const e = error as { response?: { data?: { error?: string } }; message?: string } | null
  const message = e?.response?.data?.error ?? e?.message ?? 'Something went wrong'
  return (
    <div className="bg-red-50 border border-red-200 rounded-xl p-4 flex flex-wrap items-center gap-3">
      <div className="flex-1 min-w-0 basis-52">
        <p className="text-sm font-semibold text-red-700">Couldn't load data</p>
        <p className="text-xs text-red-600/80 mt-0.5 break-words">{message}</p>
      </div>
      <button
        onClick={onRetry}
        className="px-3 py-1.5 text-sm font-medium rounded-lg bg-red-600 text-white hover:bg-red-700 shrink-0"
      >
        Retry
      </button>
    </div>
  )
}
