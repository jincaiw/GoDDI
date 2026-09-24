<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ImportAddressesDialog from '@/components/ipam/ImportAddressesDialog.vue'
import PoolWizardDialog from '@/components/ipam/PoolWizardDialog.vue'
import ReverseZoneDialog from '@/components/ipam/ReverseZoneDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listIPAMSubnets, createIPAMSubnet, updateIPAMSubnet, deleteIPAMSubnet, type IPAMSubnet, type CreateIPAMSubnetRequest } from '@/service/api/goddi/ipam'
import { listIPAMSpaces, type IPAMSpace } from '@/service/api/goddi/ipam'

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

// Importing targets one subnet: the row's outcome depends on which subnet the
// address lands in, so the entry point carries it rather than asking again.
const showImport = ref(false)
const importSubnetId = ref('')
const importSubnetLabel = ref('')

function openImport(row: IPAMSubnet) {
  if (!perm.canRead('ipam')) return
  importSubnetId.value = row.id
  importSubnetLabel.value = `${row.name} (${row.cidr})`
  showImport.value = true
}

// Same for the pool wizard: a scope is built from one subnet's addresses, and
// the plan it shows is that subnet's.
const showPool = ref(false)
const poolSubnetId = ref('')
const poolSubnetLabel = ref('')

function openPool(row: IPAMSubnet) {
  if (!perm.canWrite('ipam')) return
  poolSubnetId.value = row.id
  poolSubnetLabel.value = `${row.name} (${row.cidr})`
  showPool.value = true
}

// And for the reverse zone. The entry point only gathers the subnet; the dialog
// asks which delegation and then performs the write, because the two are
// different permissions and the second one is the one that used to be faked.
const showReverseZone = ref(false)
const reverseZoneSubnetId = ref('')
const reverseZoneSubnetLabel = ref('')

function openReverseZone(row: IPAMSubnet) {
  if (!perm.canWrite('ipam')) return
  reverseZoneSubnetId.value = row.id
  reverseZoneSubnetLabel.value = `${row.name} (${row.cidr})`
  showReverseZone.value = true
}

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateIPAMSubnetRequest>({ name: '', space_id: '', cidr: '', vlan_id: undefined, location: '', description: '' })

const spaceOptions = ref<Array<{ label: string; value: string }>>([])

const columns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('ipam.subnets.cidr'), key: 'cidr' },
  { title: () => t('ipam.subnets.vlanId'), key: 'vlan_id', width: 100 },
  { title: () => t('ipam.subnets.location'), key: 'location' },
  { title: () => t('common.actions'), key: 'actions', width: 340, render: (row: IPAMSubnet) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('ipam'), onClick: () => { if (!perm.canWrite('ipam')) return; editing.value = row; Object.assign(formData, { name: row.name, space_id: row.space_id, cidr: row.cidr, vlan_id: row.vlan_id, location: row.location, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, disabled: !perm.canRead('ipam'), onClick: () => openImport(row) }, { default: () => t('ipam.subnets.importAddresses') }),
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('ipam'), onClick: () => openPool(row) }, { default: () => t('ipam.subnets.createDhcpScope') }),
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('ipam'), onClick: () => openReverseZone(row) }, { default: () => t('ipam.subnets.generateReverseZone') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('ipam'), onClick: () => { if (!perm.canDelete('ipam')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  if (!perm.canWrite('ipam')) return
  editing.value = null
  Object.assign(formData, { name: '', space_id: '', cidr: '', vlan_id: undefined, location: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  if (!perm.canWrite('ipam')) return
  submitting.value = true
  try {
    if (editing.value) { await updateIPAMSubnet(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createIPAMSubnet(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('ipam')) return
  try { await deleteIPAMSubnet(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>

<template>
  <div>
    <PageHeader :title="t('ipam.subnets.title')">
      <NSpace>
        <NButton v-if="perm.canWrite('ipam')" type="primary" @click="openCreate">{{ t('ipam.subnets.createSubnet') }}</NButton>
        <NButton @click="loadData">{{ t('common.refresh') }}</NButton>
      </NSpace>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="subnets"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: IPAMSubnet) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <NModal v-if="showModal" v-model:show="showModal" :title="editing ? t('ipam.subnets.editSubnet') : t('ipam.subnets.createSubnet')" preset="card" style="width: 550px;">
      <NForm :model="formData" label-placement="left" label-width="80px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="formData.name" /></NFormItem>
        <NFormItem :label="t('ipam.spaces.title')">
          <NSelect v-model:value="formData.space_id" :options="spaceOptions" />
        </NFormItem>
        <NFormItem :label="t('ipam.subnets.cidr')"><NInput v-model:value="formData.cidr" placeholder="192.168.1.0/24" /></NFormItem>
        <NFormItem :label="t('ipam.subnets.vlanId')"><NInputNumber v-model:value="formData.vlan_id" :min="1" :max="4094" clearable /></NFormItem>
        <NFormItem :label="t('ipam.subnets.location')"><NInput v-model:value="formData.location" /></NFormItem>
        <NFormItem :label="t('common.descriptions')"><NInput v-model:value="formData.description" type="textarea" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('ipam')" @click="handleSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />

    <ImportAddressesDialog v-model:show="showImport" :subnet-id="importSubnetId" :subnet-label="importSubnetLabel" @imported="loadData" />

    <PoolWizardDialog v-model:show="showPool" :subnet-id="poolSubnetId" :subnet-label="poolSubnetLabel" @created="loadData" />

    <ReverseZoneDialog v-model:show="showReverseZone" :subnet-id="reverseZoneSubnetId" :subnet-label="reverseZoneSubnetLabel" @created="loadData" />
  </div>
</template>
