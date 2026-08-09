<script setup lang="ts">
import { ref, watch } from 'vue'
import type { PoolView } from '@/types'

const props = defineProps<{ view: PoolView }>()

const displayed = ref(0)
let raf = 0

function animate(target: number) {
  const start = displayed.value
  const t0 = performance.now()
  const dur = 400
  const ease = (t: number) => 1 - Math.pow(1 - t, 4)
  cancelAnimationFrame(raf)
  const step = (t: number) => {
    const p = Math.min(1, (t - t0) / dur)
    displayed.value = Math.round(start + (target - start) * ease(p))
    if (p < 1) raf = requestAnimationFrame(step)
  }
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    displayed.value = target
    return
  }
  raf = requestAnimationFrame(step)
}
watch(
  () => props.view.depth,
  (v) => animate(v),
  { immediate: true },
)
</script>

<template>
  <div class="card">
    <svg class="frame" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
      <rect x="0.5" y="0.5" width="99" height="99" fill="none" stroke="var(--rule)" stroke-width="0.5" stroke-dasharray="1.5 1.5"/>
    </svg>

    <div class="acct-tag">ACCT · {{ view.account_id.replace('acc_', '').toUpperCase() }}</div>
    <div class="label">地址池深度</div>
    <div class="number">
      <span class="digits">{{ String(displayed).padStart(2, '0') }}</span>
      <span class="of">/ {{ view.target }} 目标</span>
    </div>

    <div class="side">
      <div class="side-row">
        <span class="k">本小时已用</span>
        <span class="v mono">{{ view.hour_used }} / {{ view.hourly_max }}</span>
      </div>
      <div class="side-row">
        <span class="k">账号</span>
        <span class="v mono">{{ view.account_id }}</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.card {
  position: relative;
  background: var(--paper);
  border: 1px solid var(--ink);
  padding: 32px 32px 24px;
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: end;
  gap: 32px;
  overflow: hidden;
  isolation: isolate;

  /* 边缘齿孔:纯装饰 */
  --p: 6px;
  mask-image:
    radial-gradient(circle at center, transparent 3px, black 3.5px) 0 -6px / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) 0 100% / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) -6px 0 / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) 100% 0 / 12px 12px,
    linear-gradient(black, black);
  mask-composite: intersect;
}
.frame { position: absolute; inset: 0; width: 100%; height: 100%; z-index: -1; opacity: 0.4; }
.acct-tag {
  position: absolute; top: 12px; right: 16px;
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.16em; color: var(--dim);
}
/* 中文标题:0.24em 字距 + uppercase 是给英文的,这里收掉 */
.label {
  font-family: var(--f-body); font-size: 12px; letter-spacing: 0.08em; color: var(--dim);
}
.number { line-height: 1; margin-top: 8px; }
.digits {
  font-family: var(--f-display); font-weight: 700;
  font-size: 120px;
  letter-spacing: -0.04em;
  color: var(--ink);
  font-variant-numeric: tabular-nums lining-nums;
  border-bottom: 4px solid var(--accent);
  padding-bottom: 4px;
}
.of {
  display: inline-block;
  margin-left: 14px;
  font-family: var(--f-body); font-size: 13px; color: var(--dim);
  letter-spacing: 0.06em;
}
.side {
  border-left: 1px dashed var(--rule);
  padding-left: 24px;
  align-self: stretch;
  display: flex; flex-direction: column; justify-content: flex-end;
  gap: 8px;
  min-width: 180px;
}
.side-row {
  display: flex; justify-content: space-between; gap: 12px;
  /* 同上:键名改中文后去掉 uppercase 与宽字距 */
  .k { color: var(--dim); font-size: 12px; letter-spacing: 0.04em; }
  .v { font-size: 12px; color: var(--ink); }
}

@media (max-width: 720px) {
  .card { grid-template-columns: 1fr; gap: 16px; padding: 20px; }
  .digits { font-size: 72px; }
  .side { border-left: none; border-top: 1px dashed var(--rule); padding: 12px 0 0; }
}
</style>
