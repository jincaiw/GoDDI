<template>
  <div>
    <page-header :title="t('dns.zones.title')">
      <n-button v-if="perm.canWrite('dns')" type="primary" @click="openCreateZone">
        {{ t('dns.zones.createZone') }}
      </n-button>
    </page-header>

    <div class="filter-bar">
      <n-tabs v-model:value="activeTab" type="line" @update:value="handleTabChange">
        <n-tab name="authoritative">{{ t('dns.zones.tabAuthoritative') }}</n-tab>
        <n-tab name="allowed">{{ t('dns.zones.tabAllowed') }}</n-tab>
        <n-tab name="blocked">{{ t('dns.zones.tabBlocked') }}</n-tab>
      </n-tabs>
      <n-space align="center">
        <n-input v-model:value="searchQuery" :placeholder="t('common.search')" clearable style="width: 240px;" @keyup.enter="applyFilters" @clear="applyFilters">
          <template #prefix><n-icon><search-outline /></n-icon></template>
        </n-input>
        <n-select v-if="activeTab === 'authoritative'" v-model:value="filterType" :options="typeOptions" clearable :placeholder="t('dns.zones.zoneType')" style="width: 160px;" @update:value="applyFilters" />
        <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
        <template v-if="checkedKeys.length > 0">
          <n-text depth="3">{{ t('dns.zones.selectedCount', { n: checkedKeys.length }) }}</n-text>
          <n-button v-if="perm.canDelete('dns')" type="error" secondary :loading="batchDeleting" @click="showBatchDeleteConfirm = true">
            {{ t('dns.zones.batchDelete') }}
          </n-button>
        </template>
      </n-space>
    </div>

    <n-data-table
      :columns="columns"
      :data="zones"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: DNSZone) => row.id"
      v-model:checked-row-keys="checkedKeys"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit Zone Modal -->
    <n-modal v-if="showCreateModal" v-model:show="showCreateModal" preset="card" :title="editingZone ? t('dns.zones.editZone') : t('dns.zones.createZone')" style="width: 640px;">
      <n-form ref="formRef" :model="formData" label-placement="left" label-width="120px">
        <n-form-item :label="t('dns.zones.zoneName')" path="name">
          <n-input v-model:value="formData.name" :disabled="!!editingZone" />
        </n-form-item>
        <n-form-item :label="t('dns.zones.zoneType')" path="type">
          <n-select v-model:value="formData.type" :options="zoneTypeOptions" :disabled="!!editingZone" />
        </n-form-item>
        <template v-if="!isSpecialType">
          <n-form-item :label="t('dns.zones.ttl')" path="default_ttl">
            <n-input-number v-model:value="formData.default_ttl" :min="60" />
          </n-form-item>
          <n-form-item :label="t('dns.zones.primaryNs')" path="soa_mname">
            <n-input v-model:value="formData.soa_mname" />
          </n-form-item>
          <n-form-item :label="t('dns.zones.adminEmail')" path="soa_rname">
            <n-input v-model:value="formData.soa_rname" />
          </n-form-item>
          <n-form-item :label="t('dns.zones.refresh')" path="refresh">
            <n-input-number v-model:value="formData.refresh" :min="0" />
          </n-form-item>
          <n-form-item :label="t('dns.zones.retry')" path="retry">
            <n-input-number v-model:value="formData.retry" :min="0" />
          </n-form-item>
          <n-form-item :label="t('dns.zones.expire')" path="expire">
            <n-input-number v-model:value="formData.expire" :min="0" />
          </n-form-item>
          <n-form-item :label="t('dns.zones.minimum')" path="minimum">
            <n-input-number v-model:value="formData.minimum" :min="0" />
          </n-form-item>
        </template>
        <n-form-item v-if="activeTab !== 'authoritative' || isSpecialType" :label="t('dns.zones.specialHint')">
          <n-text depth="3" style="font-size: 12px;">{{ t('dns.zones.specialHintText') }}</n-text>
        </n-form-item>
        <n-form-item :label="t('common.enabled')" path="enabled">
          <n-switch v-model:value="formData.enabled" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showCreateModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Clone Zone Modal -->
    <n-modal v-model:show="showCloneModal" preset="card" :title="t('dns.zones.cloneTitle')" style="width: 480px;">
      <n-form label-placement="left" label-width="120px">
        <n-form-item :label="t('dns.zones.zoneName')">
          <n-text depth="2">{{ cloneSource?.name }}</n-text>
        </n-form-item>
        <n-form-item :label="t('dns.zones.newName')">
          <n-input v-model:value="cloneName" :placeholder="cloneSource ? `${cloneSource.name}-copy` : ''" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showCloneModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleClone">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Convert Zone Type Modal -->
    <n-modal v-model:show="showConvertModal" preset="card" :title="t('dns.zones.convertTitle')" style="width: 480px;">
      <n-form label-placement="left" label-width="120px">
        <n-form-item :label="t('dns.zones.zoneName')">
          <n-text depth="2">{{ convertSource?.name }}</n-text>
        </n-form-item>
        <n-form-item :label="t('dns.zones.convertTarget')">
          <n-select v-model:value="convertTarget" :options="convertTargetOptions" />
        </n-form-item>
        <n-form-item>
          <n-text depth="3" style="font-size: 12px;">{{ t('dns.zones.convertHint') }}</n-text>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showConvertModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleConvert">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog
      :show="showDeleteConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />

    <confirm-dialog
      :show="showBatchDeleteConfirm"
      :message="t('dns.zones.batchDeleteConfirm', { n: checkedKeys.length })"
      @confirm="handleBatchDelete"
      @cancel="showBatchDeleteConfirm = false"
    />
  </div>
</template>

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

const isSpecialType = computed(() => formData.type === 'allowed' || formData.type === 'blocked')

const typeOptions = [
  { label: 'Primary', value: 'primary' },
  { label: 'Secondary', value: 'secondary' },
  { label: 'Stub', value: 'stub' },
  { label: 'Forward', value: 'forward' },
]

const zoneTypeOptions = [
  ...typeOptions,
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
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  editingZone.value = null
  const presetType = activeTab.value === 'allowed' ? 'allowed' : activeTab.value === 'blocked' ? 'blocked' : 'primary'
  Object.assign(formData, { name: '', type: presetType, default_ttl: 3600, soa_mname: '', soa_rname: '', refresh: 3600, retry: 600, expire: 604800, minimum: 86400, enabled: true })
  showCreateModal.value = true
}

function openClone(zone: DNSZone) {
  cloneSource.value = zone
  cloneName.value = `${zone.name}-copy`
  showCloneModal.value = true
}

function openConvert(zone: DNSZone) {
  convertSource.value = zone
  convertTarget.value = zone.type === 'primary' ? 'secondary' : 'primary'
  showConvertModal.value = true
}

async function handleClone() {
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
  try {
    await updateDNSZone(zone.id, { enabled: !zone.enabled })
    message.success(t('common.updateSuccess'))
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function handleSubmit() {
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
