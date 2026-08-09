<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import type { Account, PoolView, TokenView } from '@/types'
import PoolDepthCard from '@/components/PoolDepthCard.vue'

const accounts = ref<Account[]>([])
const pools = ref<PoolView[]>([])
const tokens = ref<TokenView[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const [a, p, t] = await Promise.all([api.listAccounts(), api.listPool(), api.listTokens()])
    accounts.value = a
    pools.value = p
    tokens.value = t
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <section class="page">
    <div class="masthead">
      <div class="eyebrow">档案总览 · OVERVIEW</div>
      <h1>本日邮政日报</h1>
      <div class="sub">
        {{ accounts.length }} 个 iCloud 账号 · {{ tokens.length }} 个已签发 token ·
        {{ pools.reduce((s, p) => s + p.depth, 0) }} 张邮票在库
      </div>
    </div>

    <div class="stack">
      <PoolDepthCard v-for="p in pools" :key="p.account_id" :view="p" />
    </div>

    <hr class="rule-hair" />

    <div class="grid-2">
      <div class="panel">
        <div class="panel-title">
          <span class="eyebrow">iCloud 账号 · ACCOUNTS</span>
        </div>
        <ul class="list">
          <li v-for="a in accounts" :key="a.id" class="row">
            <div class="row-main">
              <div class="row-name">{{ a.name }}</div>
              <div class="row-mail mono">{{ a.icloud_email || a.real_email }}</div>
            </div>
            <div class="row-side">
              <span class="chip" :class="a.status">{{ a.status }}</span>
              <span class="mono meta">{{ a.id }}</span>
            </div>
          </li>
        </ul>
      </div>

      <div class="panel">
        <div class="panel-title">
          <span class="eyebrow">token 使用 · USAGE</span>
        </div>
        <ul class="list">
          <li v-for="t in tokens" :key="t.id" class="row">
            <div class="row-main">
              <div class="row-name">{{ t.name }} <span class="mono meta">· {{ t.id }}</span></div>
              <div class="row-mail dim">{{ t.role === 'admin' ? '管理员' : '用户' }} · last {{ t.last_used_at ? new Date(t.last_used_at).toLocaleString() : '—' }}</div>
            </div>
            <div class="row-side big">
              <span class="digit">{{ t.alias_count }}</span>
              <span class="unit">邮票</span>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<style lang="scss" scoped>
.page { max-width: 1080px; }

.masthead { margin-bottom: 24px; }
.masthead h1 {
  font-family: var(--f-display); font-weight: 700;
  font-size: 44px; letter-spacing: -0.02em; margin: 6px 0 6px;
}
.masthead .sub { color: var(--dim); font-size: 13px; }

.stack { display: grid; gap: 20px; }

.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
@media (max-width: 900px) { .grid-2 { grid-template-columns: 1fr; } }

.panel {
  background: var(--paper);
  border: 1px solid var(--rule);
  padding: 20px 22px;
}
.panel-title { border-bottom: 1px dashed var(--rule); padding-bottom: 10px; margin-bottom: 4px; }

.list { list-style: none; margin: 0; padding: 0; }
.row {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 0;
  border-bottom: 1px dashed var(--rule);
  &:last-child { border-bottom: none; }
}
.row-name { font-family: var(--f-display); font-weight: 500; font-size: 16px; }
.row-mail { color: var(--dim); font-size: 12px; margin-top: 3px; }
.row-mail.dim { font-family: var(--f-body); }
.meta { color: var(--dim); font-size: 11px; }
.chip {
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.14em;
  padding: 2px 8px; border: 1px solid var(--rule); color: var(--ink);
  text-transform: uppercase;
  &.active { border-color: var(--ok); color: var(--ok); }
  &.error { border-color: var(--stamp); color: var(--stamp); }
  &.pending { border-color: var(--dim); color: var(--dim); }
}
.row-side { display: flex; align-items: center; gap: 12px; }
.row-side.big { flex-direction: column; align-items: flex-end; gap: 0; }
.digit {
  font-family: var(--f-display); font-weight: 700; font-size: 28px;
  line-height: 1; color: var(--ink);
  font-feature-settings: "tnum" 1, "lnum" 1;
}
.unit { font-size: 10px; letter-spacing: 0.2em; color: var(--dim); text-transform: uppercase; }
</style>
