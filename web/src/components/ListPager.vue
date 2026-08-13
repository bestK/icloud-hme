<script setup lang="ts">
import { ref, watch } from 'vue'
import { PAGE_SIZES } from '@/composables/usePagination'

defineProps<{ total: number }>()

const page = defineModel<number>('page', { required: true })
const pageSize = defineModel<number>('pageSize', { required: true })

const root = ref<HTMLElement>()

// 在长列表底部点"下一页",视口还停在原处 —— 新一页是从中间开始看的。
// 翻页后把紧挨着的那段列表带回视野顶部。
watch(page, () => {
  const list = root.value?.previousElementSibling
  if (!list) return
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  list.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'start' })
})
</script>

<template>
  <div v-if="total" ref="root" class="pager">
    <el-pagination
      v-model:current-page="page"
      v-model:page-size="pageSize"
      :total="total"
      :page-sizes="PAGE_SIZES"
      layout="total, sizes, prev, pager, next, jumper"
      size="large"
      background
      hide-on-single-page
    />
  </div>
</template>

<style lang="scss" scoped>
/* 间距挂在控件本身而不是外层:只有一页时 el-pagination 整个不渲染,
   留下的空 div 不该还占着 18px */
:deep(.el-pagination) {
  width: 100%;
  margin-top: 18px;
}

/* size="large" 已经给到 40px 高,正好是这套界面的最小点击区 */
:deep(.el-pager li),
:deep(.btn-prev),
:deep(.btn-next) {
  border-radius: 0;
  border: 1px solid var(--rule);
  /* 页码在翻页时会变宽窄,等宽数字才不会让整条控件左右跳 */
  font-variant-numeric: tabular-nums;
  transition:
    background-color var(--dur-fast) var(--ease-out),
    border-color var(--dur-fast) var(--ease-out),
    color var(--dur-fast) var(--ease-out),
    scale var(--dur-fast) var(--ease-out);
}
/* 当前页是实心主色块,再套一圈灰边会像没选中 */
:deep(.el-pager li.is-active) {
  border-color: var(--primary);
}
:deep(.el-pager li:not(.is-active):hover),
:deep(.btn-prev:not(:disabled):hover),
:deep(.btn-next:not(:disabled):hover) {
  border-color: var(--ink);
}
:deep(.el-pager li:not(.is-disabled):active),
:deep(.btn-prev:not(:disabled):active),
:deep(.btn-next:not(:disabled):active) {
  scale: 0.96;
}
:deep(.el-pager li:focus-visible),
:deep(.btn-prev:focus-visible),
:deep(.btn-next:focus-visible) {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

/* 总数和跳页里的数字同样会变宽窄 */
:deep(.el-pagination__total),
:deep(.el-pagination__jump) {
  font-variant-numeric: tabular-nums;
}
/* 总数靠左、翻页靠右,中间留白 —— 列表越窄越不会挤成一坨 */
:deep(.el-pagination__total) { margin-right: auto; }
:deep(.el-pagination__jump) { margin-left: 14px; }
</style>
