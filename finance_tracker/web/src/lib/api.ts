// Thin fetch wrapper. Every call is same-origin (nginx proxies /api).
export class ApiError extends Error {
  status: number
  data: any
  constructor(status: number, message: string, data?: any) {
    super(message)
    this.status = status
    this.data = data
  }
}

type Query = Record<string, string | number | boolean | undefined | null | string[]>

export function qs(q?: Query): string {
  if (!q) return ''
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === '' || v === false) continue
    if (Array.isArray(v)) v.forEach((x) => x && p.append(k, x))
    else p.set(k, String(v === true ? '1' : v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

async function request<T>(method: string, path: string, body?: unknown, q?: Query): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} }
  if (body instanceof FormData) init.body = body
  else if (body !== undefined) {
    init.body = typeof body === 'string' ? body : JSON.stringify(body)
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
  }
  const res = await fetch(`api${path}${qs(q)}`, init)
  const text = await res.text()
  let data: any = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  if (!res.ok) {
    if (res.status === 401 && data?.error === 'locked') window.dispatchEvent(new Event('ft:locked'))
    throw new ApiError(res.status, data?.error || res.statusText, data)
  }
  return data as T
}

export const api = {
  get: <T>(path: string, q?: Query) => request<T>('GET', path, undefined, q),
  post: <T>(path: string, body?: unknown, q?: Query) => request<T>('POST', path, body ?? {}, q),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T>(path: string, q?: Query) => request<T>('DELETE', path, undefined, q),
}
