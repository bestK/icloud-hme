import { computed, ref, watch, type Ref } from 'vue'

/** 每页条数的可选项。各列表页共用一份,分页控件在哪一页都长一样。 */
export const PAGE_SIZES = [10, 20, 50, 100]

/**
 * usePagination 把一份完整列表切成分页视图。
 *
 * 上游几个接口(别名、账号、token、收件箱)都是一次性返回全量,没有服务端
 * 分页可用,所以切片只在前端做 —— 它解决的是渲染和翻找,不减少请求量。
 */
export function usePagination<T>(source: Ref<T[]>, initialSize = 20) {
  const page = ref(1)
  const pageSize = ref(initialSize)

  const total = computed(() => source.value.length)
  const paged = computed(() =>
    source.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
  )

  // 删到最后一页空了、或者把每页条数调大,都不能把人留在一张空页上
  watch([total, pageSize], () => {
    const last = Math.max(1, Math.ceil(total.value / pageSize.value))
    if (page.value > last) page.value = last
  })

  /** 换了筛选条件(比如切账号)时调用,让人从第一页看起 */
  const reset = () => {
    page.value = 1
  }

  return { page, pageSize, total, paged, reset }
}
