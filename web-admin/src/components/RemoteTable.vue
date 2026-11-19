<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
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
    /** Localized empty text; when omitted TDesign's default empty state is used. */
    emptyText?: string;
  }>(),
  { rowKey: 'id', pageSize: 20, params: () => ({}), showPagination: true, selectable: false, rowClassName: '', emptyText: '' }
);

const { t } = useI18n();

const selectedKeys = defineModel<Array<string | number>>('selectedKeys', { default: () => [] });

const tableColumns = computed<PrimaryTableCol[]>(() => {
  const columns: PrimaryTableCol[] = props.columns.map(column => ({
    ...column,
    // TDesign defaults can squeeze Chinese labels into single-character
    // lines. Give every data column a readable minimum and keep wide action
    // columns scrollable instead of wrapping operation links.
    minWidth: (column as { minWidth?: number }).minWidth ?? 120
  }));

  // Pin the action column to the right edge so it stays reachable while the
  // data columns scroll. CSS sticky cannot work inside TDesign's own scroll
  // container (verified in task 32), so use the native `fixed` column support.
  const lastIndex = columns.length - 1;
  if (lastIndex > 0 && (columns[lastIndex] as { colKey?: string }).colKey === 'op') {
    columns[lastIndex] = { ...columns[lastIndex], fixed: 'right' };
  }

  return props.selectable ? [{ colKey: 'row-select', type: 'multiple', width: 50 }, ...columns] : columns;
});

// Keep the last successful page in memory so returning to a cached page does
// not flash an empty table while the request is in flight.
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
  <!--
 aria-busy lets assistive technology announce that the table is
       refreshing, and the polite live region reports the outcome without
       stealing focus.
-->
  <div class="remote-table" :aria-busy="loading ? 'true' : 'false'">
    <p class="remote-table-status" role="status" aria-live="polite">
      {{ loading ? t('common.loading') : t('common.loadedRows', { total: pagination.total }) }}
    </p>
    <TAlert v-if="error" theme="error" role="alert" :message="error" close class="remote-table-error" @close="error = ''">
      <template #operation>
        <TButton size="small" theme="primary" variant="outline" @click="reload">{{ t('common.retry') }}</TButton>
      </template>
    </TAlert>
    <TCard :bordered="false" class="remote-table-card">
      <div class="remote-table-scroll">
        <TTable
          :data="rows"
          :columns="tableColumns"
          :loading="loading"
          :empty="emptyText || undefined"
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
      </div>
    </TCard>
  </div>
</template>

<style scoped>
.remote-table-status {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}
.remote-table-card :deep(.t-card__body) {
  padding: 0;
}
.remote-table-scroll {
  width: 100%;
  overflow-x: auto;
  overscroll-behavior-x: contain;
}
.remote-table-scroll :deep(.t-table) {
  min-width: 100%;
}
.remote-table-scroll :deep(.t-table__content) {
  white-space: nowrap;
}
.remote-table-error {
  margin-bottom: 16px;
}
</style>