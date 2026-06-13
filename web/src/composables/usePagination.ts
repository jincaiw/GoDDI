import { ref, type Ref } from 'vue'

export function usePagination(fetchFn: (page: number, pageSize: number, params?: Record<string, unknown>) => Promise<{ data: unknown[]; meta: { total: number; page: number; page_size: number; pages: number } }>) {
  const page = ref(1)
  const pageSize = ref(20)
  const total = ref(0)
  const pageCount = ref(0)
  const data: Ref<unknown[]> = ref([])
  const loading = ref(false)

  async function fetchData(params?: Record<string, unknown>) {
    loading.value = true
    try {
      const result = await fetchFn(page.value, pageSize.value, params)
      data.value = result.data
      total.value = result.meta.total
      pageCount.value = result.meta.pages
    } finally {
      loading.value = false
    }
  }

  function handlePageChange(newPage: number) {
    page.value = newPage
    fetchData()
  }

  function handlePageSizeChange(newPageSize: number) {
    pageSize.value = newPageSize
    page.value = 1
    fetchData()
  }

  function reset() {
    page.value = 1
    fetchData()
  }

  return {
    page,
    pageSize,
    total,
    pageCount,
    data,
    loading,
    fetchData,
    handlePageChange,
    handlePageSizeChange,
    reset,
  }
}
