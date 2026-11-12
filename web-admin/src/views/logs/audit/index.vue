<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { listAuditLogs, type AuditLog } from '@/service/api/goddi/logs';

defineOptions({ name: 'AuditLogsPage' });

const { t } = useI18n();

const columns: PrimaryTableCol[] = [
  { colKey: 'username', title: t('logs.audit.user'), width: 140 },
  {
    colKey: 'action',
    title: t('logs.audit.action'),
    width: 140,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as AuditLog).action ?? ''))
  },
  { colKey: 'resource', title: t('logs.audit.resource'), width: 140 },
  { colKey: 'resource_id', title: t('logs.audit.resourceId'), width: 160, ellipsis: true },
  { colKey: 'details', title: t('logs.audit.details'), ellipsis: true },
  { colKey: 'ip', title: t('logs.audit.ip'), width: 140 },
  { colKey: 'created_at', title: t('common.createdAt'), width: 180 }
];
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('logs.audit.title') }}</h2>
    </TCard>
    <RemoteTable :columns="columns" :fetcher="listAuditLogs" />
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
</style>
