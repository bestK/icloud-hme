<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { CreateResult } from '@/types'

defineProps<{ result: CreateResult | null; open: boolean }>()
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
    class="reveal"
  >
    <div v-if="result" class="stamp">
      <div class="perf-frame">
        <div class="postmark">
          <div>{{ result.source === 'pool' ? 'FROM POOL' : 'LIVE ISSUE' }}</div>
          <div class="date">{{ new Date(result.created_at).toISOString().slice(0, 10) }}</div>
        </div>
        <div class="denom">Nº {{ result.anonymous_id.slice(0, 6).toUpperCase() }}</div>
        <div class="label">你的新地址</div>
        <div class="email" @click="copy(result.email)">{{ result.email }}</div>
        <div class="foot">
          <span class="tag">{{ result.label || 'unlabeled' }}</span>
          <button class="tear" @click="copy(result.email)">轻撕复制</button>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="actions">
        <el-button plain @click="emit('close')">收起</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style lang="scss" scoped>
.reveal :deep(.el-dialog) { background: var(--bg); }

.stamp { padding: 8px 4px 0; }
.perf-frame {
  background: var(--paper);
  border: 1px solid var(--ink);
  padding: 24px 22px 20px;
  position: relative;
  mask-image:
    radial-gradient(circle at center, transparent 3px, black 3.5px) 0 -6px / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) 0 100% / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) -6px 0 / 12px 12px,
    radial-gradient(circle at center, transparent 3px, black 3.5px) 100% 0 / 12px 12px,
    linear-gradient(black, black);
  mask-composite: intersect;
}
.postmark {
  position: absolute; top: 14px; right: -12px;
  border: 2px solid var(--stamp);
  color: var(--stamp);
  padding: 6px 10px;
  transform: rotate(6deg);
  font-family: var(--f-body);
  font-size: 10px;
  letter-spacing: 0.2em;
  background: var(--paper);
  .date { font-size: 9px; opacity: 0.75; margin-top: 2px; letter-spacing: 0.12em; }
}
.denom { font-family: var(--f-mono); font-size: 11px; letter-spacing: 0.14em; color: var(--dim); }
.label {
  margin-top: 32px;
  font-family: var(--f-body);
  font-size: 11px;
  letter-spacing: 0.24em;
  color: var(--dim);
  text-transform: uppercase;
}
.email {
  margin-top: 6px;
  font-family: var(--f-display);
  font-weight: 700;
  font-size: 26px;
  color: var(--ink);
  cursor: pointer;
  word-break: break-all;
  line-height: 1.15;
  &:hover { color: var(--primary); }
}
.foot {
  margin-top: 20px;
  display: flex; align-items: center; justify-content: space-between;
  border-top: 1px dashed var(--rule);
  padding-top: 14px;
}
.tag {
  font-family: var(--f-mono); font-size: 11px;
  border: 1px solid var(--rule); padding: 2px 8px;
  color: var(--ink);
}
.tear {
  background: transparent; border: 1px dashed var(--ink);
  padding: 6px 14px;
  font-family: var(--f-body); font-size: 12px; letter-spacing: 0.08em;
  cursor: pointer; color: var(--ink);
  &:hover { background: var(--ink); color: var(--paper); border-style: solid; }
}
.actions { text-align: right; }
</style>
