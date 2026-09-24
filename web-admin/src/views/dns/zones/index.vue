<script setup lang="ts">
import { ref, reactive, computed, h, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDNSZones, createDNSZone, updateDNSZone, deleteDNSZone, batchDeleteDNSZones, cloneDNSZone, convertDNSZone, type DNSZone, type CreateDNSZoneRequest } from '@/service/api/goddi/dns'

const router = useRouter()
const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const batchDeleting = ref(false)
const zones = ref<DNSZone[]>([])
const searchQuery = ref('')
const filterType = ref<string | null>(null)
const activeTab = ref<'authoritative' | 'allowed' | 'blocked'>('authoritative')
const showCreateModal = ref(false)
const showDeleteConfirm = ref(false)
const showBatchDeleteConfirm = ref(false)
const deletingId = ref('')
const editingZone = ref<DNSZone | null>(null)
const checkedKeys = ref<Array<string | number>>([])

const showCloneModal = ref(false)
const cloneSource = ref<DNSZone | null>(null)
const cloneName = ref('')
const showConvertModal = ref(false)
const convertSource = ref<DNSZone | null>(null)
const convertTarget = ref('primary')

const typeOptions = [
  { label: 'Primary', value: 'primary' },
  { label: 'Secondary', value: 'secondary' },
  { label: 'Stub', value: 'stub' },
  { label: 'Forward', value: 'forward' },
]

const zoneTypeOptions = [
  ...typeOptions,
  { label: 'Catalog', value: 'catalog' },
  { label: t('dns.zones.typeAllowed'), value: 'allowed' },
  { label: t('dns.zones.typeBlocked'), value: 'blocked' },
]

// Special (allowed/blocked) zones cannot be converted (backend enforces
// this too); only the four IN zone types are offered as targets.
const convertTargetOptions = computed(() => typeOptions.filter((o) => o.value !== convertSource.value?.type))

function handleTabChange(tab: string) {
  activeTab.value = tab as 'authoritative' | 'allowed' | 'blocked'
  filterType.value = null
  checkedKeys.value = []
  applyFilters()
}

const pagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
})

const formData = reactive<CreateDNSZoneRequest & { enabled: boolean }>({
  name: '',
  type: 'primary',
  default_ttl: 3600,
  soa_mname: '',
  soa_rname: '',
  refresh: 3600,
  retry: 600,
  expire: 604800,
  minimum: 86400,
  enabled: true,
})

const isSpecialType = computed(() => formData.type === 'allowed' || formData.type === 'blocked')

