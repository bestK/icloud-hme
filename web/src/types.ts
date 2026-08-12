export type Role = 'admin' | 'user'

export interface Account {
  id: string
  name: string
  real_email: string
  icloud_email: string
  host: string
  status: string
  alias_total: number
  alias_active: number
  /** 上次向上游核对计数的时间。为空表示从未核对过,此时上面两个 0 是"未知"而不是"没有" */
  alias_counted_at?: string
  last_validated: string
  created_at: string
}

export interface Alias {
  email: string
  anonymousId: string
  label: string
  active: boolean
  createdAt?: string
}

export interface TokenView {
  id: string
  name: string
  role: Role
  alias_count: number
  created_at: string
  last_used_at?: string
}

export interface CreateResult {
  account_id: string
  anonymous_id: string
  email: string
  label: string
  created_at: string
  source: 'pool' | 'live'
}

export interface PoolView {
  account_id: string
  depth: number
  target: number
  hour_used: number
  hourly_max: number
}

/** 密码登录完成 */
export interface LoginDone {
  id: string
  status: 'ok'
  cookies_count: number
  /** 新 Cookie 是否通过了一次 /validate */
  validated: boolean
  /** Cookie 拿到了但校验没过时的原因 */
  warning?: string
}

/** 密码通过了,但账号开了双重认证,验证码已发到受信任设备 */
export interface LoginNeeds2FA {
  id: string
  status: 'needs_2fa'
  login_id: string
  apple_id: string
  /** login_id 的有效秒数 */
  expires_in: number
}

export type LoginResponse = LoginDone | LoginNeeds2FA

export interface MailMessage {
  id: string
  from: string
  to: string
  subject: string
  date: string
  preview: string
}
