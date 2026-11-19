<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import SectionTitle from '@/components/ipam/SectionTitle.vue';
import { ipamStatusLabel, ipamStatusTheme } from '@/composables/useIpamLifecycle';
import {
  getIPAMAddressView,
  type AddressHistoryEntry,
  type AddressView,
  type LeaseSummary,
  type PublishingRecord,
  type ReservationSummary,
  type ScopeSummary
} from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMAddressDetailDrawer' });

const props = defineProps<{
  spaceId: string;
  ip: string;
}>();

// v-model:show is a model ref, the same idiom RemoteTable uses, instead of a
// hand-written props/emit getter-setter pair.
const show = defineModel<boolean>('show', { required: true });

const { t } = useI18n();

const loading = ref(false);
const view = ref<AddressView | null>(null);
const loadFailed = ref('');

const title = computed(() => (props.ip ? `${t('ipam.detail.title')} · ${props.ip}` : t('ipam.detail.title')));


function historyTitle(entry: AddressHistoryEntry) {
  if (entry.old_status && entry.new_status) return `${entry.action}: ${ipamStatusLabel(t, entry.old_status)} → ${ipamStatusLabel(t, entry.new_status)}`;
  return entry.new_status ? `${entry.action}: ${ipamStatusLabel(t, entry.new_status)}` : entry.action;
}

const dnsColumns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('ipam.detail.recordName'), ellipsis: true },
  { colKey: 'type', title: t('ipam.detail.recordType'), width: 80 },
  { colKey: 'value', title: t('ipam.detail.recordValue'), ellipsis: true },
  { colKey: 'ttl', title: t('ipam.detail.ttl'), width: 80 },
  {
    colKey: 'enabled',
    title: t('common.status'),
    width: 100,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as PublishingRecord).enabled ? 'success' : 'default', variant: 'light' }, () =>
        (row as PublishingRecord).enabled ? t('common.enabled') : t('common.disabled')
      )
  },
  {
    colKey: 'authored_locally',
    title: t('ipam.detail.author'),
    width: 120,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as PublishingRecord).authored_locally ? 'warning' : 'default', variant: 'light' }, () =>
        (row as PublishingRecord).authored_locally ? t('ipam.detail.authorLocal') : t('ipam.detail.authorOperator')
      )
  }
];

const scopeColumns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name'), ellipsis: true },
  {
    colKey: 'range',
    title: t('ipam.detail.range'),
    cell: (_h, { row }) => `${(row as ScopeSummary).start_ip} – ${(row as ScopeSummary).end_ip}`
  },
  {
    colKey: 'router',
    title: t('ipam.detail.router'),
    width: 130,
    cell: (_h, { row }) => (row as ScopeSummary).router || '—'
  },
  {
    colKey: 'relation',
    title: t('ipam.detail.relation'),
    width: 200,
    cell: (h, { row }) => {
      const r = row as ScopeSummary;
      const tags: Array<ReturnType<typeof h>> = [];
      if (r.in_subnet) tags.push(h(TTag, { size: 'small', variant: 'light' }, () => t('ipam.detail.inSubnet')));
      if (r.in_pool) tags.push(h(TTag, { size: 'small', theme: 'warning', variant: 'light' }, () => t('ipam.detail.inPool')));
      if (r.is_gateway) tags.push(h(TTag, { size: 'small', theme: 'primary', variant: 'light' }, () => t('ipam.detail.isGateway')));
      if (!r.enabled) tags.push(h(TTag, { size: 'small', variant: 'light' }, () => t('common.disabled')));
      return h('div', { class: 'tag-row' }, tags);
    }
  }
];

const leaseColumns: PrimaryTableCol[] = [
  { colKey: 'mac_address', title: t('ipam.addresses.mac'), width: 150 },
  {
    colKey: 'hostname',
    title: t('ipam.addresses.hostname'),
    cell: (_h, { row }) => (row as LeaseSummary).hostname || '—'
  },
  {
    colKey: 'status',
    title: t('common.status'),
    width: 110,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => ipamStatusLabel(t, String((row as LeaseSummary).status)))
  },
  { colKey: 'lease_end', title: t('ipam.detail.leaseEnd'), width: 180 }
];

const reservationColumns: PrimaryTableCol[] = [
  { colKey: 'mac_address', title: t('ipam.addresses.mac'), width: 150 },
  {
    colKey: 'hostname',
    title: t('ipam.addresses.hostname'),
    cell: (_h, { row }) => (row as ReservationSummary).hostname || '—'
  },
  {
    colKey: 'enabled',
    title: t('common.status'),
    width: 110,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as ReservationSummary).enabled ? 'success' : 'default', variant: 'light' }, () =>
        (row as ReservationSummary).enabled ? t('common.enabled') : t('common.disabled')
      )
  }
];

async function load() {
  if (!props.spaceId || !props.ip) return;
  loading.value = true;
  loadFailed.value = '';
  try {
    view.value = await getIPAMAddressView(props.spaceId, props.ip);
  } catch (err: unknown) {
    // Keep the failure in the body: a blank drawer looks like an address with
    // nothing attached, which is a different answer.
    view.value = null;
    loadFailed.value = err instanceof Error ? err.message : t('common.failed');
  } finally {
    loading.value = false;
  }
}

watch(
  () => [show.value, props.spaceId, props.ip] as const,
  ([visible]) => {
    if (visible) {
      view.value = null;
      void load();
    }
  },
  { immediate: true }
);
</script>