const columns = [
  { type: 'selection' as const },
  { title: () => t('dns.zones.zoneName'), key: 'name' },
  { title: () => t('dns.zones.zoneType'), key: 'type', width: 100, render: (row: DNSZone) => h(NTag, { size: 'small', type: row.type === 'primary' ? 'success' : row.type === 'allowed' ? 'success' : row.type === 'blocked' ? 'error' : 'info' }, { default: () => row.type }) },
  { title: () => t('dns.zones.records'), key: 'records_count', width: 80 },
  { title: () => t('dns.zones.dnssec'), key: 'dnssec_enabled', width: 90, render: (row: DNSZone) => h(NTag, { size: 'small', type: row.dnssec_enabled ? 'success' : 'default' }, { default: () => row.dnssec_enabled ? 'ON' : 'OFF' }) },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: DNSZone) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dns'), onUpdateValue: () => toggleEnabled(row) }) },
  { title: () => t('common.actions'), key: 'actions', width: 300, render: (row: DNSZone) => h(NSpace, { size: 'small' }, {
    default: () => [
      ...(row.type === 'allowed' || row.type === 'blocked' ? [] : [h(NButton, { size: 'small', text: true, onClick: () => router.push(`/dns/zone-detail/${row.id}`) }, { default: () => t('common.edit') })]),
      ...(row.type === 'allowed' || row.type === 'blocked' ? [] : [h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('dns'), onClick: () => openClone(row) }, { default: () => t('dns.zones.clone') })]),
      ...(row.type === 'allowed' || row.type === 'blocked' ? [] : [h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('dns'), onClick: () => openConvert(row) }, { default: () => t('dns.zones.convert') })]),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => { if (!perm.canDelete('dns')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (searchQuery.value) params.name = searchQuery.value
    if (activeTab.value === 'authoritative') {
      params.type = filterType.value || 'authoritative'
    } else {
      params.type = activeTab.value
    }
    const result = await listDNSZones(params)
    zones.value = result.data
    pagination.itemCount = result.meta.total
    // Drop selection keys that no longer exist on this page.
    const ids = new Set(result.data.map((z: DNSZone) => z.id))
    checkedKeys.value = checkedKeys.value.filter((k) => ids.has(String(k)))
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  pagination.page = 1
  loadData()
}

function openCreateZone() {
  if (!perm.canWrite('dns')) return
  editingZone.value = null
  const presetType = activeTab.value === 'allowed' ? 'allowed' : activeTab.value === 'blocked' ? 'blocked' : 'primary'
  Object.assign(formData, { name: '', type: presetType, default_ttl: 3600, soa_mname: '', soa_rname: '', refresh: 3600, retry: 600, expire: 604800, minimum: 86400, enabled: true })
  showCreateModal.value = true
}

function openClone(zone: DNSZone) {
  if (!perm.canWrite('dns')) return
  cloneSource.value = zone
  cloneName.value = `${zone.name}-copy`
  showCloneModal.value = true
}

function openConvert(zone: DNSZone) {
  if (!perm.canWrite('dns')) return
  convertSource.value = zone
  convertTarget.value = zone.type === 'primary' ? 'secondary' : 'primary'
  showConvertModal.value = true
}

async function handleClone() {
  if (!perm.canWrite('dns')) return
  if (!cloneSource.value || !cloneName.value.trim()) {
    message.warning(t('dns.zones.newNameRequired'))
    return
  }
  submitting.value = true
  try {
    await cloneDNSZone(cloneSource.value.id, cloneName.value.trim())
    message.success(t('common.createSuccess'))
    showCloneModal.value = false
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    submitting.value = false
  }
}

async function handleConvert() {
  if (!perm.canWrite('dns')) return
  if (!convertSource.value || !convertTarget.value) return
  submitting.value = true
  try {
    await convertDNSZone(convertSource.value.id, convertTarget.value)
    message.success(t('common.updateSuccess'))
    showConvertModal.value = false
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    submitting.value = false
  }
}

async function handleBatchDelete() {
  if (!perm.canDelete('dns')) return
  showBatchDeleteConfirm.value = false
  if (checkedKeys.value.length === 0) return
  batchDeleting.value = true
  try {
    const result = await batchDeleteDNSZones(checkedKeys.value.map((k) => String(k)))
    message.success(t('dns.zones.batchDeleteDone', { n: result.deleted }))
    checkedKeys.value = []
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    batchDeleting.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  loadData()
}

function handlePageSizeChange(pageSize: number) {
  pagination.pageSize = pageSize
  pagination.page = 1
  loadData()
}

async function toggleEnabled(zone: DNSZone) {
  if (!perm.canWrite('dns')) return
  try {
    await updateDNSZone(zone.id, { enabled: !zone.enabled })
    message.success(t('common.updateSuccess'))
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleSubmit() {
  if (!perm.canWrite('dns')) return
  submitting.value = true
  try {
    if (editingZone.value) {
      await updateDNSZone(editingZone.value.id, formData)
      message.success(t('common.updateSuccess'))
    } else {
      await createDNSZone(formData)
      message.success(t('common.createSuccess'))
    }
    showCreateModal.value = false
    editingZone.value = null
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    submitting.value = false
  }
}

async function handleDelete() {
  if (!perm.canDelete('dns')) return
  try {
    await deleteDNSZone(deletingId.value)
    message.success(t('common.deleteSuccess'))
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>

<template>
  <div>
    <PageHeader :title="t('dns.zones.title')">
      <NButton v-if="perm.canWrite('dns')" type="primary" @click="openCreateZone">
        {{ t('dns.zones.createZone') }}
      </NButton>
    </PageHeader>

    <div class="filter-bar">
      <NTabs v-model:value="activeTab" type="line" @update:value="handleTabChange">
        <NTab name="authoritative">{{ t('dns.zones.tabAuthoritative') }}</NTab>
        <NTab name="allowed">{{ t('dns.zones.tabAllowed') }}</NTab>
        <NTab name="blocked">{{ t('dns.zones.tabBlocked') }}</NTab>
      </NTabs>
      <NSpace align="center">
        <NInput v-model:value="searchQuery" :placeholder="t('common.search')" clearable style="width: 240px;" @keyup.enter="applyFilters" @clear="applyFilters">
          <template #prefix><NIcon><SearchOutline /></NIcon></template>
        </NInput>
        <NSelect v-if="activeTab === 'authoritative'" v-model:value="filterType" :options="typeOptions" clearable :placeholder="t('dns.zones.zoneType')" style="width: 160px;" @update:value="applyFilters" />
        <NButton @click="loadData">{{ t('common.refresh') }}</NButton>
        <template v-if="checkedKeys.length > 0">
          <NText depth="3">{{ t('dns.zones.selectedCount', { n: checkedKeys.length }) }}</NText>
          <NButton v-if="perm.canDelete('dns')" type="error" secondary :loading="batchDeleting" @click="showBatchDeleteConfirm = true">
            {{ t('dns.zones.batchDelete') }}
          </NButton>
        </template>
      </NSpace>
    </div>

    <NDataTable
      v-model:checked-row-keys="checkedKeys"
      :columns="columns"
      :data="zones"
      :loading="loading" remote
      :pagination="pagination"
      :row-key="(row: DNSZone) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit Zone Modal -->
    <NModal v-if="showCreateModal" v-model:show="showCreateModal" preset="card" :title="editingZone ? t('dns.zones.editZone') : t('dns.zones.createZone')" style="width: 640px;">
      <NForm :model="formData" label-placement="left" label-width="120px">
        <NFormItem :label="t('dns.zones.zoneName')" path="name">
          <NInput v-model:value="formData.name" :disabled="!!editingZone" />
        </NFormItem>
        <NFormItem :label="t('dns.zones.zoneType')" path="type">
          <NSelect v-model:value="formData.type" :options="zoneTypeOptions" :disabled="!!editingZone" />
        </NFormItem>
        <template v-if="!isSpecialType">
          <NFormItem :label="t('dns.zones.ttl')" path="default_ttl">
            <NInputNumber v-model:value="formData.default_ttl" :min="60" />
          </NFormItem>
          <NFormItem :label="t('dns.zones.primaryNs')" path="soa_mname">
            <NInput v-model:value="formData.soa_mname" />
          </NFormItem>
          <NFormItem :label="t('dns.zones.adminEmail')" path="soa_rname">
            <NInput v-model:value="formData.soa_rname" />
          </NFormItem>
          <NFormItem :label="t('dns.zones.refresh')" path="refresh">
            <NInputNumber v-model:value="formData.refresh" :min="0" />
          </NFormItem>
          <NFormItem :label="t('dns.zones.retry')" path="retry">
            <NInputNumber v-model:value="formData.retry" :min="0" />
          </NFormItem>
          <NFormItem :label="t('dns.zones.expire')" path="expire">
            <NInputNumber v-model:value="formData.expire" :min="0" />
          </NFormItem>
          <NFormItem :label="t('dns.zones.minimum')" path="minimum">
            <NInputNumber v-model:value="formData.minimum" :min="0" />
          </NFormItem>
        </template>
        <NFormItem v-if="activeTab !== 'authoritative' || isSpecialType" :label="t('dns.zones.specialHint')">
          <NText depth="3" style="font-size: 12px;">{{ t('dns.zones.specialHintText') }}</NText>
        </NFormItem>
        <NFormItem :label="t('common.enabled')" path="enabled">
          <NSwitch v-model:value="formData.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCreateModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('dns')" @click="handleSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Clone Zone Modal -->
    <NModal v-model:show="showCloneModal" preset="card" :title="t('dns.zones.cloneTitle')" style="width: 480px;">
      <NForm label-placement="left" label-width="120px">
        <NFormItem :label="t('dns.zones.zoneName')">
          <NText depth="2">{{ cloneSource?.name }}</NText>
        </NFormItem>
        <NFormItem :label="t('dns.zones.newName')">
          <NInput v-model:value="cloneName" :placeholder="cloneSource ? `${cloneSource.name}-copy` : ''" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCloneModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('dns')" @click="handleClone">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Convert Zone Type Modal -->
    <NModal v-model:show="showConvertModal" preset="card" :title="t('dns.zones.convertTitle')" style="width: 480px;">
      <NForm label-placement="left" label-width="120px">
        <NFormItem :label="t('dns.zones.zoneName')">
          <NText depth="2">{{ convertSource?.name }}</NText>
        </NFormItem>
        <NFormItem :label="t('dns.zones.convertTarget')">
          <NSelect v-model:value="convertTarget" :options="convertTargetOptions" />
        </NFormItem>
        <NFormItem>
          <NText depth="3" style="font-size: 12px;">{{ t('dns.zones.convertHint') }}</NText>
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showConvertModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('dns')" @click="handleConvert">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />

    <ConfirmDialog
      :show="showBatchDeleteConfirm"
      :message="t('dns.zones.batchDeleteConfirm', { n: checkedKeys.length })"
      @confirm="handleBatchDelete"
      @cancel="showBatchDeleteConfirm = false"
    />
  </div>
</template>
