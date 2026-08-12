import axios, { AxiosError, type AxiosInstance } from 'axios'
import type {
  Account, Alias, CreateResult, FillerStatus, LoginDone, LoginResponse, MailMessage, PoolView,
  Role, TokenView,
} from '@/types'

const TOKEN_KEY = 'hme_token'
const ROLE_KEY = 'hme_role'

export function getToken(): string {
  return sessionStorage.getItem(TOKEN_KEY) ?? ''
}
export function setToken(v: string) {
  if (v) sessionStorage.setItem(TOKEN_KEY, v)
  else sessionStorage.removeItem(TOKEN_KEY)
}
export function getRole(): Role | '' {
  return (sessionStorage.getItem(ROLE_KEY) as Role) || ''
}
export function setRole(v: Role | '') {
  if (v) sessionStorage.setItem(ROLE_KEY, v)
  else sessionStorage.removeItem(ROLE_KEY)
}

const http: AxiosInstance = axios.create({
  baseURL: '/api',
  timeout: 30_000,
})

http.interceptors.request.use((cfg) => {
  const t = getToken()
  if (t) cfg.headers['Authorization'] = `Bearer ${t}`
  return cfg
})

/** 带上后端状态码和 iCloud 原始状态码的错误,message 是后端给的可读原因 */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status?: number,
    readonly upstreamStatus?: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

interface ErrBody {
  success: false
  message?: string
  /** "upstream" = iCloud 会话的问题,不是面板 token 的问题 */
  scope?: string
  upstream_status?: number
}

http.interceptors.response.use(
  (resp) => resp,
  (err: AxiosError<ErrBody>) => {
    const body = err.response?.data
    // 401 有两种:面板 token 失效(该踢回登录页)和 iCloud 会话失效(只该报错)。
    // 不分开的话,一个账号 Cookie 过期就会把管理员从面板上挤下去。
    if (err.response?.status === 401 && body?.scope !== 'upstream') {
      setToken('')
      setRole('')
      if (!location.pathname.startsWith('/login')) {
        location.assign('/login')
      }
    }
    // axios 默认的 message 是 "Request failed with status code 502",
    // 什么信息都没有;后端把可读原因放在 body.message 里。
    if (body?.message) {
      return Promise.reject(new ApiError(body.message, err.response?.status, body.upstream_status))
    }
    return Promise.reject(err)
  },
)

function unwrap<T>(data: any): T {
  if (data && data.success === false) throw new Error(data.message || '请求失败')
  return data.data as T
}

// 试探当前 token 的 role:访问 /api/accounts,200=admin,403=user,401=无效
export async function probeRole(): Promise<Role> {
  try {
    const r = await http.get('/accounts')
    if (r.status === 200) return 'admin'
  } catch (e) {
    const ax = e as AxiosError
    if (ax.response?.status === 403) return 'user'
    if (ax.response?.status === 401) throw new Error('token 无效')
    throw e
  }
  return 'user'
}

export const api = {
  listAccounts: async () => unwrap<Account[]>((await http.get('/accounts')).data),

  addAccount: async (payload: { name: string; cookies?: string; host?: string; proxy?: string }) =>
    unwrap<Account>((await http.post('/accounts', payload)).data),

  removeAccount: async (id: string) =>
    unwrap<{ id: string }>((await http.delete(`/accounts/${id}`)).data),

  updateCookies: async (id: string, cookies: Record<string, string>) =>
    unwrap<{ id: string; cookies_count: number }>(
      (await http.put(`/accounts/${id}/cookies`, { cookies })).data,
    ),

  // 第一步:提交密码。开了 2FA 会返回 needs_2fa,此时验证码已经发到受信任设备,
  // 拿着 login_id 走 verifyLogin —— 不能重新提交密码,那会让 Apple 重发新码。
  loginAccount: async (id: string, password: string) =>
    unwrap<LoginResponse>((await http.post(`/accounts/${id}/login`, { password })).data),

  // 第二步:提交受信任设备上收到的验证码
  verifyLogin: async (id: string, loginId: string, code: string) =>
    unwrap<LoginDone>(
      (await http.post(`/accounts/${id}/login/verify`, { login_id: loginId, code })).data,
    ),

  revalidateAccount: async (id: string) =>
    unwrap<{
      id: string
      status: string
      alias_total: number
      alias_active: number
      alias_counted_at: string
      last_validated: string
    }>((await http.post(`/accounts/${id}/revalidate`)).data),

  setAppPassword: async (id: string, icloud_email: string, app_password: string) =>
    unwrap<{ id: string; icloud_email: string }>(
      (await http.post(`/accounts/${id}/password`, { icloud_email, app_password })).data,
    ),

  reload: async () => unwrap<{ message: string }>((await http.post('/reload')).data),


  listPool: async () => unwrap<PoolView[]>((await http.get('/pool')).data),

  fillerStatus: async () => unwrap<FillerStatus>((await http.get('/pool/filler')).data),

  listTokens: async () => unwrap<TokenView[]>((await http.get('/tokens')).data),

  createToken: async (name: string) =>
    unwrap<TokenView & { secret: string }>((await http.post('/tokens', { name })).data),

  deleteToken: async (id: string) =>
    unwrap<{ id: string }>((await http.delete(`/tokens/${id}`)).data),

  listAliases: async (accountId: string) =>
    unwrap<{ account_id: string; count: number; aliases: Alias[] | null }>(
      (await http.get('/aliases', { params: { account_id: accountId } })).data,
    ),

  createAlias: async (accountId: string, label: string) =>
    unwrap<CreateResult>((await http.post('/create', { account_id: accountId, label })).data),

  deactivateAlias: async (anonymousId: string, accountId: string) =>
    unwrap<{ anonymous_id: string; success: boolean }>(
      (await http.post(`/aliases/${anonymousId}/deactivate`, { account_id: accountId })).data,
    ),

  reactivateAlias: async (anonymousId: string, accountId: string) =>
    unwrap<{ anonymous_id: string; success: boolean }>(
      (await http.post(`/aliases/${anonymousId}/reactivate`, { account_id: accountId })).data,
    ),

  deleteAlias: async (anonymousId: string, accountId: string) =>
    unwrap<{ anonymous_id: string }>(
      (await http.delete(`/aliases/${anonymousId}`, { data: { account_id: accountId } })).data,
    ),

  inbox: async (accountId: string, alias: string, limit = 20) =>
    unwrap<{
      account_id: string
      alias?: string
      count: number
      messages: MailMessage[]
      method: 'imap' | 'web_api'
    }>((await http.get('/inbox', { params: { account_id: accountId, alias, limit } })).data),
}
