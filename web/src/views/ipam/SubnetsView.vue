<template>
  <div>
    <page-header :title="t('ipam.subnets.title')">
      <n-space>
        <n-button v-if="perm.canWrite('ipam')" type="primary" @click="openCreate">{{ t('ipam.subnets.createSubnet') }}</n-button>
        <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
      </n-space>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="subnets"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: IPAMSubnet) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('ipam.subnets.editSubnet') : t('ipam.subnets.createSubnet')" preset="card" style="width: 550px;">
      <n-form :model="formData" label-placement="left" label-width="80px">
        <n-form-item :label="t('common.name')"><n-input v-model:value="formData.name" /></n-form-item>
        <n-form-item :label="t('ipam.spaces.title')">
          <n-select v-model:value="formData.space_id" :options="spaceOptions" />
        </n-form-item>
        <n-form-item :label="t('ipam.subnets.cidr')"><n-input v-model:value="formData.cidr" placeholder="192.168.1.0/24" /></n-form-item>
        <n-form-item :label="t('ipam.subnets.vlanId')"><n-input-number v-model:value="formData.vlan_id" :min="1" :max="4094" clearable /></n-form-item>
        <n-form-item :label="t('ipam.subnets.location')"><n-input v-model:value="formData.location" /></n-form-item>
        <n-form-item :label="t('common.descriptions')"><n-input v-model:value="formData.description" type="textarea" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listIPAMSubnets, createIPAMSubnet, updateIPAMSubnet, deleteIPAMSubnet, generateDHCPScope, generateReverseZone, type IPAMSubnet, type CreateIPAMSubnetRequest } from '@/api/ipam'
import { listIPAMSpaces, type IPAMSpace } from '@/api/ipam'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const subnets = ref<IPAMSubnet[]>([])
const spaces = ref<IPAMSpace[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const editing = ref<IPAMSubnet | null>(null)

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateIPAMSubnetRequest>({ name: '', space_id: '', cidr: '', vlan_id: undefined, location: '', description: '' })

const spaceOptions = ref<Array<{ label: string; value: string }>>([])

const columns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('ipam.subnets.cidr'), key: 'cidr' },
  { title: () => t('ipam.subnets.vlanId'), key: 'vlan_id', width: 100 },
  { title: () => t('ipam.subnets.location'), key: 'location' },
  { title: () => t('common.actions'), key: 'actions', width: 240, render: (row: IPAMSubnet) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { editing.value = row; Object.assign(formData, { name: row.name, space_id: row.space_id, cidr: row.cidr, vlan_id: row.vlan_id, location: row.location, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', disabled: !perm.canWrite('ipam'), onClick: () => handleGenerateDhcp(row.id) }, { default: () => t('ipam.subnets.generateDhcpScope') }),
      h(NButton, { size: 'small', disabled: !perm.canWrite('ipam'), onClick: () => handleGenerateReverse(row.id) }, { default: () => t('ipam.subnets.generateReverseZone') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('ipam'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const [subnetsResult, spacesResult] = await Promise.all([
      listIPAMSubnets({ page: pagination.page, page_size: pagination.pageSize }),
      listIPAMSpaces({ page: 1, page_size: 100 }),
    ])
    subnets.value = subnetsResult.data
    pagination.itemCount = subnetsResult.meta.total
    spaces.value = spacesResult.data
    spaceOptions.value = spacesResult.data.map(s => ({ label: s.name, value: s.id }))
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  editing.value = null
  Object.assign(formData, { name: '', space_id: '', cidr: '', vlan_id: undefined, location: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (editing.value) { await updateIPAMSubnet(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createIPAMSubnet(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteIPAMSubnet(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

async function handleGenerateDhcp(subnetId: string) {
  try { await generateDHCPScope(subnetId); message.success(t('common.success')) } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleGenerateReverse(subnetId: string) {
  try { await generateReverseZone(subnetId); message.success(t('common.success')) } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(loadData)
</script>
