<template>
  <div>
    <page-header :title="t('dns.zones.title')">
      <n-button v-if="perm.canWrite('dns')" type="primary" @click="showCreateModal = true">
        {{ t('dns.zones.createZone') }}
      </n-button>
    </page-header>

    <n-card style="margin-bottom: 16px;">
      <n-space>
        <n-input v-model:value="searchQuery" :placeholder="t('common.search')" clearable style="width: 240px;" @keyup.enter="loadData">
          <template #prefix><n-icon><search-outline /></n-icon></template>
        </n-input>
        <n-select v-model:value="filterType" :options="typeOptions" :placeholder="t('dns.zones.zoneType')" style="width: 160px;" @update:value="loadData" />
        <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
      </n-space>
    </n-card>

    <n-data-table
      :columns="columns"
      :data="zones"
      :loading="loading"
      :pagination="zones.length > 0 ? pagination : false"
      :row-key="(row: DNSZone) => row.id"
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

    <confirm-dialog
      :show="showDeleteConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDNSZones, createDNSZone, updateDNSZone, deleteDNSZone, type DNSZone, type CreateDNSZoneRequest } from '@/api/dns'

const router = useRouter()
const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const zones = ref<DNSZone[]>([])
const searchQuery = ref('')
const filterType = ref<string | null>(null)
const showCreateModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const editingZone = ref<DNSZone | null>(null)

const typeOptions = [
  { label: 'Primary', value: 'primary' },
  { label: 'Secondary', value: 'secondary' },
  { label: 'Stub', value: 'stub' },
  { label: 'Forward', value: 'forward' },
]

const zoneTypeOptions = typeOptions

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
  { title: t('dns.zones.zoneName'), key: 'name' },
  { title: t('dns.zones.zoneType'), key: 'type', width: 100, render: (row: DNSZone) => h(NTag, { size: 'small', type: row.type === 'primary' ? 'success' : 'info' }, { default: () => row.type }) },
  { title: t('dns.zones.records'), key: 'records_count', width: 80 },
  { title: t('dns.zones.dnssec'), key: 'dnssec_enabled', width: 90, render: (row: DNSZone) => h(NTag, { size: 'small', type: row.dnssec_enabled ? 'success' : 'default' }, { default: () => row.dnssec_enabled ? 'ON' : 'OFF' }) },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: DNSZone) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dns'), onUpdateValue: () => toggleEnabled(row) }) },
  { title: t('common.actions'), key: 'actions', width: 200, render: (row: DNSZone) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => router.push(`/dns/zones/${row.id}`) }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dns'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (searchQuery.value) params.search = searchQuery.value
    if (filterType.value) params.type = filterType.value
    const result = await listDNSZones(params)
    zones.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    loading.value = false
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
