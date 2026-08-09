<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import { computed } from 'vue'

const auth = useAuthStore()

interface NavItem { path: string; label: string; hint: string; adminOnly?: boolean }
const nav: NavItem[] = [
  { path: '/overview', label: '总览',   hint: 'OVERVIEW', adminOnly: true },
  { path: '/aliases',  label: '别名',   hint: 'ALIASES' },
  { path: '/inbox',    label: '收件箱', hint: 'INBOX' },
  { path: '/tokens',   label: '令牌',   hint: 'TOKENS', adminOnly: true },
]

const items = computed(() => nav.filter((n) => !n.adminOnly || auth.isAdmin))
</script>

<template>
  <nav class="side">
    <div class="section">
      <div class="section-label">导航 / NAVIGATION</div>
      <ul>
        <li v-for="n in items" :key="n.path">
          <router-link :to="n.path" class="item" active-class="active">
            <span class="hint">{{ n.hint }}</span>
            <span class="label">{{ n.label }}</span>
          </router-link>
        </li>
      </ul>
    </div>
    <div class="section footer">
      <div class="section-label">档案 / RECORD</div>
      <div class="stamp">
        <svg viewBox="0 0 100 60" width="100%">
          <rect x="4" y="4" width="92" height="52" fill="none" stroke="var(--rule)" stroke-dasharray="3 3"/>
          <text x="50" y="34" text-anchor="middle" font-family="Fraunces, serif" font-size="14" fill="var(--dim)">HME · REGISTRY</text>
        </svg>
      </div>
    </div>
  </nav>
</template>

<style lang="scss" scoped>
.side {
  border-right: 1px solid var(--rule);
  background: var(--paper);
  padding: 24px 16px;
  display: flex;
  flex-direction: column;
  gap: 32px;
}

.section-label {
  font-size: 10px;
  letter-spacing: 0.2em;
  color: var(--dim);
  text-transform: uppercase;
  padding: 0 8px 12px;
  border-bottom: 1px dashed var(--rule);
  margin-bottom: 8px;
}

ul { list-style: none; margin: 0; padding: 0; }

.item {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 10px 8px;
  color: var(--ink);
  border-left: 2px solid transparent;
  &:hover { background: var(--bg); text-decoration: none; }
  &.active {
    border-left-color: var(--stamp);
    background: var(--bg);
  }
  .hint {
    font-family: var(--f-mono);
    font-size: 10px;
    letter-spacing: 0.14em;
    color: var(--dim);
  }
  .label {
    font-family: var(--f-display);
    font-weight: 500;
    font-size: 15px;
  }
  &.active .label { color: var(--stamp); }
}

.footer { margin-top: auto; opacity: 0.75; }

@media (max-width: 720px) {
  .side {
    border-right: none;
    border-bottom: 1px solid var(--rule);
    padding: 12px;
    flex-direction: row;
    align-items: center;
    justify-content: flex-start;
    gap: 12px;
    overflow-x: auto;
  }
  .section-label, .footer { display: none; }
  ul { display: flex; gap: 8px; }
  .item { padding: 6px 10px; border-left: 0; border-bottom: 2px solid transparent; }
  .item.active { border-bottom-color: var(--stamp); }
  .item .hint { display: none; }
}
</style>
