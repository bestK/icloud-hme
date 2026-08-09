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

export interface MailMessage {
  id: string
  from: string
  to: string
  subject: string
  date: string
  preview: string
}
