<script setup lang="ts">
import { h } from 'vue';
import { useI18n } from 'vue-i18n';
import { Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { formatAPITime } from '@/utils/datetime';
import RemoteTable from '@/components/RemoteTable.vue';
import { listDNSQueryLogs, type DNSQueryLog } from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSQueryLogsPage' });

/** Starter list page: TCard page header + shared remote TTable. */

const { t } = useI18n();

function yesNo(value: boolean) {
  return h(
    TTag,
    { size: 'small', theme: value ? 'success' : 'default', variant: 'light' },
    () => (value ? t('common.yes') : t('common.no'))
  );
}

const columns: PrimaryTableCol[] = [
  { colKey: 'query_name', title: t('logs.dns.queryName'), minWidth: 220, ellipsis: true },
  { colKey: 'query_type', title: t('logs.dns.queryType'), width: 100 },
  { colKey: 'client_ip', title: t('logs.dns.clientIp'), width: 140 },
  { colKey: 'response_code', title: t('logs.dns.responseCode'), width: 130 },
  {
    colKey: 'response_time_ms',
    title: t('logs.dns.responseTime'),
    width: 110,
    cell: (_h, { row }) => `${(row as DNSQueryLog).response_time_ms} ms`
  },
  { colKey: 'cached', title: t('logs.dns.cached'), width: 100, cell: (_h, { row }) => yesNo(Boolean((row as DNSQueryLog).cached)) },
  { colKey: 'blocked', title: t('logs.dns.blocked'), width: 100, cell: (_h, { row }) => yesNo(Boolean((row as DNSQueryLog).blocked)) },
  { colKey: 'upstream', title: t('logs.dns.upstream'), width: 160, ellipsis: true },
  { colKey: 'created_at', title: t('common.createdAt'), width: 180, cell: (_h, { row }) => formatAPITime((row as any).created_at) }
];
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('logs.dns.title') }}</h2>
      </div>
    </TCard>
    <RemoteTable :columns="columns" :fetcher="listDNSQueryLogs" />
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