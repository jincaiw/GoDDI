<script setup lang="ts">
import { namedDeleteBody } from '@/utils/confirm';
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, MessagePlugin, Tag as TTag, Link as TLink } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import { dhcpLeaseStatusLabel } from '@/composables/useDhcpStatusLabels';
import { formatAPITime } from '@/utils/datetime';
import { deleteDHCPLease, listDHCPLeases, type DHCPLease } from '@/service/api/goddi/dhcp';

defineOptions({ name: 'DHCPLeasesPage' });

/** Starter list page: TCard header + TCard filter bar + shared remote TTable. */
const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const searchQuery = ref('');
const statusFilter = ref<string | undefined>(undefined);

const statusValues = ['active', 'offered', 'expired', 'released', 'conflict'];
const statusOptions = statusValues.map(value => ({ label: dhcpLeaseStatusLabel(t, value), value }));

// An offer is pending and a conflict is an address held out of the pool after
// a DECLINE; neither should look like a confirmed binding.
const statusTheme: Record<string, 'success' | 'primary' | 'warning' | 'danger' | 'default'> = {
  active: 'success',
  offered: 'primary',
  conflict: 'warning',
  expired: 'default',
  released: 'default'
};

const params = computed<Record<string, unknown>>(() => {
  const p: Record<string, unknown> = {};
  if (searchQuery.value) p.search = searchQuery.value;
  if (statusFilter.value) p.status = statusFilter.value;
  return p;
});

const columns: PrimaryTableCol[] = [
  { colKey: 'ip', title: t('dhcp.leases.ip'), width: 150 },
  { colKey: 'mac', title: t('dhcp.leases.mac'), width: 170 },
  { colKey: 'hostname', title: t('dhcp.leases.hostname'), ellipsis: true },
  { colKey: 'scope_name', title: t('dhcp.leases.scope'), ellipsis: true },
  {
    colKey: 'status',
    title: t('common.status'),
    width: 110,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: statusTheme[(row as DHCPLease).status] ?? 'default', variant: 'light' }, () =>
        dhcpLeaseStatusLabel(t, String((row as DHCPLease).status))
      )
  },
  { colKey: 'lease_start', title: t('dhcp.leases.startTime'), width: 170, cell: (_h, { row }) => formatAPITime((row as DHCPLease).lease_start) },
  { colKey: 'lease_end', title: t('dhcp.leases.endTime'), width: 170, cell: (_h, { row }) => formatAPITime((row as DHCPLease).lease_end) },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TLink,
        {
          theme: 'danger',
          disabled: !perm.canDelete('dhcp'),
          onClick: () => confirmRelease((row as DHCPLease).id)
        },
        () => t('dhcp.leases.release')
      )
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function confirmRelease(id: string) {
  if (!perm.canDelete('dhcp')) return;
  const dlg = DialogPlugin.confirm({
    header: t('dhcp.leases.release'),
    body: namedDeleteBody(id),
    theme: 'danger',
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
    onConfirm: async () => {
      try {
        await deleteDHCPLease(id);
        void MessagePlugin.success(t('common.success'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('dhcp.leases.title') }}</h2>
      </div>
      <TButton variant="outline" @click="tableRef?.reload()">
        <template #icon><TIcon name="refresh" /></template>
        {{ t('common.refresh') }}
      </TButton>
    </TCard>

    <TCard :bordered="false" class="filter-bar">
      <TSpace>
        <TInput
          v-model="searchQuery"
          clearable
          :placeholder="t('common.search') + ' IP/MAC'"
          style="width: 240px"
          @enter="tableRef?.reload()"
        />
        <TSelect
          v-model="statusFilter"
          :options="statusOptions"
          clearable
          :placeholder="t('common.status')"
          style="width: 150px"
        />
      </TSpace>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listDHCPLeases" :params="params" />
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
}
.filter-bar :deep(.t-card__body) {
  padding: 12px 20px;
}
</style>