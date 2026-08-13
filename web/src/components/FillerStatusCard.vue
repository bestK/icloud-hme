<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { FillerStatus } from '@/types'

const props = defineProps<{ status: FillerStatus | null }>()
// 到点时通知父组件重拉一次,不然"下次执行"会一直停在一个过去的时间上
const emit = defineEmits<{ (e: 'due'): void }>()

const now = ref(Date.now())
const timer = window.setInterval(() => { now.value = Date.now() }, 1000)
onUnmounted(() => window.clearInterval(timer))

const leftMs = computed(() => {
  const next = props.status?.next_run_at
  if (!next) return null
  return new Date(next).getTime() - now.value
})

// 每个 next_run_at 只通知一次,否则父组件加载失败时会陷进重试循环
let notified = ''
watch(leftMs, (v) => {
  const next = props.status?.next_run_at
  if (!next || v === null || v > 0 || notified === next) return
  notified = next
  // 留几秒给这一轮补池跑完,否则拉回来的还是旧数字
  window.setTimeout(() => emit('due'), 3000)
})

function fmtInterval(sec: number): string {
  if (sec <= 0) return '—'
  if (sec % 3600 === 0) return `${sec / 3600} 小时`
  if (sec % 60 === 0) return `${sec / 60} 分钟`
  return `${sec} 秒`
}

function fmtClock(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  const today = new Date()
  const sameDay = d.toDateString() === today.toDateString()
  return sameDay
    ? d.toLocaleTimeString('zh-CN', { hour12: false })
    : d.toLocaleString('zh-CN', { hour12: false })
}

const leftText = computed(() => {
  if (leftMs.value === null) return '—'
  if (leftMs.value <= 0) return '即将执行'
  const s = Math.round(leftMs.value / 1000)
  const m = Math.floor(s / 60)
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
})
</script>

<template>
  <div v-if="status" class="filler">
    <div class="head">
      <span class="eyebrow">定时补池 · AUTO REFILL</span>
      <span v-if="!status.enabled" class="state off">未启用</span>
      <span v-else-if="status.running" class="state on">运行中</span>
      <span v-else class="state warn">已配置未启动</span>
    </div>

    <template v-if="status.enabled">
      <div class="rule-line">
        每 {{ fmtInterval(status.interval_seconds) }}一轮 · 每账号每小时最多建
        {{ status.hourly_max }} 个 · 同一轮内每 {{ fmtInterval(status.spacing_seconds) }}建一个
      </div>
      <div class="rule-line tight">
        空闲时不会停在保障水位上,只要配额有余量就一直囤,直到账号触及
        {{ status.hard_cap }} 个别名的上限 —— 等需求上来了再建就来不及了。
        池深度低于 {{ status.target }} 个算见底。
        池空时的实时创建也占用上面这份每小时配额,补池会自动让路。
      </div>

      <div class="grid">
        <div class="cell">
          <div class="k">上次执行</div>
          <div class="v mono">{{ fmtClock(status.last_run_at) }}</div>
          <div class="note">补了 {{ status.last_added }} 个</div>
        </div>
        <div class="cell">
          <div class="k">下次执行</div>
          <div class="v mono">{{ fmtClock(status.next_run_at) }}</div>
          <div class="note">还有 {{ leftText }}</div>
        </div>
        <div class="cell">
          <div class="k">累计补入</div>
          <div class="v mono">{{ status.total_added }}</div>
          <div class="note">本次启动以来</div>
        </div>
      </div>

      <div v-if="status.last_error" class="err">
        <span class="err-mark">最近失败</span>
        <span class="err-msg">{{ status.last_error }}</span>
        <span class="err-at mono">{{ fmtClock(status.last_error_at) }}</span>
      </div>
    </template>

    <div v-else class="rule-line">
      后台不会自动预建别名,每次创建都要现场向 iCloud 申请,会慢几秒。
      要打开就把 POOL_TARGET、POOL_INTERVAL、POOL_HOURLY_MAX 都设成大于 0 再重启。
    </div>
  </div>
</template>

<style lang="scss" scoped>
.filler {
  background: var(--paper);
  border: 1px solid var(--rule);
  padding: 18px 22px 20px;
}
.head {
  display: flex; align-items: center; justify-content: space-between;
  border-bottom: 1px dashed var(--rule);
  padding-bottom: 10px;
}
.state {
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.14em;
  padding: 2px 8px; border: 1px solid var(--rule);
  &.on { border-color: var(--ok); color: var(--ok); }
  &.off { border-color: var(--dim); color: var(--dim); }
  &.warn { border-color: var(--accent); color: var(--accent); }
}
.rule-line {
  color: var(--dim); font-size: 12px; line-height: 1.7;
  padding: 10px 0 2px;
  &.tight { padding-top: 0; }
}
.grid {
  display: grid; grid-template-columns: repeat(3, 1fr);
  gap: 20px; margin-top: 10px;
}
@media (max-width: 720px) { .grid { grid-template-columns: 1fr; gap: 12px; } }
.cell { border-left: 1px dashed var(--rule); padding-left: 14px; }
.cell:first-child { border-left: none; padding-left: 0; }
.k { color: var(--dim); font-size: 11px; letter-spacing: 0.06em; }
.v {
  font-family: var(--f-display); font-weight: 700; font-size: 22px;
  line-height: 1.2; margin-top: 2px;
  font-variant-numeric: tabular-nums lining-nums;
}
.note { color: var(--dim); font-size: 11px; margin-top: 2px; }
.err {
  margin-top: 14px; padding-top: 10px;
  border-top: 1px dashed var(--rule);
  display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap;
}
.err-mark {
  border: 1px solid var(--accent); color: var(--accent);
  font-size: 10px; letter-spacing: 0.14em; padding: 2px 6px; flex: none;
}
.err-msg { font-size: 12px; color: var(--ink); word-break: break-all; }
.err-at { font-size: 11px; color: var(--dim); margin-left: auto; }
</style>
