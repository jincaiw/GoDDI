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

    <!-- Zone Info -->
    <n-card style="margin-bottom: 16px;" v-if="zone">
      <n-descriptions :column="4" label-placement="left">
        <n-descriptions-item :label="t('dns.zones.zoneType')">{{ zone.type }}</n-descriptions-item>
        <n-descriptions-item :label="t('dns.zones.ttl')">{{ zone.default_ttl }}</n-descriptions-item>
        <n-descriptions-item :label="t('dns.zones.serial')">{{ zone.serial }}</n-descriptions-item>
        <n-descriptions-item :label="t('dns.zones.dnssec')">
          <n-space :size="6" align="center">
            <n-tag :type="zone.dnssec_enabled ? 'warning' : 'default'" size="small">{{ zone.dnssec_enabled ? 'ON' : 'OFF' }}</n-tag>
            <n-tag size="small" type="warning">{{ t('common.experimental') }}</n-tag>
          </n-space>
        </n-descriptions-item>
        <n-descriptions-item :label="t('dns.zones.primaryNs')">{{ zone.soa_mname || '-' }}</n-descriptions-item>
        <n-descriptions-item :label="t('dns.zones.adminEmail')">{{ zone.soa_rname || '-' }}</n-descriptions-item>
        <n-descriptions-item :label="t('common.enabled')">
          <n-switch :value="zone.enabled" @update:value="toggleZoneEnabled" :disabled="!perm.canWrite('dns')" />
        </n-descriptions-item>
      </n-descriptions>

      <!-- DNSSEC Controls -->
      <n-divider />
      <n-alert type="warning" size="small" style="margin-bottom: 12px;">
        {{ t('dns.zones.dnssecExperimentalHint') }}
      </n-alert>
      <n-space>
        <n-button v-if="!zone.dnssec_enabled && perm.canWrite('dns')" type="warning" size="small" @click="handleDnssecAction('enable')">{{ t('dns.zones.enableDnssec') }}</n-button>
        <n-button v-if="zone.dnssec_enabled && perm.canWrite('dns')" type="warning" size="small" @click="handleDnssecAction('disable')">{{ t('dns.zones.disableDnssec') }}</n-button>
        <n-button v-if="zone.dnssec_enabled && perm.canWrite('dns')" size="small" @click="handleDnssecAction('rotate')">{{ t('dns.zones.rotateKeys') }}</n-button>
      </n-space>
    </n-card>

    <!-- Access Control (ACL) -->
    <n-card style="margin-bottom: 16px;" :title="t('dns.zones.aclTitle')" v-if="zone && zone.type !== 'allowed' && zone.type !== 'blocked'">
      <n-form label-placement="left" label-width="160px">
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

    <!-- Records Table -->
    <n-card :title="t('dns.records.title')">
      <template #header-extra>
        <n-space>
          <n-input v-model:value="recordSearch" :placeholder="t('common.search')" clearable style="width: 200px;" @keyup.enter="loadRecords">
            <template #prefix><n-icon><search-outline /></n-icon></template>
          </n-input>
          <n-select v-model:value="recordTypeFilter" :options="recordTypeOptions" clearable :placeholder="t('dns.records.recordType')" style="width: 120px;" @update:value="loadRecords" />
          <n-button @click="loadRecords">{{ t('common.refresh') }}</n-button>
        </n-space>
      </template>

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

    <confirm-dialog
      :show="showDeleteRecordConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDeleteRecord"
      @cancel="showDeleteRecordConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import {
  getDNSZone, updateDNSZone, exportZoneFile, syncSecondaryZone,
  enableDNSSEC, disableDNSSEC, rotateDNSSECKeys,
  listDNSRecords, createDNSRecord, updateDNSRecord, deleteDNSRecord,
  type DNSZone, type DNSRecord, type CreateDNSRecordRequest, type ZoneACL,
} from '@/service/api/goddi/dns'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const zoneId = route.params.id as string
const zone = ref<DNSZone | null>(null)
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
  'A', 'AAAA', 'CNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT', 'CAA', 'TLSA',
].map(t => ({ label: t, value: t }))

const checkedKeys = ref<string[]>([])

const recordPagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
})

const recordForm = reactive<CreateDNSRecordRequest & { enabled: boolean }>({
  name: '',
  type: 'A',
  value: '',
  ttl: 3600,
  priority: undefined,
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
    aclForm.allow_query = [...(acl?.allow_query ?? [])]
    aclForm.allow_transfer = [...(acl?.allow_transfer ?? [])]
    aclForm.allow_update = [...(acl?.allow_update ?? [])]
    aclForm.notify = [...(acl?.notify ?? [])]
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

// Zone ACL editor state. Empty lists mean unrestricted.
const aclForm = reactive<Required<ZoneACL>>({
  allow_query: [],
  allow_transfer: [],
  allow_update: [],
  notify: [],
})
const aclSaving = ref(false)

async function saveACL() {
  aclSaving.value = true
  try {
    // Send the ACL unconditionally: an all-empty list clears restrictions
    // server-side because the store treats empty lists as "allow all".
    const acl: Required<ZoneACL> = {
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
  recordForm.enabled = true
}

onMounted(() => {
  loadZone()
  loadRecords()
})
</script>
