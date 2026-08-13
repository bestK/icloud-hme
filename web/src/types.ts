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
  account_name?: string
  /** 非 active 的账号会被定时补池跳过 */
  account_status?: string
  depth: number
  /** 最低保障水位。深度会一路涨过它,不是进度条的分母 */
  target: number
  hour_used: number
  hourly_max: number
  /** 账号已用掉的别名总数。alias_counted 为 false 时这个 0 是"没核对过" */
  alias_total: number
  alias_counted: boolean
  /** Apple 给单账号的别名上限,补池真正的天花板 */
  alias_cap: number
}

/** 定时补池调度器的运行快照 */
export interface FillerStatus {
  enabled: boolean
  running: boolean
  /** 最低保障水位,不是补池的终点 */
  target: number
  hourly_max: number
  interval_seconds: number
  /** 同一轮内两次创建之间的间隔 */
  spacing_seconds: number
  /** 每个账号能囤到的天花板 */
  hard_cap: number
  /** 为空表示还没跑过第一轮 */
  last_run_at?: string
  next_run_at?: string
  /** 上一轮补进池子的个数 */
  last_added: number
  /** 本次进程启动以来累计补的个数 */
  total_added: number
  last_error?: string
  last_error_at?: string
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
  /** 纯文本正文。没有 text/plain 分段时是从 HTML 剥出来的 */
  preview: string
  /** text/html 原文。纯文本邮件、正文过大、或走 web_api 那条路时没有 */
  html?: string
  /** 邮件所在邮箱:INBOX / Junk */
  folder?: string
}
