import { computed, ref, watch, type Ref } from 'vue'

/** 每页条数的可选项。各列表页共用一份,分页控件在哪一页都长一样。 */
export const PAGE_SIZES = [10, 20, 50, 100]

/**
 * usePagination 把一份完整列表切成分页视图。
 *
 * 只给那些接口本来就返回全量、且全量就是全部的列表用(别名、账号、token):
 * 切片在前端做,解决的是渲染和翻找,不减少请求量。别名列表尤其如此 ——
 * iCloud 上游的接口不支持分页,后端也得整份拉回来,改成翻一页请求一次
 * 反而会成倍放大对上游的调用。
 *
 * 收件箱不走这里。邮件可能有几百封、每封正文几百 KB,而且后端能给出准确
 * 总数,所以那边是真正的服务端分页 —— 拿一批局部数据在前端切页,会让人
 * 以为翻到底就是全部。
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
