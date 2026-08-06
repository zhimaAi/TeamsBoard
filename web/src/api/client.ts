/** * API request encapsulation * baseURL: /api/local */

const BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/local'

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  headers?: Record<string, string>
  params?: Record<string, string | number | boolean | undefined>
}

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export class ApiError extends Error {
  status: number
  code?: number

  constructor(message: string, status: number, code?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

function buildUrl(path: string, params?: RequestOptions['params']): string {
  const url = `${BASE_URL}${path.startsWith('/') ? path : `/${path}`}`
  if (!params) return url
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null) {
      search.append(key, String(value))
    }
  }
  const qs = search.toString()
  return qs ? `${url}?${qs}` : url
}

export async function request<T = unknown>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = 'GET', body, headers = {}, params } = options

  const url = buildUrl(path, params)

  const finalHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
    ...headers,
  }
  const response = await fetch(url, {
    method,
    headers: finalHeaders,
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })

  if (!response.ok) {
    let errorMessage = `HTTP ${response.status}: ${response.statusText}`
    try {
      const errorBody = await response.json()
      errorMessage = errorBody.error || errorBody.message || errorMessage
    } catch {
      // ignore parse error
    }
    throw new ApiError(errorMessage, response.status)
  }

  const contentType = response.headers.get('content-type') || ''
  if (!contentType.includes('application/json')) {
    return (await response.text()) as unknown as T
  }

  const result: ApiResponse<T> = await response.json()

  // Only unified responses with code are considered envelopes; business objects of native APIs may also legally contain data fields.
  if (result.code !== undefined) {
    if (result.code !== 0 && result.code !== 200) {
      throw new ApiError(result.message || 'Request failed', response.status, result.code)
    }
    return result.data !== undefined ? result.data : (result as unknown as T)
  }

  return result as unknown as T
}

export const apiClient = {
  get: <T = unknown>(path: string, params?: RequestOptions['params']) =>
    request<T>(path, { method: 'GET', params }),

  post: <T = unknown>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body }),

  put: <T = unknown>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body }),

  delete: <T = unknown>(path: string) =>
    request<T>(path, { method: 'DELETE' }),
}

export default apiClient
