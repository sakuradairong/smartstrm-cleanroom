export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, init)
  } catch {
    throw new ApiError('网络错误，请检查连接后重试', 0)
  }
  if (response.status === 204) return undefined as T
  const text = await response.text()
  let body: unknown = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = null
    }
  }
  if (!response.ok) {
    const message = (body as { error?: string } | null)?.error || `请求失败（HTTP ${response.status}）`
    throw new ApiError(message, response.status)
  }
  return body as T
}

const jsonInit = (method: string, payload?: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(payload ?? {}),
})

export const getJSON = <T>(path: string) => api<T>(path)
export const postJSON = <T>(path: string, payload?: unknown) => api<T>(path, jsonInit('POST', payload))
export const putJSON = <T>(path: string, payload: unknown) => api<T>(path, jsonInit('PUT', payload))
export const deleteJSON = <T>(path: string) => api<T>(path, { method: 'DELETE' })
