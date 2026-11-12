<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import type { PageInfo, PrimaryTableCol } from 'tdesign-vue-next';

/**
 * Shared TDesign remote-paginated table.
 *
 * Extracted from the repeated Naive UI NDataTable pattern in the log/list
 * pages so remaining migrations only declare columns + a fetcher. Keeps the
 * backend contract unchanged: fetcher receives {page, page_size} (merged with
 * optional reactive `params`) and returns the `{data, meta:{total}}` shape
 * produced by service/api/goddi/client.ts getList(). Loading, error and
 * pagination state live here. `params` is deep-watched so filter changes
 * reload page 1 automatically.
 */
const props = withDefaults(
  defineProps<{
    columns: PrimaryTableCol[];
    fetcher: (params: Record<string, unknown>) => Promise<{ data: unknown[]; meta: { total: number } }>;
    rowKey?: string;
    pageSize?: number;
    params?: Record<string, unknown>;
    showPagination?: boolean;
    /** Prepends a multiple-selection column and syncs keys with v-model:selectedKeys. */
    selectable?: boolean;
    /** Forwarded to t-table: string or ({row}) => string. */
    rowClassName?: string | ((params: { row: unknown }) => string);
  }>(),
  { rowKey: 'id', pageSize: 20, params: () => ({}), showPagination: true, selectable: false, rowClassName: '' }
);

const selectedKeys = defineModel<Array<string | number>>('selectedKeys', { default: () => [] });

const tableColumns = computed<PrimaryTableCol[]>(() =>
  props.selectable
    ? [{ colKey: 'row-select', type: 'multiple', width: 50 }, ...props.columns]
    : props.columns
);

const rows = ref<unknown[]>([]);
const loading = ref(false);
const error = ref('');
const pagination = reactive({
  current: 1,
  pageSize: props.pageSize,
  total: 0,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50]
});

async function reload() {
  loading.value = true;
  error.value = '';
  try {
    const result = await props.fetcher({
      ...props.params,
      page: pagination.current,
      page_size: pagination.pageSize
    });
    rows.value = result.data;
    pagination.total = result.meta?.total ?? result.data.length;
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Request failed';
  } finally {
    loading.value = false;
  }
}

function onPageChange(info: PageInfo) {
  pagination.current = info.current;
  pagination.pageSize = info.pageSize;
  void reload();
}

onMounted(reload);

watch(
  () => props.params,
  () => {
    pagination.current = 1;
    void reload();
  },
  { deep: true }
);

defineExpose({ reload });
</script>

<template>
  <div class="remote-table">
    <TAlert v-if="error" theme="error" :message="error" close class="remote-table-error" @close="error = ''" />
    <TCard :bordered="false">
      <TTable
        :data="rows"
        :columns="tableColumns"
        :loading="loading"
        :row-key="rowKey"
        :row-class-name="rowClassName"
        :selected-row-keys="selectable ? selectedKeys : undefined"
        stripe
        hover
        :pagination="showPagination ? {
          current: pagination.current,
          pageSize: pagination.pageSize,
          total: pagination.total,
          showPageSize: pagination.showPageSize,
          pageSizeOptions: pagination.pageSizeOptions
        } : undefined"
        @select-change="(keys: Array<string | number>) => (selectedKeys = keys)"
        @page-change="onPageChange"
      />
    </TCard>
  </div>
</template>

<style scoped>
.remote-table-error {
  margin-bottom: 16px;
}
</style>
