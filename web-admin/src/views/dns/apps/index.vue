<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { listApps, type DNSApp } from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSAppsPage' });

/**
 * App marketplace page.
 *
 * The backend /api/v1/apps endpoints return 501 in this release, so this page
 * renders an explicit "not available" empty state. It is a real product gap,
 * not a UI migration issue (see docs/audit/RUNTIME-VALIDATION-LEGACY.md).
 */
const { t } = useI18n();
const available = ref(false);
const apps = ref<DNSApp[]>([]);
const loading = ref(false);
const error = ref('');

const columns: PrimaryTableCol[] = [
  { colKey: 'id', title: t('dns.apps.colId') },
  { colKey: 'name', title: t('dns.apps.colName') },
  { colKey: 'version', title: t('dns.apps.colVersion'), width: 100 },
  {
    colKey: 'enabled',
    title: t('dns.apps.colStatus'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TTag,
        { size: 'small', theme: (row as DNSApp).enabled ? 'success' : 'default', variant: 'light' },
        () => ((row as DNSApp).enabled ? t('dns.apps.statusOn') : t('dns.apps.statusOff'))
      )
  }
];

async function loadApps() {
  loading.value = true;
  error.value = '';
  try {
    const result = await listApps();
    apps.value = result ?? [];
    available.value = apps.value.length > 0;
  } catch (err) {
    apps.value = [];
    available.value = false;
    error.value = err instanceof Error ? err.message : t('common.failed');
  } finally {
    loading.value = false;
  }
}

onMounted(loadApps);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('dns.apps.title') }}</h2>
      </div>
    </TCard>
    <TCard :bordered="false">
      <TAlert v-if="error" theme="error" role="alert" :message="error" class="apps-error">
        <template #operation>
          <TButton size="small" theme="primary" variant="outline" @click="loadApps">{{ t('common.retry') }}</TButton>
        </template>
      </TAlert>
      <TLoading :loading="loading" size="small">
        <TTable v-if="available" :columns="columns" :data="apps" row-key="id" stripe hover />
        <TEmpty v-else :description="t('dns.apps.notAvailable')">
          <template #image><TIcon name="app" size="48px" /></template>
          <template #action>
            <TTag theme="warning" variant="light-outline">{{ t('dns.apps.notImplemented') }}</TTag>
            <p class="apps-hint">{{ t('dns.apps.hint') }}</p>
          </template>
        </TEmpty>
      </TLoading>
    </TCard>
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  padding: 16px 20px;
}
.apps-hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
