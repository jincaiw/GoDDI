<template>
  <div>
    <page-header :title="zone?.name || ''" :subtitle="t('dns.zones.title')">
      <n-space>
        <n-button @click="router.push('/dns/zones')">{{ t('common.cancel') }}</n-button>
        <n-button v-if="perm.canWrite('dns')" type="primary" @click="openCreateRecord">{{ t('dns.records.createRecord') }}</n-button>
        <n-button v-if="perm.canWrite('dns')" @click="handleExport">{{ t('dns.zones.exportZone') }}</n-button>
        <n-button v-if="perm.canWrite('dns')" @click="handleSync" :disabled="zone?.type !== 'slave'">{{ t('dns.zones.syncZone') }}</n-button>
      </n-space>
    </page-header>

    <n-card>
      <n-tabs v-model:value="activeTab" type="line" @update:value="handleTabChange">
        <!-- Records -->
        <n-tab-pane name="records" :tab="t('dns.zones.tabRecords')" display-directive="show">
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-input v-model:value="recordSearch" :placeholder="t('common.search')" clearable style="width: 200px;" @keyup.enter="loadRecords">
              <template #prefix><n-icon><search-outline /></n-icon></template>
            </n-input>
            <n-select v-model:value="recordTypeFilter" :options="recordTypeOptions" clearable :placeholder="t('dns.records.recordType')" style="width: 130px;" @update:value="loadRecords" />
            <n-button @click="loadRecords">{{ t('common.refresh') }}</n-button>
          </n-space>

          <n-data-table
            :columns="recordColumns"
            :data="records"
            :loading="recordsLoading"
            remote :pagination="recordPagination"
            :row-key="(row: DNSRecord) => row.id"
            @update:page="handleRecordPageChange"
            @update:page-size="handleRecordPageSizeChange"
            :checked-row-keys="checkedKeys"
          />
        </n-tab-pane>

        <!-- Options -->
        <n-tab-pane name="options" :tab="t('dns.zones.tabOptions')" display-directive="show">
          <n-descriptions v-if="zone" :column="4" label-placement="left" style="margin-bottom: 16px;">
            <n-descriptions-item :label="t('dns.zones.zoneType')">{{ zone.type }}</n-descriptions-item>
            <n-descriptions-item :label="t('dns.zones.ttl')">{{ zone.default_ttl }}</n-descriptions-item>
            <n-descriptions-item :label="t('dns.zones.serial')">{{ zone.serial }}</n-descriptions-item>
            <n-descriptions-item :label="t('common.enabled')">
              <n-switch :value="zone.enabled" @update:value="toggleZoneEnabled" :disabled="!perm.canWrite('dns')" />
            </n-descriptions-item>
            <n-descriptions-item :label="t('dns.zones.primaryNs')">{{ zone.soa_mname || '-' }}</n-descriptions-item>
            <n-descriptions-item :label="t('dns.zones.adminEmail')">{{ zone.soa_rname || '-' }}</n-descriptions-item>
            <n-descriptions-item :label="t('dns.zones.refresh')">{{ zone.refresh }}</n-descriptions-item>
            <n-descriptions-item :label="t('dns.zones.minimum')">{{ zone.minimum }}</n-descriptions-item>
          </n-descriptions>

          <n-card :title="t('dns.zones.aclTitle')" v-if="zone && zone.type !== 'allowed' && zone.type !== 'blocked'" embedded>
            <n-form label-placement="left" label-width="180px">
              <n-form-item :label="t('dns.zones.queryAccess')">
                <n-select v-model:value="aclForm.query_access" :options="queryAccessOptions" :disabled="!perm.canWrite('dns')" style="width: 320px;" clearable />
              </n-form-item>
              <n-form-item :label="t('dns.zones.aclAllowQuery')">
                <n-dynamic-tags v-model:value="aclForm.allow_query" :disabled="!perm.canWrite('dns')" />
              </n-form-item>
              <n-form-item :label="t('dns.zones.aclAllowTransfer')">
                <n-dynamic-tags v-model:value="aclForm.allow_transfer" :disabled="!perm.canWrite('dns')" />
              </n-form-item>
              <n-form-item :label="t('dns.zones.aclAllowUpdate')">
                <n-dynamic-tags v-model:value="aclForm.allow_update" :disabled="!perm.canWrite('dns')" />
              </n-form-item>
              <n-form-item :label="t('dns.zones.aclNotify')">
                <n-dynamic-tags v-model:value="aclForm.notify" :disabled="!perm.canWrite('dns')" />
              </n-form-item>
            </n-form>
            <n-text depth="3" style="font-size: 12px;">{{ t('dns.zones.aclHint') }}</n-text>
            <n-divider />
            <n-space>
              <n-button v-if="perm.canWrite('dns')" type="primary" size="small" :loading="aclSaving" @click="saveACL">{{ t('common.save') }}</n-button>
            </n-space>
          </n-card>

          <!-- Catalog zone: member list (RFC 9432) -->
          <n-card v-if="zone && zone.type === 'catalog'" :title="t('dns.zones.catalogMembers')" embedded style="margin-top: 16px;">
            <n-space v-if="catalogMembers.length" size="small">
              <n-tag v-for="m in catalogMembers" :key="m" size="small" type="info">{{ m }}</n-tag>
            </n-space>
            <n-empty v-else :description="t('dns.zones.catalogMembersEmpty')" size="small" />
          </n-card>

          <!-- Non-catalog zone: join/leave a catalog -->
          <n-card v-if="zone && !['allowed', 'blocked', 'catalog'].includes(zone.type)" :title="t('dns.zones.catalogTitle')" embedded style="margin-top: 16px;">
            <n-form label-placement="left" label-width="180px">
              <n-form-item :label="t('dns.zones.catalogJoin')">
                <n-select v-model:value="catalogForm.catalog" :options="catalogOptions" :disabled="!perm.canWrite('dns')" style="width: 320px;" />
              </n-form-item>
            </n-form>
            <n-text depth="3" style="font-size: 12px;">{{ t('dns.zones.catalogHint') }}</n-text>
            <n-divider />
            <n-space>
              <n-button v-if="perm.canWrite('dns')" type="primary" size="small" :loading="catalogSaving" @click="saveCatalog">{{ t('common.save') }}</n-button>
            </n-space>
          </n-card>
        </n-tab-pane>

        <!-- Permissions -->
        <n-tab-pane name="permissions" :tab="t('dns.zones.tabPermissions')" display-directive="show">
          <n-alert type="info" size="small" style="margin-bottom: 12px;">{{ t('dns.zones.permHint') }}</n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button v-if="perm.canWrite('dns')" size="small" @click="addPermission">{{ t('dns.zones.permAdd') }}</n-button>
            <n-button v-if="perm.canWrite('dns')" type="primary" size="small" :loading="permSaving" @click="savePermissions">{{ t('common.save') }}</n-button>
          </n-space>
          <n-data-table :columns="permissionColumns" :data="permissions" :row-key="(row: ZonePermission) => row.principal_type + ':' + row.principal_id" />
        </n-tab-pane>

        <!-- History -->
        <n-tab-pane name="history" :tab="t('dns.zones.tabHistory')" display-directive="show">
          <n-data-table
            :columns="historyColumns"
            :data="history"
            :loading="historyLoading"
            remote :pagination="historyPagination"
            :row-key="(row: ZoneChangeEntry) => row.id"
            @update:page="handleHistoryPageChange"
          />
        </n-tab-pane>

        <!-- DNSSEC -->
        <n-tab-pane name="dnssec" :tab="t('dns.zones.tabDnssec')" display-directive="show">
          <n-alert type="warning" size="small" style="margin-bottom: 12px;">
            {{ t('dns.zones.dnssecExperimentalHint') }}
          </n-alert>
          <n-space style="margin-bottom: 16px;">
            <n-button v-if="!zone?.dnssec_enabled && perm.canWrite('dns')" type="warning" size="small" @click="handleDnssecAction('enable')">{{ t('dns.zones.enableDnssec') }}</n-button>
            <n-button v-if="zone?.dnssec_enabled && perm.canWrite('dns')" type="warning" size="small" @click="handleDnssecAction('disable')">{{ t('dns.zones.disableDnssec') }}</n-button>
            <n-button v-if="zone?.dnssec_enabled && perm.canWrite('dns')" size="small" @click="handleDnssecAction('rotate')">{{ t('dns.zones.rotateKeys') }}</n-button>
            <n-button v-if="perm.canWrite('dns')" size="small" type="primary" ghost @click="openGenerateKey">{{ t('dns.zones.dnssecGenerateKey') }}</n-button>
            <n-button v-if="perm.canWrite('dns')" size="small" @click="handlePromoteStandby">{{ t('dns.zones.dnssecPromote') }}</n-button>
            <n-button size="small" @click="loadDsRecords">{{ t('dns.zones.dnssecShowDs') }}</n-button>
          </n-space>
          <n-data-table
            v-if="dnssecStatus?.keys?.length"
            :columns="dnssecKeyColumns"
            :data="dnssecStatus.keys"
            :row-key="(row: DNSSECKey) => row.id"
          />
          <n-card v-if="dsRecords.length" :title="t('dns.zones.dnssecDsTitle')" embedded style="margin-top: 16px;">
            <n-data-table :columns="dsColumns" :data="dsRecords" :row-key="(row: DSInfo) => String(row.key_tag)" />
          </n-card>
          <n-card :title="t('dns.zones.dnssecNsec3Title')" embedded style="margin-top: 16px;">
            <n-form label-placement="left" label-width="160px" inline>
              <n-form-item :label="t('dns.zones.dnssecNsec3Iterations')">
                <n-input-number v-model:value="nsec3Form.iterations" :min="0" :max="100" :disabled="!perm.canWrite('dns')" />
              </n-form-item>
              <n-form-item :label="t('dns.zones.dnssecNsec3Salt')">
                <n-input v-model:value="nsec3Form.salt" placeholder="hex (可留空)" :disabled="!perm.canWrite('dns')" style="width: 220px;" />
              </n-form-item>
              <n-form-item :label="t('dns.zones.dnssecNsec3Optout')">
                <n-switch v-model:value="nsec3Form.optout" :disabled="!perm.canWrite('dns')" />
              </n-form-item>
            </n-form>
            <n-text depth="3" style="font-size: 12px;">{{ t('dns.zones.dnssecNsec3Hint') }}</n-text>
            <n-divider />
            <n-button v-if="perm.canWrite('dns')" type="primary" size="small" :loading="nsec3Saving" @click="saveNsec3">{{ t('common.save') }}</n-button>
          </n-card>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- Add/Edit Record Modal -->
    <n-modal v-if="showAddRecord" v-model:show="showAddRecord" preset="card" :title="editingRecord ? t('dns.records.editRecord') : t('dns.records.createRecord')" style="width: 500px;">
      <n-form :model="recordForm" label-placement="left" label-width="100px">
        <n-form-item :label="t('dns.records.recordName')">
          <n-input v-model:value="recordForm.name" />
        </n-form-item>
        <n-form-item :label="t('dns.records.recordType')">
          <n-select v-model:value="recordForm.type" :options="recordTypeOptions" :disabled="!!editingRecord" />
        </n-form-item>
        <n-form-item :label="t('dns.records.recordValue')">
          <n-input v-model:value="recordForm.value" type="textarea" :rows="3" />
        </n-form-item>
        <n-form-item :label="t('dns.zones.ttl')">
          <n-input-number v-model:value="recordForm.ttl" :min="0" />
        </n-form-item>
        <n-form-item :label="t('common.priority')">
          <n-input-number v-model:value="recordForm.priority" :min="0" :max="65535" />
        </n-form-item>
        <n-form-item label="expiry_ttl">
          <n-input-number v-model:value="recordForm.expiry_ttl" :min="0" clearable :placeholder="'seconds / 0 = never'" />
        </n-form-item>
        <n-form-item :label="t('common.enabled')">
          <n-switch v-model:value="recordForm.enabled" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAddRecord = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="recordSubmitting" @click="handleRecordSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Generate DNSSEC Key Modal -->
    <n-modal v-if="showGenerateKey" v-model:show="showGenerateKey" preset="card" :title="t('dns.zones.dnssecGenerateKey')" style="width: 420px;">
      <n-form label-placement="left" label-width="100px">
        <n-form-item label="Key Type">
          <n-select v-model:value="generateKeyType" :options="[{ label: 'KSK', value: 'KSK' }, { label: 'ZSK', value: 'ZSK' }]" />
        </n-form-item>
        <n-form-item :label="t('dns.records.recordType') + ' / ' + 'Algorithm'">
          <n-select v-model:value="generateKeyAlgorithm" :options="dnssecAlgorithmOptions" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showGenerateKey = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleGenerateKey">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog
      :show="showDeleteRecordConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDeleteRecord"
      @cancel="showDeleteRecordConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, h, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import {
  getDNSZone, updateDNSZone, exportZoneFile, syncSecondaryZone,
  enableDNSSEC, disableDNSSEC, rotateDNSSECKeys, getDNSSECStatus,
  listDNSRecords, createDNSRecord, updateDNSRecord, deleteDNSRecord,
  getZoneHistory, getZonePermissions, setZonePermissions, getCatalogMembers, listDNSZones,
  getZoneDSRecords, generateDNSSECKey, deleteDNSSECKey, toggleDNSSECKey, promoteDNSSECStandbyKeys,
  getNSEC3Params, setNSEC3Params,
  type DNSZone, type DNSRecord, type CreateDNSRecordRequest, type ZoneACL,
  type ZonePermission, type ZoneChangeEntry, type DNSSECKey, type DNSSECStatus, type DSInfo,
} from '@/service/api/goddi/dns'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const zoneId = route.params.id as string
const zone = ref<DNSZone | null>(null)
const activeTab = ref('records')
const records = ref<DNSRecord[]>([])
const recordsLoading = ref(false)
const recordSubmitting = ref(false)
const recordSearch = ref('')
const recordTypeFilter = ref<string | null>(null)
const showAddRecord = ref(false)
const showDeleteRecordConfirm = ref(false)
const deletingRecordId = ref('')
const editingRecord = ref<DNSRecord | null>(null)

