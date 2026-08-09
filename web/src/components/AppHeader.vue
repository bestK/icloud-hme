<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import { useRouter } from 'vue-router'

const auth = useAuthStore()
const router = useRouter()

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <header class="hdr">
    <div class="brand">
      <svg class="stamp" viewBox="0 0 32 32" aria-hidden="true">
        <rect x="3" y="3" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.5" stroke-dasharray="2 2"/>
        <text x="16" y="21" text-anchor="middle" font-family="Fraunces, serif" font-size="14" font-weight="700" fill="var(--stamp)">H</text>
      </svg>
      <div class="wordmark">
        <div class="line-1">iCLOUD · HME</div>
        <div class="line-2">Registry</div>
      </div>
    </div>
    <div class="meta">
      <span class="role" :class="auth.role">
        <span class="dot" aria-hidden="true" />
        {{ auth.role || '—' }}
      </span>
      <button class="logout" @click="logout">退出</button>
    </div>
  </header>
</template>

<style lang="scss" scoped>
.hdr {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 40px;
  border-bottom: 1px solid var(--rule);
  background: var(--paper);
}
.brand { display: flex; align-items: center; gap: 14px; color: var(--ink); }
.stamp { width: 36px; height: 36px; color: var(--primary); }
.wordmark .line-1 {
  font-family: var(--f-body);
  font-size: 11px;
  letter-spacing: 0.24em;
  color: var(--dim);
}
.wordmark .line-2 {
  font-family: var(--f-display);
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.01em;
}

.meta { display: flex; align-items: center; gap: 16px; }

.role {
  display: inline-flex; align-items: center; gap: 6px;
  font-family: var(--f-mono);
  font-size: 12px;
  padding: 4px 10px;
  border: 1px solid var(--rule);
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--ink);
  background: var(--bg);
  &.admin .dot { background: var(--stamp); }
  &.user .dot { background: var(--primary); }
}
.dot { width: 6px; height: 6px; border-radius: 50%; background: var(--dim); display: inline-block; }

.logout {
  background: transparent;
  border: 1px solid var(--ink);
  color: var(--ink);
  padding: 6px 14px;
  font-family: var(--f-body);
  font-size: 12px;
  letter-spacing: 0.08em;
  cursor: pointer;
  &:hover { background: var(--ink); color: var(--paper); }
}

@media (max-width: 720px) {
  .hdr { padding: 14px 16px; }
}
</style>
