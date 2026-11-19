<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { formatAPITime } from '@/utils/datetime';
import RemoteTable from '@/components/RemoteTable.vue';
import { listDHCPLogs, type DHCPLog } from '@/service/api/goddi/dhcp';

defineOptions({ name: 'DHCPLogsPage' });

/** Starter list page: TCard page header + shared remote TTable. */

const { t } = useI18n();
function eventLabel(v: string) { const key = `logs.dhcp.eventValues.${v.toLowerCase()}`; const out = t(key); return out === key ? v : out; }

const columns: PrimaryTableCol[] = [
  {
    colKey: 'event_type',
    title: t('logs.dhcp.eventType'),
    width: 130,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => eventLabel(String((row as DHCPLog).event_type ?? '')))
  },
  { colKey: 'client_mac', title: t('logs.dhcp.clientMac'), width: 160 },
  { colKey: 'client_ip', title: t('logs.dhcp.clientIp'), width: 140 },
  { colKey: 'hostname', title: t('logs.dhcp.hostname'), width: 180, ellipsis: true },
  { colKey: 'scope_name', title: t('logs.dhcp.scope'), width: 160, ellipsis: true },
  { colKey: 'message', title: t('logs.dhcp.message'), minWidth: 240, ellipsis: true },
  { colKey: 'created_at', title: t('common.createdAt'), width: 180, cell: (_h, { row }) => formatAPITime((row as any).created_at) }
];
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('logs.dhcp.title') }}</h2>
      </div>
    </TCard>
    <RemoteTable :columns="columns" :fetcher="listDHCPLogs" />
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