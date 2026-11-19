<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { formatAPITime } from '@/utils/datetime';
import RemoteTable from '@/components/RemoteTable.vue';
import { listAuditLogs, type AuditLog } from '@/service/api/goddi/logs';

defineOptions({ name: 'AuditLogsPage' });

/** Starter list page: TCard page header + shared remote TTable. */

const { t } = useI18n();
function actionLabel(v: string) { const key = `logs.audit.actionValues.${v.toLowerCase()}`; const out = t(key); return out === key ? v : out; }

const columns: PrimaryTableCol[] = [
  { colKey: 'username', title: t('logs.audit.user'), width: 140 },
  {
    colKey: 'action',
    title: t('logs.audit.action'),
    width: 140,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => actionLabel(String((row as AuditLog).action ?? '')))
  },
  { colKey: 'resource', title: t('logs.audit.resource'), width: 140 },
  { colKey: 'resource_id', title: t('logs.audit.resourceId'), width: 160, ellipsis: true },
  { colKey: 'details', title: t('logs.audit.details'), ellipsis: true },
  { colKey: 'ip', title: t('logs.audit.ip'), width: 140 },
  { colKey: 'created_at', title: t('common.createdAt'), width: 180, cell: (_h, { row }) => formatAPITime((row as any).created_at) }
];
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('logs.audit.title') }}</h2>
      </div>
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
</style>