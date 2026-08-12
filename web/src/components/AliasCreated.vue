<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { CreateResult } from '@/types'

defineProps<{
  result: CreateResult | null
  open: boolean
  /** 账号名,拿不到时退回显示 account_id */
  accountName?: string
}>()
const emit = defineEmits<{ (e: 'close'): void }>()

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败,请手动选中')
  }
}
</script>

<template>
  <el-dialog
    :model-value="open"
    :show-close="false"
    :close-on-click-modal="false"
    width="520px"
    align-center
    @close="emit('close')"
    class="created"
  >
    <div v-if="result" class="body">
      <div class="card">
        <div class="source">
          <div>{{ result.source === 'pool' ? '来自地址池' : '实时申请' }}</div>
          <div class="date">{{ new Date(result.created_at).toISOString().slice(0, 10) }}</div>
        </div>
        <div class="alias-id">ID {{ result.anonymous_id.slice(0, 6).toUpperCase() }}</div>
        <div class="label">你的新地址</div>
        <button
          class="email copyable"
          type="button"
          :title="`复制 ${result.email}`"
          @click="copy(result.email)"
        >{{ result.email }}</button>
        <div class="owner">
          <span class="k">归属账号</span>
          <span class="v">{{ accountName || result.account_id }}</span>
          <span v-if="accountName" class="v mono id">{{ result.account_id }}</span>
        </div>
        <div class="foot">
          <span class="tag">{{ result.label || '无标签' }}</span>
          <button class="copy-btn" type="button" @click="copy(result.email)">复制地址</button>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="actions">
        <el-button plain @click="emit('close')">关闭</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style lang="scss" scoped>
.created :deep(.el-dialog) { background: var(--bg); }

.body { padding: 8px 4px 0; }
.card {
  background: var(--paper);
  border: 1px solid var(--ink);
  padding: 24px 22px 20px;
  position: relative;
  /* 边缘齿孔:纯装饰 */
  mask-image:
    radial-gradient(circle at center, transparent 3px, black 3.5px) 0 -6px / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) 0 100% / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) -6px 0 / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) 100% 0 / 12px 12px,
    linear-gradient(black, black);
  mask-composite: intersect;
}
/* 来源标记:池命中 or 实时申请 */
.source {
  position: absolute; top: 14px; right: -12px;
  border: 2px solid var(--accent);
  color: var(--accent);
  padding: 6px 10px;
  /* 用独立 rotate 而不是 transform,避免和 badge-in 关键帧里的 rotate 叠加 */
  rotate: 6deg;
  font-family: var(--f-body);
  font-size: 10px;
  letter-spacing: 0.2em;
  background: var(--paper);
  .date { font-size: 9px; opacity: 0.75; margin-top: 2px; letter-spacing: 0.12em; }

  /* 最后落下 */
  animation: badge-in 260ms var(--ease-out) 380ms both;
}
.alias-id { font-family: var(--f-mono); font-size: 11px; letter-spacing: 0.14em; color: var(--dim); }

/* 分段入场:ID → 标签 → 地址 → 页脚,每级差 90ms。
   这是低频的一次性揭示,值得用顺序讲清层级。 */
.alias-id { animation: rise 300ms var(--ease-out) both; }
.label    { animation: rise 300ms var(--ease-out) 90ms both; }
.email    { animation: rise 300ms var(--ease-out) 180ms both; }
.owner    { animation: rise 300ms var(--ease-out) 240ms both; }
.foot     { animation: rise 300ms var(--ease-out) 300ms both; }

@keyframes rise {
  from { opacity: 0; translate: 0 8px; }
  to   { opacity: 1; translate: 0 0; }
}
@keyframes badge-in {
  from { opacity: 0; scale: 1.35; rotate: 14deg; }
  to   { opacity: 1; scale: 1; rotate: 6deg; }
}
.label {
  margin-top: 32px;
  font-family: var(--f-body);
  font-size: 11px;
  letter-spacing: 0.24em;
  color: var(--dim);
  text-transform: uppercase;
}
.email {
  display: block;
  margin-top: 6px;
  font-family: var(--f-display);
  font-weight: 700;
  font-size: 26px;
  color: var(--ink);
  word-break: break-all;
  line-height: 1.15;
}
.owner {
  margin-top: 16px;
  display: flex; align-items: baseline; gap: 10px;
  .k { color: var(--dim); font-size: 11px; letter-spacing: 0.06em; flex: none; }
  .v { font-size: 12px; color: var(--ink); }
  .id { color: var(--dim); font-size: 11px; margin-left: auto; }
}
.foot {
  margin-top: 14px;
  display: flex; align-items: center; justify-content: space-between;
  border-top: 1px dashed var(--rule);
  padding-top: 14px;
}
.tag {
  font-family: var(--f-mono); font-size: 11px;
  border: 1px solid var(--rule); padding: 2px 8px;
  color: var(--ink);
}
.copy-btn {
  display: inline-flex; align-items: center; justify-content: center;
  min-height: var(--hit);
  background: transparent; border: 1px dashed var(--ink);
  padding: 6px 16px;
  font-family: var(--f-body); font-size: 12px; letter-spacing: 0.08em;
  cursor: pointer; color: var(--ink);
  transition:
    background-color var(--dur-fast) var(--ease-out),
    color var(--dur-fast) var(--ease-out),
    scale var(--dur-fast) var(--ease-out);
  &:hover { background: var(--ink); color: var(--paper); border-style: solid; }
  &:active { scale: 0.96; }
  &:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
}
.actions { text-align: right; }
</style>
