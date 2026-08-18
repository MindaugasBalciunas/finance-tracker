import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

// Markdown renders AI output (GitHub-flavored: tables, lists, bold, code)
// with compact, chat-friendly styling. Wide tables scroll inside their own
// container instead of stretching the bubble.
export default function Markdown({ children, invert = false }: { children: string; invert?: boolean }) {
  const muted = invert ? 'text-indigo-100' : 'text-gray-500'
  const border = invert ? 'border-indigo-300/40' : 'border-gray-200'
  return (
    <div className="text-sm leading-relaxed space-y-2 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          p: ({ children }) => <p className="whitespace-pre-wrap">{children}</p>,
          ul: ({ children }) => <ul className="list-disc pl-5 space-y-1">{children}</ul>,
          ol: ({ children }) => <ol className="list-decimal pl-5 space-y-1">{children}</ol>,
          li: ({ children }) => <li>{children}</li>,
          h1: ({ children }) => <p className="font-semibold text-base mt-2">{children}</p>,
          h2: ({ children }) => <p className="font-semibold text-base mt-2">{children}</p>,
          h3: ({ children }) => <p className="font-semibold mt-2">{children}</p>,
          h4: ({ children }) => <p className="font-semibold mt-1">{children}</p>,
          strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer" className="underline">
              {children}
            </a>
          ),
          code: ({ children, className }) =>
            className ? (
              <code className={`block overflow-x-auto rounded-lg p-2 text-xs font-mono ${invert ? 'bg-indigo-700/60' : 'bg-gray-800 text-gray-100'}`}>
                {children}
              </code>
            ) : (
              <code className={`px-1 py-0.5 rounded text-[0.85em] font-mono ${invert ? 'bg-indigo-700/60' : 'bg-gray-200/80'}`}>
                {children}
              </code>
            ),
          pre: ({ children }) => <pre className="my-1">{children}</pre>,
          table: ({ children }) => (
            <div className="overflow-x-auto -mx-1 px-1">
              <table className={`text-xs border-collapse [&_td]:px-2 [&_td]:py-1 [&_th]:px-2 [&_th]:py-1 [&_td]:border [&_th]:border ${invert ? '[&_td]:border-indigo-300/40 [&_th]:border-indigo-300/40' : '[&_td]:border-gray-200 [&_th]:border-gray-200'}`}>
                {children}
              </table>
            </div>
          ),
          th: ({ children }) => <th className={`text-left font-semibold ${muted}`}>{children}</th>,
          blockquote: ({ children }) => (
            <blockquote className={`border-l-2 pl-3 ${border} ${muted}`}>{children}</blockquote>
          ),
          hr: () => <hr className={`my-2 ${border}`} />,
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
