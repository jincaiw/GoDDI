import axios from 'axios'
import type { AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface PaginatedApiResponse<T = unknown> {
  code: number
  message: string
  data: T[]
  meta: {
    total: number
    page: number
    page_size: number
    pages: number
  }
}

const client: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// Flag to mark refresh requests so interceptor skips them
const SKIP_REFRESH_HEADER = 'X-Skip-Refresh-Interceptor'

interface PendingRequest {
  resolve: (value?: unknown) => void
  reject: (reason?: unknown) => void
}

let isRefreshing = false
let pendingQueue: PendingRequest[] = []

function subscribeTokenRefresh(cb: PendingRequest) {
  pendingQueue.push(cb)
}

function onTokenRefreshed() {
  pendingQueue.forEach((cb) => cb.resolve())
  pendingQueue = []
}

function onTokenRefreshFailed(err: unknown) {
  pendingQueue.forEach((cb) => cb.reject(err))
  pendingQueue = []
}

async function doRefresh(): Promise<void> {
  const authStore = useAuthStore()
  const currentToken = authStore.token
  if (!currentToken) {
    throw new Error('No token available for refresh')
  }
  // The refresh endpoint authenticates via Bearer token (not body).
  const response = await client.post<ApiResponse<{ token: string; csrf_token: string; expires_in: number }>>(
    '/auth/refresh',
    {},
    { headers: { [SKIP_REFRESH_HEADER]: '1' } }
  )
  const result = response.data.data
  authStore.setToken(result.token, '', result.csrf_token)
}

// Request interceptor: add Authorization header and CSRF token
client.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const authStore = useAuthStore()
    const token = authStore.token
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }
    const csrfToken = authStore.csrfToken
    if (csrfToken && config.headers) {
      config.headers['X-CSRF-Token'] = csrfToken
    }
    return config
  },
  (error) => Promise.reject(error)
)

// Response interceptor: handle errors, refresh on 401, then extract data
client.interceptors.response.use(
  (response: AxiosResponse) => {
    const data = response.data as ApiResponse
    if (data.code !== undefined && data.code !== 0) {
      const error = new Error(data.message || 'Request failed') as Error & { code?: number }
      error.code = data.code
      return Promise.reject(error)
    }
    return response
  },
  async (error) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean; _skipRefresh?: boolean }
    const isRefreshRequest = originalRequest?.headers?.[SKIP_REFRESH_HEADER] !== undefined

    if (error.response) {
      const status = error.response.status
      const isAuthEndpoint = ['/auth/login', '/auth/refresh'].some(url => originalRequest?.url?.includes(url))
      if (status === 401 && !originalRequest._retry && !isRefreshRequest && !isAuthEndpoint) {
        originalRequest._retry = true

        if (isRefreshing) {
          // Wait for the in-progress refresh to complete, then retry
          return new Promise((resolve, reject) => {
            subscribeTokenRefresh({ resolve, reject })
          }).then(() => {
            const authStore = useAuthStore()
            const token = authStore.token
            if (token && originalRequest.headers) {
              originalRequest.headers.Authorization = `Bearer ${token}`
            }
            return client.request(originalRequest)
          })
        }

        isRefreshing = true
        try {
          await doRefresh()
          isRefreshing = false
          onTokenRefreshed()
          const authStore = useAuthStore()
          const token = authStore.token
          if (token && originalRequest.headers) {
            originalRequest.headers.Authorization = `Bearer ${token}`
          }
          return client.request(originalRequest)
        } catch (refreshErr) {
          isRefreshing = false
          onTokenRefreshFailed(refreshErr)
          // Refresh failed: invalidate local auth state immediately so the UI
          // is consistent with the server. We do NOT call authStore.logout()
          // here because that would issue another /auth/logout request against
          // an already-expired session, racing with the refresh failure and
          // potentially masking the real cause. Clearing tokens is enough.
          const authStore = useAuthStore()
          authStore.clearToken()
          router.push('/login')
          return Promise.reject(refreshErr)
        }
      }

      const data = error.response.data as ApiResponse
      const message = data?.message || error.message || 'Request failed'
      return Promise.reject(new Error(message))
    }
    return Promise.reject(error)
  }
)

export default client

// Helper functions for common API patterns
export async function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  const response = await client.get<ApiResponse<T>>(url, { params })
  return response.data.data
}

export async function getList<T>(
  url: string,
  params?: Record<string, unknown>
): Promise<{ data: T[]; meta: PaginatedApiResponse<T>['meta'] }> {
  const response = await client.get<unknown>(url, { params })
  const body = response.data as Record<string, unknown>
  // Handle paginated response format: { code, data: [], meta: {...} }
  if (body.data !== undefined && body.meta !== undefined) {
    return { data: body.data as T[], meta: body.meta as PaginatedApiResponse<T>['meta'] }
  }
  // Handle direct array format: { code, data: [] }
  if (Array.isArray(body.data)) {
    return { data: body.data as T[], meta: { total: body.data.length, page: 1, page_size: body.data.length, pages: 1 } }
  }
  // Fallback: return empty
  return { data: [], meta: { total: 0, page: 1, page_size: 20, pages: 0 } }
}

export async function post<T>(url: string, data?: unknown): Promise<T> {
  const response = await client.post<ApiResponse<T>>(url, data)
  return response.data.data
}

export async function put<T>(url: string, data?: unknown): Promise<T> {
  const response = await client.put<ApiResponse<T>>(url, data)
  return response.data.data
}

export async function del<T = void>(url: string): Promise<T> {
  const response = await client.delete<ApiResponse<T>>(url)
  return response.data.data
}
