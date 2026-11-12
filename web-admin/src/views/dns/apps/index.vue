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

const columns: PrimaryTableCol[] = [
  { colKey: 'id', title: 'ID' },
  { colKey: 'name', title: 'Name' },
  { colKey: 'version', title: 'Version', width: 100 },
  {
    colKey: 'enabled',
    title: 'Status',
    width: 100,
    cell: (h, { row }) =>
      h(
        TTag,
        { size: 'small', theme: (row as DNSApp).enabled ? 'success' : 'default', variant: 'light' },
        () => ((row as DNSApp).enabled ? 'ON' : 'OFF')
      )
  }
];

onMounted(async () => {
  try {
    const result = await listApps();
    apps.value = result ?? [];
    available.value = apps.value.length > 0;
  } catch {
    available.value = false;
  }
});
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dns.apps.title') }}</h2>
    </TCard>
    <TCard :bordered="false">
      <TTable v-if="available" :columns="columns" :data="apps" row-key="id" stripe hover />
      <TEmpty v-else :description="t('dns.apps.notAvailable')">
        <template #image><TIcon name="app" size="48px" /></template>
        <template #action>
          <TTag theme="warning" variant="light-outline">501 Not Implemented</TTag>
          <p class="apps-hint">{{ t('dns.apps.hint') }}</p>
        </template>
      </TEmpty>
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
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.apps-hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: #9ca3af;
}
</style>