const recordTypeOptions = [
  'A', 'AAAA', 'CNAME', 'DNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT', 'CAA',
  'TLSA', 'SVCB', 'HTTPS', 'URI', 'SSHFP', 'NAPTR', 'DS', 'DNSKEY', 'HINFO', 'LOC', 'RP', 'SPF', 'APL',
].map(t => ({ label: t, value: t }))

const checkedKeys = ref<string[]>([])

const recordPagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
})

const recordForm = reactive<CreateDNSRecordRequest & { enabled: boolean; expiry_ttl?: number }>({
  name: '',
  type: 'A',
  value: '',
  ttl: 3600,
  priority: undefined,
  expiry_ttl: undefined,
  enabled: true,
})

const recordColumns = [
  { title: () => t('dns.records.recordName'), key: 'name', ellipsis: { tooltip: true } },
  { title: () => t('dns.records.recordType'), key: 'type', width: 80, render: (row: DNSRecord) => h(NTag, { size: 'small' }, { default: () => row.type }) },
  { title: () => t('dns.records.recordValue'), key: 'value', ellipsis: { tooltip: true } },
  { title: () => t('dns.zones.ttl'), key: 'ttl', width: 80 },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: DNSRecord) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dns'), onUpdateValue: () => toggleRecordEnabled(row) }) },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: DNSRecord) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, onClick: () => editRecord(row) }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => { deletingRecordId.value = row.id; showDeleteRecordConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadZone() {
  try {
    zone.value = await getDNSZone(zoneId)
    const acl = zone.value.acl
    aclForm.query_access = acl?.query_access ?? ''
    aclForm.allow_query = [...(acl?.allow_query ?? [])]
    aclForm.allow_transfer = [...(acl?.allow_transfer ?? [])]
    aclForm.allow_update = [...(acl?.allow_update ?? [])]
    aclForm.notify = [...(acl?.notify ?? [])]
    await loadCatalogData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

// --- Catalog membership (RFC 9432 / Technitium parity) ---

const catalogMembers = ref<string[]>([])
const catalogZones = ref<DNSZone[]>([])
const catalogSaving = ref(false)
// Backend semantics for the catalog field: "-" | "none" leaves the current
// catalog, a zone name joins it, absent/"" keeps it unchanged.
const catalogForm = reactive<{ catalog: string }>({ catalog: '-' })

const catalogOptions = computed(() => [
  { label: t('dns.zones.catalogNone'), value: '-' },
  ...catalogZones.value.map(z => ({ label: z.name, value: z.name })),
])

async function loadCatalogData() {
  if (!zone.value) return
  catalogForm.catalog = zone.value.catalog || '-'
  if (zone.value.type === 'catalog') {
    try {
      catalogMembers.value = (await getCatalogMembers(zoneId)) ?? []
    } catch {
      catalogMembers.value = []
    }
    return
  }
  try {
    const result = await listDNSZones({ page: 1, page_size: 100, type: 'catalog' })
    catalogZones.value = result.data
  } catch {
    catalogZones.value = []
  }
}

async function saveCatalog() {
  catalogSaving.value = true
  try {
    zone.value = await updateDNSZone(zoneId, { catalog: catalogForm.catalog })
    catalogForm.catalog = zone.value.catalog || '-'
    message.success(t('common.success'))
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    catalogSaving.value = false
  }
}

// Zone ACL editor state. Empty lists mean unrestricted.
const aclForm = reactive<ZoneACL & { query_access?: string }>({
  query_access: '',
  allow_query: [],
  allow_transfer: [],
  allow_update: [],
  notify: [],
})
const aclSaving = ref(false)

const queryAccessOptions = [
  { label: t('dns.zones.queryAccessDefault'), value: '' },
  { label: t('dns.zones.queryAccessAllow'), value: 'allow' },
  { label: t('dns.zones.queryAccessDeny'), value: 'deny' },
  { label: t('dns.zones.queryAccessPrivate'), value: 'allow_only_private_networks' },
]

async function saveACL() {
  aclSaving.value = true
  try {
    // Send the ACL unconditionally: an all-empty list clears restrictions
    // server-side because the store treats empty lists as "allow all".
    const acl: ZoneACL = {
      query_access: aclForm.query_access || undefined,
      allow_query: aclForm.allow_query,
      allow_transfer: aclForm.allow_transfer,
      allow_update: aclForm.allow_update,
      notify: aclForm.notify,
    }
    await updateDNSZone(zoneId, { acl })
    message.success(t('common.updateSuccess'))
    loadZone()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    aclSaving.value = false
  }
}

async function loadRecords() {
  recordsLoading.value = true
  try {
    const params: Record<string, unknown> = { page: recordPagination.page, page_size: recordPagination.pageSize }
    if (recordSearch.value) params.name = recordSearch.value
    if (recordTypeFilter.value) params.type = recordTypeFilter.value
    const result = await listDNSRecords(zoneId, params)
    records.value = result.data
    recordPagination.itemCount = result.meta.total
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    recordsLoading.value = false
  }
}

function handleRecordPageChange(page: number) {
  recordPagination.page = page
  loadRecords()
}

function handleRecordPageSizeChange(pageSize: number) {
  recordPagination.pageSize = pageSize
  recordPagination.page = 1
  loadRecords()
}

async function toggleZoneEnabled() {
  if (!zone.value) return
  try {
    await updateDNSZone(zoneId, { enabled: !zone.value.enabled })
    message.success(t('common.updateSuccess'))
    loadZone()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function toggleRecordEnabled(record: DNSRecord) {
  try {
    await updateDNSRecord(zoneId, record.id, { enabled: !record.enabled })
    message.success(t('common.updateSuccess'))
    loadRecords()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

function editRecord(record: DNSRecord) {
  editingRecord.value = record
  recordForm.name = record.name
  recordForm.type = record.type
  recordForm.value = record.value
  recordForm.ttl = record.ttl
  recordForm.priority = record.priority
  recordForm.enabled = record.enabled
  recordForm.expiry_ttl = undefined
  showAddRecord.value = true
}

function openCreateRecord() {
  editingRecord.value = null
  resetRecordForm()
  showAddRecord.value = true
}

async function handleRecordSubmit() {
  recordSubmitting.value = true
  try {
    if (editingRecord.value) {
      await updateDNSRecord(zoneId, editingRecord.value.id, recordForm)
      message.success(t('common.updateSuccess'))
    } else {
      await createDNSRecord(zoneId, recordForm)
      message.success(t('common.createSuccess'))
    }
    showAddRecord.value = false
    editingRecord.value = null
    resetRecordForm()
    loadRecords()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    recordSubmitting.value = false
  }
}

async function handleDeleteRecord() {
  try {
    await deleteDNSRecord(zoneId, deletingRecordId.value)
    message.success(t('common.deleteSuccess'))
    loadRecords()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
  showDeleteRecordConfirm.value = false
}

async function handleExport() {
  try {
    const response = await exportZoneFile(zoneId)
    const url = window.URL.createObjectURL(new Blob([response.data as BlobPart]))
    const link = document.createElement('a')
    link.href = url
    link.download = `${zone.value?.name || 'zone'}.txt`
    link.click()
    window.URL.revokeObjectURL(url)
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleSync() {
  try {
    await syncSecondaryZone(zoneId)
    message.success(t('common.success'))
    loadZone()
    loadRecords()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleDnssecAction(action: string) {
  try {
    if (action === 'enable') await enableDNSSEC(zoneId)
    else if (action === 'disable') await disableDNSSEC(zoneId)
    else if (action === 'rotate') await rotateDNSSECKeys(zoneId)
    message.success(t('common.success'))
    loadZone()
    dnssecLoaded.value = false
    loadDnssec()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

function resetRecordForm() {
  recordForm.name = ''
  recordForm.type = 'A'
  recordForm.value = ''
  recordForm.ttl = 3600
  recordForm.priority = undefined
  recordForm.expiry_ttl = undefined
  recordForm.enabled = true
}

// --- Per-zone permissions tab ---

const permissions = ref<ZonePermission[]>([])
const permSaving = ref(false)
const permLoaded = ref(false)

const permissionColumns = [
  { title: () => t('dns.zones.permPrincipalType'), key: 'principal_type', width: 120, render: (row: ZonePermission) => h(NTag, { size: 'small', type: row.principal_type === 'user' ? 'info' : 'warning' }, { default: () => (row.principal_type === 'user' ? t('dns.zones.permPrincipalUser') : t('dns.zones.permPrincipalGroup')) }) },
  { title: () => t('dns.zones.permPrincipalId'), key: 'principal_id' },
  { title: () => t('dns.zones.permView'), key: 'can_view', width: 80, render: (row: ZonePermission) => h(NSwitch, { value: row.can_view, disabled: !perm.canWrite('dns'), onUpdateValue: (v: boolean) => { row.can_view = v } }) },
  { title: () => t('dns.zones.permModify'), key: 'can_modify', width: 80, render: (row: ZonePermission) => h(NSwitch, { value: row.can_modify, disabled: !perm.canWrite('dns'), onUpdateValue: (v: boolean) => { row.can_modify = v } }) },
  { title: () => t('dns.zones.permDelete'), key: 'can_delete', width: 80, render: (row: ZonePermission) => h(NSwitch, { value: row.can_delete, disabled: !perm.canWrite('dns'), onUpdateValue: (v: boolean) => { row.can_delete = v } }) },
  { title: () => t('common.actions'), key: 'actions', width: 80, render: (row: ZonePermission) => h(
    NButton,
    { size: 'small', text: true, type: 'error', disabled: !perm.canWrite('dns'), onClick: () => removePermission(row) },
    { default: () => t('common.delete') },
  ) },
]

async function loadPermissions() {
  try {
    const list = await getZonePermissions(zoneId)
    permissions.value = list ?? []
    permLoaded.value = true
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

function addPermission() {
  permissions.value.push({ principal_type: 'user', principal_id: '', can_view: true, can_modify: false, can_delete: false })
}

function removePermission(row: ZonePermission) {
  permissions.value = permissions.value.filter(p => !(p.principal_type === row.principal_type && p.principal_id === row.principal_id))
}

async function savePermissions() {
  const invalid = permissions.value.find(p => !p.principal_id.trim())
  if (invalid) {
    message.warning(t('dns.zones.permPrincipalId'))
    return
  }
  permSaving.value = true
  try {
    await setZonePermissions(zoneId, permissions.value)
    message.success(t('common.updateSuccess'))
    loadPermissions()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    permSaving.value = false
  }
}

// --- Zone history tab ---

const history = ref<ZoneChangeEntry[]>([])
const historyLoading = ref(false)
const historyLoaded = ref(false)
const historyPagination = reactive({
  page: 1,
  pageSize: 50,
  itemCount: 0,
})

const historyColumns = [
  { title: 'Serial', key: 'serial', width: 120 },
  { title: 'Action', key: 'change_type', width: 90, render: (row: ZoneChangeEntry) => h(NTag, { size: 'small', type: row.change_type === 'add' ? 'success' : 'error' }, { default: () => row.change_type }) },
  { title: () => t('dns.records.recordName'), key: 'name', ellipsis: { tooltip: true } },
  { title: () => t('dns.records.recordType'), key: 'type', width: 80 },
  { title: () => t('dns.records.recordValue'), key: 'value', ellipsis: { tooltip: true } },
  { title: () => t('dns.zones.ttl'), key: 'ttl', width: 70 },
  { title: 'Time', key: 'created_at', width: 170 },
]

async function loadHistory() {
  historyLoading.value = true
  try {
    const result = await getZoneHistory(zoneId, { page: historyPagination.page, page_size: historyPagination.pageSize })
    history.value = result.data ?? []
    historyPagination.itemCount = result.meta.total
    historyLoaded.value = true
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    historyLoading.value = false
  }
}

function handleHistoryPageChange(page: number) {
  historyPagination.page = page
  loadHistory()
}

// --- DNSSEC tab ---

const dnssecStatus = ref<DNSSECStatus | null>(null)
const dnssecLoaded = ref(false)

const dnssecKeyColumns = [
  { title: 'Key Tag', key: 'key_tag', width: 100 },
  { title: 'ID', key: 'id', width: 140, ellipsis: { tooltip: true } },
  { title: 'Algorithm', key: 'algorithm', width: 140 },
  { title: 'Type', key: 'key_type', width: 90, render: (row: DNSSECKey) => h(NTag, { size: 'small', type: row.key_type === 'KSK' ? 'warning' : 'info' }, { default: () => row.key_type }) },
  { title: 'Enabled', key: 'enabled', width: 90, render: (row: DNSSECKey) => h(NSwitch, { value: row.enabled ?? true, size: 'small', disabled: !perm.canWrite('dns'), onUpdateValue: (v: boolean) => handleToggleKey(row, v) }) },
  { title: 'Created', key: 'created_at', width: 170 },
  { title: 'Actions', key: 'actions', width: 90, render: (row: DNSSECKey) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteKey(row) }, { default: () => t('common.delete') }) },
]

// --- DNSSEC key lifecycle / DS / NSEC3 (Technitium parity) ---

const dsRecords = ref<DSInfo[]>([])

const dsColumns = [
  { title: 'Key Tag', key: 'key_tag', width: 100 },
  { title: 'Algorithm', key: 'algorithm', width: 150 },
  { title: 'Digest Type', key: 'digest_type', width: 110 },
  { title: 'Digest', key: 'digest', ellipsis: { tooltip: true } },
]

const nsec3Form = reactive<{ iterations: number; salt: string; optout: boolean }>({ iterations: 0, salt: '', optout: false })
const nsec3Saving = ref(false)
const showGenerateKey = ref(false)
const generateKeyType = ref<'KSK' | 'ZSK'>('KSK')
const generateKeyAlgorithm = ref('ECDSAP256SHA256')

const dnssecAlgorithmOptions = [
  'ECDSAP256SHA256', 'ECDSAP384SHA384', 'ED25519', 'RSASHA256', 'RSASHA512',
].map(a => ({ label: a, value: a }))

async function loadDsRecords() {
  try {
    dsRecords.value = (await getZoneDSRecords(zoneId)) ?? []
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleToggleKey(key: DNSSECKey, enabled: boolean) {
  try {
    await toggleDNSSECKey(zoneId, key.id, enabled)
    key.enabled = enabled
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleDeleteKey(key: DNSSECKey) {
  try {
    await deleteDNSSECKey(zoneId, key.id)
    message.success(t('common.success'))
    dnssecLoaded.value = false
    loadDnssec()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

function openGenerateKey() {
  generateKeyType.value = 'KSK'
  generateKeyAlgorithm.value = 'ECDSAP256SHA256'
  showGenerateKey.value = true
}

async function handleGenerateKey() {
  try {
    await generateDNSSECKey(zoneId, generateKeyType.value, generateKeyAlgorithm.value)
    showGenerateKey.value = false
    message.success(t('common.success'))
    dnssecLoaded.value = false
    loadDnssec()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handlePromoteStandby() {
  try {
    await promoteDNSSECStandbyKeys(zoneId)
    message.success(t('common.success'))
    dnssecLoaded.value = false
    loadDnssec()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function loadNsec3() {
  try {
    const p = await getNSEC3Params(zoneId)
    if (p) {
      nsec3Form.iterations = p.iterations
      nsec3Form.salt = p.salt
      nsec3Form.optout = p.optout
    }
  } catch {
    // NSEC3 params are optional; absence should not break the tab.
  }
}

async function saveNsec3() {
  nsec3Saving.value = true
  try {
    await setNSEC3Params(zoneId, { ...nsec3Form })
    message.success(t('common.success'))
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    nsec3Saving.value = false
  }
}

async function loadDnssec() {
  if (dnssecLoaded.value) return
  try {
    dnssecStatus.value = await getDNSSECStatus(zoneId)
    dnssecLoaded.value = true
  } catch {
    // DNSSEC is experimental; absence of the endpoint should not break the tab.
    dnssecLoaded.value = true
  }
  loadNsec3()
}

function handleTabChange(tab: string) {
  if (tab === 'permissions' && !permLoaded.value) loadPermissions()
  else if (tab === 'history' && !historyLoaded.value) loadHistory()
  else if (tab === 'dnssec' && !dnssecLoaded.value) loadDnssec()
}

onMounted(() => {
  loadZone()
  loadRecords()
})
</script>
