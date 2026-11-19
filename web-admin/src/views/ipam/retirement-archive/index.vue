<script setup lang="ts">
import { h, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Tag as TTag } from 'tdesign-vue-next';
import { formatAPITime } from '@/utils/datetime';
import {
  listIPAMRetirementAudit,
  purgeIPAMSpace,
  purgeIPAMSubnet,
  restoreIPAMSpace,
  restoreIPAMSubnet,
  type RetirementAuditEntry
} from '@/service/api/goddi/ipam';
import { usePermission } from '@/composables/usePermission';
import RemoteTable from '@/components/RemoteTable.vue';

defineOptions({ name: 'IPAMRetirementArchivePage' });

const { t } = useI18n();
const perm = usePermission();
const tableRef = ref<{ reload: () => Promise<void> } | null>(null);

function kindLabel(kind: string): string {
  const key = `ipam.archive.kindValues.${kind}`;
  const out = t(key);
  return out === key ? kind : out;
}

const columns: PrimaryTableCol[] = [
  {
    colKey: 'kind',
    title: t('ipam.archive.kind'),
    width: 110,
    cell: (_h, { row }) =>
      h(TTag, { size: 'small', variant: 'light' }, () => kindLabel((row as RetirementAuditEntry).kind))
  },
  { colKey: 'name', title: t('common.name'), minWidth: 180, ellipsis: true },
  { colKey: 'id', title: t('ipam.archive.resourceId'), width: 260, ellipsis: true },
  {
    colKey: 'retired_at',
    title: t('ipam.archive.retiredAt'),
    width: 170,
    cell: (_h, { row }) => formatAPITime((row as RetirementAuditEntry).retired_at)
  },
  { colKey: 'retired_by', title: t('ipam.archive.retiredBy'), width: 140 },
  { colKey: 'retire_reason', title: t('ipam.archive.reason'), minWidth: 160, ellipsis: true },
  { colKey: 'allocated_children', title: t('ipam.archive.allocatedChildren'), width: 150 },
  { colKey: 'history_children', title: t('ipam.archive.historyChildren'), width: 140 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (_h, { row }) => {
      const r = row as RetirementAuditEntry;
      return h(
        TSpace,
        { size: 'small' },
        () => [
          h(
            TLink,
            {
              theme: 'primary',
              disabled: !perm.canWrite('ipam'),
              onClick: () => confirmRestore(r)
            },
            () => t('ipam.archive.restore')
          ),
          h(
            TLink,
            {
              theme: 'danger',
              disabled: !perm.canDelete('ipam') || r.allocated_children > 0 || r.history_children > 0,
              onClick: () => confirmPurge(r)
            },
            () => t('ipam.archive.purge')
          )
        ]
      );
    }
  }
];

function confirmRestore(row: RetirementAuditEntry) {
  if (!perm.canWrite('ipam')) return;
  const dlg = DialogPlugin.confirm({
    header: t('ipam.archive.restore'),
    body: t('ipam.archive.restoreConfirm', { name: row.name }),
    confirmBtn: { content: t('common.confirm') },
    onConfirm: async () => {
      try {
        if (row.kind === 'space') await restoreIPAMSpace(row.id);
        else await restoreIPAMSubnet(row.id);
        void MessagePlugin.success(t('ipam.archive.restoreSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
      }
      dlg.hide();
    }
  });
}

function confirmPurge(row: RetirementAuditEntry) {
  if (!perm.canDelete('ipam')) return;
  const dlg = DialogPlugin.confirm({
    header: t('ipam.archive.purge'),
    body: t('ipam.archive.purgeConfirm', { name: row.name }),
    theme: 'danger',
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
    onConfirm: async () => {
      try {
        if (row.kind === 'space') await purgeIPAMSpace(row.id);
        else await purgeIPAMSubnet(row.id);
        void MessagePlugin.success(t('ipam.archive.purgeSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
      }
      dlg.hide();
    }
  });
}

async function fetchAudit() {
  const data = await listIPAMRetirementAudit();
  return { data, meta: { total: data.length } };
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('ipam.archive.title') }}</h2>
    </TCard>
    <TAlert theme="info" :message="t('ipam.archive.localOnlyHint')" />
    <RemoteTable
      ref="tableRef"
      :columns="columns"
      :fetcher="fetchAudit"
      :show-pagination="false"
      :empty-text="t('ipam.archive.empty')"
    />
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