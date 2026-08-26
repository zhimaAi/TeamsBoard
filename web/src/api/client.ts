/** * API request encapsulation * baseURL: /api/local */

import { getApiToken, invalidateApiToken } from './token'

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
  code?: number | string

  constructor(message: string, status: number, code?: number | string) {
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
  console.log('[api] ' + method + ' ' + url)

  const finalHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
    ...headers,
  }
  // /auth/* 是后端放行的握手/登录入口，不携带 token，避免引导请求依赖 token 就绪
  const needsToken = !path.startsWith('/auth/')
  if (needsToken) {
    const token = await getApiToken()
    if (token) finalHeaders['X-GoTeams-Api-Token'] = token
  }

  const response = await fetchWithTokenRetry(url, {
    method,
    headers: finalHeaders,
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
    // 会话接口必须拿到实时状态；Electron 磁盘缓存曾缓存过本地后端的过期响应
    //（如 index.html），命中后请求根本不发出，导致登录态判断永远错误。
    cache: 'no-store',
  })

  if (!response.ok) {
    let errorMessage = `HTTP ${response.status}: ${response.statusText}`
    let errorCode: number | string | undefined
    try {
      const errorBody = await response.json()
      errorMessage = errorBody.error || errorBody.message || errorMessage
      errorCode = errorBody.code
    } catch {
      // ignore parse error
    }
    throw new ApiError(errorMessage, response.status, errorCode)
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

// 后端重启后 token 会失效（dev 模式常见）：收到 403 时清除缓存、重取 token 并重试一次。
async function fetchWithTokenRetry(
  url: string,
  init: RequestInit,
): Promise<Response> {
  let response = await fetch(url, init)
  const sentToken = (init.headers as Record<string, string>)['X-GoTeams-Api-Token']
  if (response.status !== 403 || !sentToken) return response

  invalidateApiToken()
  const newToken = await getApiToken()
  if (!newToken || newToken === sentToken) return response

  const retryHeaders = { ...(init.headers as Record<string, string>) }
  retryHeaders['X-GoTeams-Api-Token'] = newToken
  response = await fetch(url, { ...init, headers: retryHeaders })
  return response
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