<template>
  <TDrawer v-model:visible="show" :header="title" size="700px" placement="right" :footer="false">
    <TLoading :loading="loading" size="small">
      <template v-if="view">
        <TAlert
          v-if="!view.access.dns || !view.access.dhcp || view.access.dns_partial"
          theme="info"
          :message="t('ipam.detail.permissionLimited')"
          class="mb-4"
        />
        <!-- The four subsystems can disagree about one address; report it. -->
        <TAlert v-if="view.conflicts.length > 0" theme="warning" :title="t('ipam.detail.conflicts')" class="mb-4">
          <ul class="conflict-list">
            <li v-for="(conflict, index) in view.conflicts" :key="index">{{ conflict }}</li>
          </ul>
        </TAlert>

        <TDescriptions :column="2" bordered size="small" class="mb-5">
          <TDescriptionsItem :label="t('ipam.addresses.ip')" :span="2">
            <TSpace size="small" align="center">
              <span>{{ view.address.ip_address }}</span>
              <TTag size="small" :theme="ipamStatusTheme(view.address.status)" variant="light">
                {{ ipamStatusLabel(t, view.address.status) }}
              </TTag>
            </TSpace>
          </TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.detail.space')">{{ view.space?.name || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.addresses.subnet')">
            {{ view.subnet ? `${view.subnet.name} (${view.subnet.cidr})` : '—' }}
          </TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.addresses.hostname')">{{ view.address.hostname || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.addresses.mac')">{{ view.address.mac_address || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.addresses.owner')">{{ view.address.owner || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.addresses.device')">{{ view.address.device || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.addresses.location')">{{ view.address.location || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('ipam.detail.lastSeen')">{{ view.address.last_seen || '—' }}</TDescriptionsItem>
          <TDescriptionsItem :label="t('common.description')" :span="2">
            {{ view.address.description || '—' }}
          </TDescriptionsItem>
        </TDescriptions>

        <template v-if="view.access.dns">
          <SectionTitle :text="t('ipam.detail.dnsRecords')" :count="view.dns_records.length" />
          <TEmpty v-if="view.dns_records.length === 0" size="small" class="section-empty" :description="t('ipam.detail.dnsRecordsEmpty')" />
          <TTable
            v-else
            size="small"
            :columns="dnsColumns"
            :data="view.dns_records"
            row-key="id"
            stripe
            hover
          />
        </template>

        <template v-if="view.access.dhcp">
          <SectionTitle :text="t('ipam.detail.scopes')" :count="view.dhcp_scopes.length" />
          <!-- A bounded list read as exhaustive would claim "does not cover". -->
          <TAlert v-if="view.scopes_truncated" theme="info" :message="t('ipam.detail.scopesTruncated')" class="mb-2" />
          <TEmpty
            v-if="view.dhcp_scopes.length === 0" size="small" class="section-empty"
            :description="view.scopes_truncated ? t('ipam.detail.scopesUnknown') : t('ipam.detail.scopesEmpty')"
          />
          <TTable v-else size="small" :columns="scopeColumns" :data="view.dhcp_scopes" row-key="id" stripe hover />

          <SectionTitle :text="t('ipam.detail.leases')" :count="view.dhcp_leases.length" />
          <TEmpty v-if="view.dhcp_leases.length === 0" size="small" class="section-empty" :description="t('ipam.detail.leasesEmpty')" />
          <TTable v-else size="small" :columns="leaseColumns" :data="view.dhcp_leases" row-key="id" stripe hover />

          <SectionTitle :text="t('ipam.detail.reservations')" :count="view.dhcp_reservations.length" />
          <TEmpty v-if="view.dhcp_reservations.length === 0" size="small" class="section-empty" :description="t('ipam.detail.reservationsEmpty')" />
          <TTable
            v-else
            size="small"
            :columns="reservationColumns"
            :data="view.dhcp_reservations"
            row-key="id"
            stripe
            hover
          />
        </template>

        <SectionTitle :text="t('ipam.detail.history')" :count="view.history.length" />
        <TEmpty v-if="view.history.length === 0" size="small" class="section-empty" :description="t('ipam.detail.historyEmpty')" />
        <TTimeline v-else>
          <TTimelineItem
            v-for="(entry, index) in view.history"
            :key="index"
            :label="entry.created_at"
          >
            <div class="history-title">{{ historyTitle(entry) }}</div>
            <div v-if="entry.changed_by" class="history-meta">{{ t('ipam.detail.changedBy') }}: {{ entry.changed_by }}</div>
            <div v-if="entry.reason" class="history-meta">{{ t('ipam.detail.reason') }}: {{ entry.reason }}</div>
            <div v-if="entry.source" class="history-meta">{{ t('ipam.detail.source') }}: {{ entry.source }}</div>
          </TTimelineItem>
        </TTimeline>
      </template>

      <TAlert v-else-if="loadFailed" theme="error" role="alert" :message="loadFailed">
        <template #operation>
          <TButton size="small" theme="primary" variant="outline" @click="load">{{ t('common.retry') }}</TButton>
        </template>
      </TAlert>
      <TEmpty v-else-if="!loading" :description="t('ipam.detail.nothingToShow')" />
    </TLoading>
  </TDrawer>
</template>

<style scoped>
.mb-4 {
  margin-bottom: 16px;
}
.mb-5 {
  margin-bottom: 20px;
}
.mb-2 {
  margin-bottom: 8px;
}
.section-empty {
  margin: 4px 0 18px;
}
.section-empty :deep(.t-empty__description) {
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}
.conflict-list {
  margin: 0;
  padding-left: 18px;
}
.history-title {
  font-weight: 500;
}
.history-meta {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
:deep(.tag-row) {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
</style>