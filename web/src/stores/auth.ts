import { defineStore } from 'pinia'
import { getRole, getToken, setRole, setToken } from '@/api'
import { probeRole } from '@/api'
import type { Role } from '@/types'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getToken(),
    role: getRole() as Role | '',
  }),
  getters: {
    isAdmin: (s) => s.role === 'admin',
    isLoggedIn: (s) => !!s.token && !!s.role,
  },
  actions: {
    async login(token: string) {
      setToken(token)
      this.token = token
      try {
        const r = await probeRole()
        setRole(r)
        this.role = r
      } catch (e) {
        setToken('')
        setRole('')
        this.token = ''
        this.role = ''
        throw e
      }
    },
    logout() {
      setToken('')
      setRole('')
      this.token = ''
      this.role = ''
    },
  },
})
