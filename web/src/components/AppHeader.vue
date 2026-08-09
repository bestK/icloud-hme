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
      <svg class="logo" viewBox="0 0 32 32" aria-hidden="true">
        <rect x="3" y="3" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.75" stroke-dasharray="2 2" vector-effect="non-scaling-stroke"/>
        <text x="16" y="21" text-anchor="middle" font-family="Fraunces, serif" font-size="14" font-weight="700" fill="var(--accent)">H</text>
      </svg>
      <div class="wordmark">
        <div class="line-1">iCLOUD</div>
        <div class="line-2">Hide My Email</div>
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
.logo { width: 36px; height: 36px; color: var(--primary); }
.wordmark .line-1 {
  font-family: var(--f-body);
  font-size: 11px;
  letter-spacing: 0.24em;
  color: var(--dim);
}
.wordmark .line-2 {
  font-family: var(--f-display);
  /* "Hide My Email" 比原来的 "Registry" 长,22px 会挤到 role 徽标 */
  font-size: 19px;
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
  &.admin .dot { background: var(--accent); }
  &.user .dot { background: var(--primary); }
}
.dot { width: 6px; height: 6px; border-radius: 50%; background: var(--dim); display: inline-block; }

.logout {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: var(--hit);
  background: transparent;
  border: 1px solid var(--ink);
  color: var(--ink);
  padding: 6px 16px;
  font-family: var(--f-body);
  font-size: 12px;
  letter-spacing: 0.08em;
  cursor: pointer;
  transition:
    background-color var(--dur-fast) var(--ease-out),
    color var(--dur-fast) var(--ease-out),
    scale var(--dur-fast) var(--ease-out);
  &:hover { background: var(--ink); color: var(--paper); }
  &:active { scale: 0.96; }
  &:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
}

@media (max-width: 720px) {
  .hdr { padding: 14px 16px; }
}
</style>
