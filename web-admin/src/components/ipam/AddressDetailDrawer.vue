<template>
  <n-drawer :show="show" :width="700" placement="right" @update:show="emit('update:show', $event)">
    <n-drawer-content :title="title" closable :native-scrollbar="false">
      <n-spin :show="loading">
        <template v-if="view">
          <!-- The four subsystems can disagree about one address. Saying so is
               the reason this view exists; a conflict the operator has to
               discover by cross-reading three pages would not be reported. -->
          <n-alert v-if="view.conflicts.length > 0" type="warning" :title="t('ipam.detail.conflicts')" style="margin-bottom: 16px;">
            <ul style="margin: 0; padding-left: 18px;">
              <li v-for="(conflict, index) in view.conflicts" :key="index">{{ conflict }}</li>
            </ul>
          </n-alert>

          <n-descriptions :column="2" bordered size="small" label-placement="left" style="margin-bottom: 20px;">
            <n-descriptions-item :label="t('ipam.addresses.ip')" :span="2">
              <n-space align="center" :size="8">
                <span>{{ view.address.ip_address }}</span>
                <n-tag size="small" :type="statusTag(view.address.status)">{{ view.address.status }}</n-tag>
              </n-space>
            </n-descriptions-item>
            <n-descriptions-item :label="t('ipam.detail.space')">{{ view.space?.name || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.addresses.subnet')">
              {{ view.subnet ? `${view.subnet.name} (${view.subnet.cidr})` : '—' }}
            </n-descriptions-item>
            <n-descriptions-item :label="t('ipam.addresses.hostname')">{{ view.address.hostname || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.addresses.mac')">{{ view.address.mac_address || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.addresses.owner')">{{ view.address.owner || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.addresses.device')">{{ view.address.device || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.addresses.location')">{{ view.address.location || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.detail.lastSeen')">{{ view.address.last_seen || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('common.description')" :span="2">{{ view.address.description || '—' }}</n-descriptions-item>
          </n-descriptions>

          <section-title :text="t('ipam.detail.dnsRecords')" :count="view.dns_records.length" />
          <p v-if="view.dns_records.length === 0" class="empty">{{ t('ipam.detail.dnsRecordsEmpty') }}</p>
          <n-data-table
            v-else
            size="small"
            :bordered="false"
            :single-line="false"
            :row-key="(row: PublishingRecord) => row.id"
            :columns="dnsColumns"
            :data="view.dns_records"
          />

          <section-title :text="t('ipam.detail.scopes')" :count="view.dhcp_scopes.length" />
          <!-- A bounded list read as an exhaustive one turns "was not listed"
               into "does not cover". The API says which it is. -->
          <n-alert v-if="view.scopes_truncated" type="info" :show-icon="false" style="margin-bottom: 8px;">
            {{ t('ipam.detail.scopesTruncated') }}
          </n-alert>
          <p v-if="view.dhcp_scopes.length === 0" class="empty">
            {{ view.scopes_truncated ? t('ipam.detail.scopesUnknown') : t('ipam.detail.scopesEmpty') }}
          </p>
          <n-data-table
            v-else
            size="small"
            :bordered="false"
            :single-line="false"
            :row-key="(row: ScopeSummary) => row.id"
            :columns="scopeColumns"
            :data="view.dhcp_scopes"
          />

          <section-title :text="t('ipam.detail.leases')" :count="view.dhcp_leases.length" />
          <p v-if="view.dhcp_leases.length === 0" class="empty">{{ t('ipam.detail.leasesEmpty') }}</p>
          <n-data-table
            v-else
            size="small"
            :bordered="false"
            :single-line="false"
            :row-key="(row: LeaseSummary) => row.id"
            :columns="leaseColumns"
            :data="view.dhcp_leases"
          />

          <section-title :text="t('ipam.detail.reservations')" :count="view.dhcp_reservations.length" />
          <p v-if="view.dhcp_reservations.length === 0" class="empty">{{ t('ipam.detail.reservationsEmpty') }}</p>
          <n-data-table
            v-else
            size="small"
            :bordered="false"
            :single-line="false"
            :row-key="(row: ReservationSummary) => row.id"
            :columns="reservationColumns"
            :data="view.dhcp_reservations"
          />

          <section-title :text="t('ipam.detail.history')" :count="view.history.length" />
          <p v-if="view.history.length === 0" class="empty">{{ t('ipam.detail.historyEmpty') }}</p>
          <n-timeline v-else size="medium" style="padding-left: 4px;">
            <n-timeline-item
              v-for="(entry, index) in view.history"
              :key="index"
              :time="entry.created_at"
              :title="historyTitle(entry)"
            >
              <span v-if="entry.changed_by">{{ t('ipam.detail.changedBy') }}: {{ entry.changed_by }}</span>
              <span v-if="entry.reason">{{ t('ipam.detail.reason') }}: {{ entry.reason }}</span>
              <span v-if="entry.source">{{ t('ipam.detail.source') }}: {{ entry.source }}</span>
            </n-timeline-item>
          </n-timeline>
        </template>

        <n-empty v-else-if="!loading" :description="loadFailed || t('ipam.detail.nothingToShow')" />
      </n-spin>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTag, useMessage } from 'naive-ui'
import SectionTitle from '@/components/ipam/SectionTitle.vue'
import {
  getIPAMAddressView,
  type AddressHistoryEntry,
  type AddressView,
  type LeaseSummary,
  type PublishingRecord,
  type ReservationSummary,
  type ScopeSummary,
} from '@/service/api/goddi/ipam'

const props = defineProps<{
  show: boolean
  spaceId: string
  ip: string
}>()

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
}>()

const { t } = useI18n()
const message = useMessage()

const loading = ref(false)
const view = ref<AddressView | null>(null)
const loadFailed = ref('')

const title = computed(() => (props.ip ? `${t('ipam.detail.title')} · ${props.ip}` : t('ipam.detail.title')))

function statusTag(status: string) {
  const map: Record<string, 'success' | 'default' | 'warning' | 'error'> = {
    used: 'success',
    available: 'default',
    reserved: 'warning',
    conflict: 'error',
  }
  return map[status] || 'default'
}

function historyTitle(entry: AddressHistoryEntry) {
  if (entry.old_status && entry.new_status) return `${entry.action}: ${entry.old_status} → ${entry.new_status}`
  return entry.new_status ? `${entry.action}: ${entry.new_status}` : entry.action
}

const dnsColumns = [
  { title: () => t('ipam.detail.recordName'), key: 'name', ellipsis: { tooltip: true } },
  { title: () => t('ipam.detail.recordType'), key: 'type', width: 70 },
  { title: () => t('ipam.detail.recordValue'), key: 'value', ellipsis: { tooltip: true } },
  { title: () => t('ipam.detail.ttl'), key: 'ttl', width: 70 },
  {
    title: () => t('common.status'),
    key: 'enabled',
    width: 90,
    render: (row: PublishingRecord) =>
      h(
        NTag,
        { size: 'small', type: row.enabled ? 'success' : 'default', bordered: false },
        { default: () => (row.enabled ? t('common.enabled') : t('common.disabled')) },
      ),
  },
  {
    // Whether a name can be trusted to still exist: a data-plane write reaches
    // the control database by a push that lags.
    title: () => t('ipam.detail.author'),
    key: 'authored_locally',
    width: 110,
    render: (row: PublishingRecord) =>
      h(
        NTag,
        { size: 'small', type: row.authored_locally ? 'warning' : 'default', bordered: false },
        { default: () => (row.authored_locally ? t('ipam.detail.authorLocal') : t('ipam.detail.authorOperator')) },
      ),
  },
]

const scopeColumns = [
  { title: () => t('common.name'), key: 'name', ellipsis: { tooltip: true } },
  {
    title: () => t('ipam.detail.range'),
    key: 'range',
    render: (row: ScopeSummary) => `${row.start_ip} – ${row.end_ip}`,
  },
  { title: () => t('ipam.detail.router'), key: 'router', width: 130, render: (row: ScopeSummary) => row.router || '—' },
  {
    title: () => t('ipam.detail.relation'),
    key: 'relation',
    width: 190,
    render: (row: ScopeSummary) =>
      h(
        'div',
        { style: 'display:flex;gap:4px;flex-wrap:wrap' },
        [
          row.in_subnet ? h(NTag, { size: 'small', bordered: false }, { default: () => t('ipam.detail.inSubnet') }) : null,
          row.in_pool
            ? h(NTag, { size: 'small', type: 'warning', bordered: false }, { default: () => t('ipam.detail.inPool') })
            : null,
          row.is_gateway
            ? h(NTag, { size: 'small', type: 'info', bordered: false }, { default: () => t('ipam.detail.isGateway') })
            : null,
          !row.enabled
            ? h(NTag, { size: 'small', type: 'default', bordered: false }, { default: () => t('common.disabled') })
            : null,
        ].filter(Boolean),
      ),
  },
]

const leaseColumns = [
  { title: () => t('ipam.addresses.mac'), key: 'mac_address', width: 140 },
  { title: () => t('ipam.addresses.hostname'), key: 'hostname', render: (row: LeaseSummary) => row.hostname || '—' },
  {
    title: () => t('common.status'),
    key: 'status',
    width: 110,
    render: (row: LeaseSummary) => h(NTag, { size: 'small', bordered: false }, { default: () => row.status }),
  },
  { title: () => t('ipam.detail.leaseEnd'), key: 'lease_end', width: 180 },
]

const reservationColumns = [
  { title: () => t('ipam.addresses.mac'), key: 'mac_address', width: 140 },
  { title: () => t('ipam.addresses.hostname'), key: 'hostname', render: (row: ReservationSummary) => row.hostname || '—' },
  {
    title: () => t('common.status'),
    key: 'enabled',
    width: 100,
    render: (row: ReservationSummary) =>
      h(
        NTag,
        { size: 'small', type: row.enabled ? 'success' : 'default', bordered: false },
        { default: () => (row.enabled ? t('common.enabled') : t('common.disabled')) },
      ),
  },
]

async function load() {
  if (!props.spaceId || !props.ip) return
  loading.value = true
  loadFailed.value = ''
  try {
    view.value = await getIPAMAddressView(props.spaceId, props.ip)
  } catch (err: unknown) {
    // Keep the failure in the body: a drawer that opens blank looks like an
    // address with nothing attached to it, which is a different answer.
    view.value = null
    loadFailed.value = err instanceof Error ? err.message : t('common.failed')
    message.error(loadFailed.value)
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.show, props.spaceId, props.ip] as const,
  ([show]) => {
    if (show) {
      view.value = null
      load()
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.empty {
  margin: 0 0 18px;
  color: var(--text-color-3, #999);
  font-size: 13px;
}
</style>
