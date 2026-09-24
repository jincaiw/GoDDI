<script setup lang="ts">
import { ref, reactive, computed, h, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { ApiError } from '@/service/api/goddi/client'
import {
  getDNSZone, updateDNSZone, exportZoneFile, importZoneFile, syncSecondaryZone,
  enableDNSSEC, disableDNSSEC, rotateDNSSECKeys, getDNSSECStatus,
  listDNSRecords, createDNSRecord, updateDNSRecord, deleteDNSRecord,
  getZoneHistory, getZonePermissions, setZonePermissions, getCatalogMembers, listDNSZones,
  getZoneDSRecords, generateDNSSECKey, deleteDNSSECKey, toggleDNSSECKey, promoteDNSSECStandbyKeys,
  getNSEC3Params, setNSEC3Params,
  type DNSZone, type DNSRecord, type CreateDNSRecordRequest, type ZoneACL,
  type ZonePermission, type ZoneChangeEntry, type DNSSECKey, type DNSSECStatus, type DSInfo, type ZoneImportPreview,
} from '@/service/api/goddi/dns'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const showImport = ref(false)
const importFormat = ref<'bind' | 'csv'>('bind')
const importContent = ref('')
const importPreview = ref<ZoneImportPreview | null>(null)
const importPreviewLoading = ref(false)
const importApplying = ref(false)
const importFormatOptions = [
  { label: 'BIND zone file', value: 'bind' },
  { label: 'CSV', value: 'csv' },
]

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
].map(type => ({ label: type, value: type }))

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
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('dns'), onClick: () => editRecord(row) }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => { if (!perm.canDelete('dns')) return; deletingRecordId.value = row.id; showDeleteRecordConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

// Zone ACL editor state. Empty lists mean unrestricted.
const aclForm = reactive<ZoneACL & { query_access?: string }>({
  query_access: '',
  allow_query: [],
  allow_transfer: [],
  allow_update: [],
  notify: [],
})
const aclSaving = ref(false)

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
  if (!perm.canWrite('dns')) return
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

const queryAccessOptions = [
  { label: t('dns.zones.queryAccessDefault'), value: '' },
  { label: t('dns.zones.queryAccessAllow'), value: 'allow' },
  { label: t('dns.zones.queryAccessDeny'), value: 'deny' },
  { label: t('dns.zones.queryAccessPrivate'), value: 'allow_only_private_networks' },
]

async function saveACL() {
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
  try {
    await updateDNSRecord(zoneId, record.id, { enabled: !record.enabled })
    message.success(t('common.updateSuccess'))
    loadRecords()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

function editRecord(record: DNSRecord) {
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
  editingRecord.value = null
  resetRecordForm()
  showAddRecord.value = true
}

async function handleRecordSubmit() {
  if (!perm.canWrite('dns')) return
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
  if (!perm.canDelete('dns')) return
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

function clearImportPreview() {
  importPreview.value = null
}

async function loadImportFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    importContent.value = await file.text()
    importPreview.value = null
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    input.value = ''
  }
}

async function previewImport() {
  if (!perm.canWrite('dns')) return
  importPreviewLoading.value = true
  try {
    const response = await importZoneFile(zoneId, importContent.value, importFormat.value, true)
    importPreview.value = response.data.data as ZoneImportPreview
  } catch (err: unknown) {
    if (err instanceof ApiError && err.payload && typeof err.payload === 'object' && 'conflicts' in err.payload) {
      importPreview.value = err.payload as ZoneImportPreview
    } else {
      importPreview.value = null
    }
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    importPreviewLoading.value = false
  }
}

async function applyImport() {
  if (!perm.canWrite('dns')) return
  if (!importPreview.value?.valid) return
  importApplying.value = true
  try {
    await importZoneFile(zoneId, importContent.value, importFormat.value)
    message.success(t('dns.zones.importSuccess'))
    showImport.value = false
    importContent.value = ''
    importPreview.value = null
    loadZone()
    loadRecords()
  } catch (err: unknown) {
    if (err instanceof ApiError && err.payload && typeof err.payload === 'object' && 'conflicts' in err.payload) {
      importPreview.value = err.payload as ZoneImportPreview
    } else {
      importPreview.value = null
    }
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    importApplying.value = false
  }
}

async function handleSync() {
  if (!perm.canWrite('dns')) return
  try {
    await syncSecondaryZone(zoneId)
    message.success(t('common.success'))
    loadZone()
    loadRecords()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

const dnssecLoaded = ref(false)

async function handleDnssecAction(action: string) {
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
  permissions.value.push({ principal_type: 'user', principal_id: '', can_view: true, can_modify: false, can_delete: false })
}

function removePermission(row: ZonePermission) {
  if (!perm.canWrite('dns')) return
  permissions.value = permissions.value.filter(p => !(p.principal_type === row.principal_type && p.principal_id === row.principal_id))
}

async function savePermissions() {
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
  try {
    await toggleDNSSECKey(zoneId, key.id, enabled)
    key.enabled = enabled
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleDeleteKey(key: DNSSECKey) {
  if (!perm.canDelete('dns')) return
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
  if (!perm.canWrite('dns')) return
  generateKeyType.value = 'KSK'
  generateKeyAlgorithm.value = 'ECDSAP256SHA256'
  showGenerateKey.value = true
}

async function handleGenerateKey() {
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
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

<template>
  <div>
    <PageHeader :title="zone?.name || ''" :subtitle="t('dns.zones.title')">
      <NSpace>
        <NButton @click="router.push('/dns/zones')">{{ t('common.cancel') }}</NButton>
        <NButton v-if="perm.canWrite('dns')" type="primary" @click="openCreateRecord">{{ t('dns.records.createRecord') }}</NButton>
        <NButton v-if="perm.canWrite('dns')" @click="showImport = true">{{ t('dns.zones.importZone') }}</NButton>
        <NButton v-if="perm.canRead('dns')" @click="handleExport">{{ t('dns.zones.exportZone') }}</NButton>
        <NButton v-if="perm.canWrite('dns')" :disabled="zone?.type !== 'slave'" @click="handleSync">{{ t('dns.zones.syncZone') }}</NButton>
      </NSpace>
    </PageHeader>

    <NCard>
      <NTabs v-model:value="activeTab" type="line" @update:value="handleTabChange">
        <!-- Records -->
        <NTabPane name="records" :tab="t('dns.zones.tabRecords')" display-directive="show">
          <NSpace justify="end" style="margin-bottom: 12px;">
            <NInput v-model:value="recordSearch" :placeholder="t('common.search')" clearable style="width: 200px;" @keyup.enter="loadRecords">
              <template #prefix><NIcon><SearchOutline /></NIcon></template>
            </NInput>
            <NSelect v-model:value="recordTypeFilter" :options="recordTypeOptions" clearable :placeholder="t('dns.records.recordType')" style="width: 130px;" @update:value="loadRecords" />
            <NButton @click="loadRecords">{{ t('common.refresh') }}</NButton>
          </NSpace>

          <NDataTable
            :columns="recordColumns"
            :data="records"
            :loading="recordsLoading"
            remote :pagination="recordPagination"
            :row-key="(row: DNSRecord) => row.id"
            :checked-row-keys="checkedKeys"
            @update:page="handleRecordPageChange"
            @update:page-size="handleRecordPageSizeChange"
          />
        </NTabPane>

        <!-- Options -->
        <NTabPane name="options" :tab="t('dns.zones.tabOptions')" display-directive="show">
          <NDescriptions v-if="zone" :column="4" label-placement="left" style="margin-bottom: 16px;">
            <NDescriptionsItem :label="t('dns.zones.zoneType')">{{ zone.type }}</NDescriptionsItem>
            <NDescriptionsItem :label="t('dns.zones.ttl')">{{ zone.default_ttl }}</NDescriptionsItem>
            <NDescriptionsItem :label="t('dns.zones.serial')">{{ zone.serial }}</NDescriptionsItem>
            <NDescriptionsItem :label="t('common.enabled')">
              <NSwitch :value="zone.enabled" :disabled="!perm.canWrite('dns')" @update:value="toggleZoneEnabled" />
            </NDescriptionsItem>
            <NDescriptionsItem :label="t('dns.zones.primaryNs')">{{ zone.soa_mname || '-' }}</NDescriptionsItem>
            <NDescriptionsItem :label="t('dns.zones.adminEmail')">{{ zone.soa_rname || '-' }}</NDescriptionsItem>
            <NDescriptionsItem :label="t('dns.zones.refresh')">{{ zone.refresh }}</NDescriptionsItem>
            <NDescriptionsItem :label="t('dns.zones.minimum')">{{ zone.minimum }}</NDescriptionsItem>
          </NDescriptions>

          <NCard v-if="zone && zone.type !== 'allowed' && zone.type !== 'blocked'" :title="t('dns.zones.aclTitle')" embedded>
            <NForm label-placement="left" label-width="180px">
              <NFormItem :label="t('dns.zones.queryAccess')">
                <NSelect v-model:value="aclForm.query_access" :options="queryAccessOptions" :disabled="!perm.canWrite('dns')" style="width: 320px;" clearable />
              </NFormItem>
              <NFormItem :label="t('dns.zones.aclAllowQuery')">
                <NDynamicTags v-model:value="aclForm.allow_query" :disabled="!perm.canWrite('dns')" />
              </NFormItem>
              <NFormItem :label="t('dns.zones.aclAllowTransfer')">
                <NDynamicTags v-model:value="aclForm.allow_transfer" :disabled="!perm.canWrite('dns')" />
              </NFormItem>
              <NFormItem :label="t('dns.zones.aclAllowUpdate')">
                <NDynamicTags v-model:value="aclForm.allow_update" :disabled="!perm.canWrite('dns')" />
              </NFormItem>
              <NFormItem :label="t('dns.zones.aclNotify')">
                <NDynamicTags v-model:value="aclForm.notify" :disabled="!perm.canWrite('dns')" />
              </NFormItem>
            </NForm>
            <NText depth="3" style="font-size: 12px;">{{ t('dns.zones.aclHint') }}</NText>
            <NDivider />
            <NSpace>
              <NButton v-if="perm.canWrite('dns')" type="primary" size="small" :loading="aclSaving" @click="saveACL">{{ t('common.save') }}</NButton>
            </NSpace>
          </NCard>

          <!-- Catalog zone: member list (RFC 9432) -->
          <NCard v-if="zone && zone.type === 'catalog'" :title="t('dns.zones.catalogMembers')" embedded style="margin-top: 16px;">
            <NSpace v-if="catalogMembers.length" size="small">
              <NTag v-for="m in catalogMembers" :key="m" size="small" type="info">{{ m }}</NTag>
            </NSpace>
            <NEmpty v-else :description="t('dns.zones.catalogMembersEmpty')" size="small" />
          </NCard>

          <!-- Non-catalog zone: join/leave a catalog -->
          <NCard v-if="zone && !['allowed', 'blocked', 'catalog'].includes(zone.type)" :title="t('dns.zones.catalogTitle')" embedded style="margin-top: 16px;">
            <NForm label-placement="left" label-width="180px">
              <NFormItem :label="t('dns.zones.catalogJoin')">
                <NSelect v-model:value="catalogForm.catalog" :options="catalogOptions" :disabled="!perm.canWrite('dns')" style="width: 320px;" />
              </NFormItem>
            </NForm>
            <NText depth="3" style="font-size: 12px;">{{ t('dns.zones.catalogHint') }}</NText>
            <NDivider />
            <NSpace>
              <NButton v-if="perm.canWrite('dns')" type="primary" size="small" :loading="catalogSaving" @click="saveCatalog">{{ t('common.save') }}</NButton>
            </NSpace>
          </NCard>
        </NTabPane>

        <!-- Permissions -->
        <NTabPane name="permissions" :tab="t('dns.zones.tabPermissions')" display-directive="show">
          <NAlert type="info" size="small" style="margin-bottom: 12px;">{{ t('dns.zones.permHint') }}</NAlert>
          <NSpace justify="end" style="margin-bottom: 12px;">
            <NButton v-if="perm.canWrite('dns')" size="small" @click="addPermission">{{ t('dns.zones.permAdd') }}</NButton>
            <NButton v-if="perm.canWrite('dns')" type="primary" size="small" :loading="permSaving" @click="savePermissions">{{ t('common.save') }}</NButton>
          </NSpace>
          <NDataTable :columns="permissionColumns" :data="permissions" :row-key="(row: ZonePermission) => row.principal_type + ':' + row.principal_id" />
        </NTabPane>

        <!-- History -->
        <NTabPane name="history" :tab="t('dns.zones.tabHistory')" display-directive="show">
          <NDataTable
            :columns="historyColumns"
            :data="history"
            :loading="historyLoading"
            remote :pagination="historyPagination"
            :row-key="(row: ZoneChangeEntry) => row.id"
            @update:page="handleHistoryPageChange"
          />
        </NTabPane>

        <!-- DNSSEC -->
        <NTabPane name="dnssec" :tab="t('dns.zones.tabDnssec')" display-directive="show">
          <NAlert type="warning" size="small" style="margin-bottom: 12px;">
            {{ t('dns.zones.dnssecExperimentalHint') }}
          </NAlert>
          <NSpace style="margin-bottom: 16px;">
            <NButton v-if="!zone?.dnssec_enabled && perm.canWrite('dns')" type="warning" size="small" @click="handleDnssecAction('enable')">{{ t('dns.zones.enableDnssec') }}</NButton>
            <NButton v-if="zone?.dnssec_enabled && perm.canWrite('dns')" type="warning" size="small" @click="handleDnssecAction('disable')">{{ t('dns.zones.disableDnssec') }}</NButton>
            <NButton v-if="zone?.dnssec_enabled && perm.canWrite('dns')" size="small" @click="handleDnssecAction('rotate')">{{ t('dns.zones.rotateKeys') }}</NButton>
            <NButton v-if="perm.canWrite('dns')" size="small" type="primary" ghost @click="openGenerateKey">{{ t('dns.zones.dnssecGenerateKey') }}</NButton>
            <NButton v-if="perm.canWrite('dns')" size="small" @click="handlePromoteStandby">{{ t('dns.zones.dnssecPromote') }}</NButton>
            <NButton size="small" @click="loadDsRecords">{{ t('dns.zones.dnssecShowDs') }}</NButton>
          </NSpace>
          <NDataTable
            v-if="dnssecStatus?.keys?.length"
            :columns="dnssecKeyColumns"
            :data="dnssecStatus.keys"
            :row-key="(row: DNSSECKey) => row.id"
          />
          <NCard v-if="dsRecords.length" :title="t('dns.zones.dnssecDsTitle')" embedded style="margin-top: 16px;">
            <NDataTable :columns="dsColumns" :data="dsRecords" :row-key="(row: DSInfo) => String(row.key_tag)" />
          </NCard>
          <NCard :title="t('dns.zones.dnssecNsec3Title')" embedded style="margin-top: 16px;">
            <NForm label-placement="left" label-width="160px" inline>
              <NFormItem :label="t('dns.zones.dnssecNsec3Iterations')">
                <NInputNumber v-model:value="nsec3Form.iterations" :min="0" :max="100" :disabled="!perm.canWrite('dns')" />
              </NFormItem>
              <NFormItem :label="t('dns.zones.dnssecNsec3Salt')">
                <NInput v-model:value="nsec3Form.salt" placeholder="hex (可留空)" :disabled="!perm.canWrite('dns')" style="width: 220px;" />
              </NFormItem>
              <NFormItem :label="t('dns.zones.dnssecNsec3Optout')">
                <NSwitch v-model:value="nsec3Form.optout" :disabled="!perm.canWrite('dns')" />
              </NFormItem>
            </NForm>
            <NText depth="3" style="font-size: 12px;">{{ t('dns.zones.dnssecNsec3Hint') }}</NText>
            <NDivider />
            <NButton v-if="perm.canWrite('dns')" type="primary" size="small" :loading="nsec3Saving" @click="saveNsec3">{{ t('common.save') }}</NButton>
          </NCard>
        </NTabPane>
      </NTabs>
    </NCard>

    <NModal v-model:show="showImport" preset="card" :title="t('dns.zones.importZone')" style="width: min(760px, 92vw);">
      <NSpace vertical size="large">
        <NAlert type="info" size="small">{{ t('dns.zones.importPreviewHint') }}</NAlert>
        <NSpace align="center">
          <NSelect v-model:value="importFormat" :options="importFormatOptions" style="width: 180px" @update:value="clearImportPreview" />
          <input type="file" :accept="importFormat === 'csv' ? '.csv,text/csv' : '.zone,.bind,.txt,text/plain'" @change="loadImportFile" />
        </NSpace>
        <NInput v-model:value="importContent" type="textarea" :rows="12" :placeholder="t('dns.zones.importContentPlaceholder')" @update:value="clearImportPreview" />
        <NAlert v-if="importPreview" :type="importPreview.valid ? 'success' : 'warning'" :title="importPreview.valid ? t('dns.zones.importPreviewValid') : t('dns.zones.importPreviewConflicts')">
          <div>{{ t('dns.zones.importRecordCount', { count: importPreview.record_count }) }}</div>
          <div v-if="Object.keys(importPreview.record_types || {}).length">{{ Object.entries(importPreview.record_types).map(([type, count]) => `${type}: ${count}`).join(' · ') }}</div>
          <ul v-if="importPreview.conflicts?.length">
            <li v-for="(conflict, index) in importPreview.conflicts" :key="`${conflict.row}-${index}`">
              {{ t('dns.zones.importConflictRow', { row: conflict.row ?? conflict.record, owner: conflict.owner, type: conflict.type, message: conflict.message }) }}
            </li>
          </ul>
        </NAlert>
      </NSpace>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showImport = false">{{ t('common.cancel') }}</NButton>
          <NButton :loading="importPreviewLoading" :disabled="!perm.canWrite('dns') || !importContent.trim()" @click="previewImport">{{ t('dns.zones.importPreview') }}</NButton>
          <NButton type="primary" :loading="importApplying" :disabled="!perm.canWrite('dns') || !importPreview?.valid" @click="applyImport">{{ t('dns.zones.importApply') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Add/Edit Record Modal -->
    <NModal v-if="showAddRecord" v-model:show="showAddRecord" preset="card" :title="editingRecord ? t('dns.records.editRecord') : t('dns.records.createRecord')" style="width: 500px;">
      <NForm :model="recordForm" label-placement="left" label-width="100px">
        <NFormItem :label="t('dns.records.recordName')">
          <NInput v-model:value="recordForm.name" />
        </NFormItem>
        <NFormItem :label="t('dns.records.recordType')">
          <NSelect v-model:value="recordForm.type" :options="recordTypeOptions" :disabled="!!editingRecord" />
        </NFormItem>
        <NFormItem :label="t('dns.records.recordValue')">
          <NInput v-model:value="recordForm.value" type="textarea" :rows="3" />
        </NFormItem>
        <NFormItem :label="t('dns.zones.ttl')">
          <NInputNumber v-model:value="recordForm.ttl" :min="0" />
        </NFormItem>
        <NFormItem :label="t('common.priority')">
          <NInputNumber v-model:value="recordForm.priority" :min="0" :max="65535" />
        </NFormItem>
        <NFormItem label="expiry_ttl">
          <NInputNumber v-model:value="recordForm.expiry_ttl" :min="0" clearable placeholder="seconds / 0 = never" />
        </NFormItem>
        <NFormItem :label="t('common.enabled')">
          <NSwitch v-model:value="recordForm.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showAddRecord = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="recordSubmitting" :disabled="!perm.canWrite('dns')" @click="handleRecordSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Generate DNSSEC Key Modal -->
    <NModal v-if="showGenerateKey" v-model:show="showGenerateKey" preset="card" :title="t('dns.zones.dnssecGenerateKey')" style="width: 420px;">
      <NForm label-placement="left" label-width="100px">
        <NFormItem label="Key Type">
          <NSelect v-model:value="generateKeyType" :options="[{ label: 'KSK', value: 'KSK' }, { label: 'ZSK', value: 'ZSK' }]" />
        </NFormItem>
        <NFormItem :label="t('dns.records.recordType') + ' / ' + 'Algorithm'">
          <NSelect v-model:value="generateKeyAlgorithm" :options="dnssecAlgorithmOptions" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showGenerateKey = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :disabled="!perm.canWrite('dns')" @click="handleGenerateKey">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog
      :show="showDeleteRecordConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDeleteRecord"
      @cancel="showDeleteRecordConfirm = false"
    />
  </div>
</template>
