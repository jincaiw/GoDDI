<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listAPITokens, createAPIToken, deleteAPIToken, type APIToken } from '@/service/api/goddi/user'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const creating = ref(false)
const tokens = ref<APIToken[]>([])
const showCreateModal = ref(false)
const showTokenModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const createdToken = ref('')

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const createForm = reactive<{ name: string; description: string; expires_at: number | null }>({ name: '', description: '', expires_at: null })

const columns = [
  { title: () => t('admin.tokens.tokenName'), key: 'name' },
  { title: () => t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: () => t('admin.users.username'), key: 'username' },
  { title: () => t('admin.tokens.expiresAt'), key: 'expires_at', width: 160 },
  { title: () => t('admin.tokens.lastUsedAt'), key: 'last_used_at', width: 160 },
  { title: () => t('common.actions'), key: 'actions', width: 100, render: (row: APIToken) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('token'), onClick: () => { if (!perm.canDelete('token')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listAPITokens({ page: pagination.page, page_size: pagination.pageSize })
    tokens.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  if (!perm.canWrite('token')) return
  Object.assign(createForm, { name: '', description: '', expires_at: null })
  showCreateModal.value = true
}

async function handleCreate() {
  if (!perm.canWrite('token')) return
  creating.value = true
  try {
    const result = await createAPIToken({
      name: createForm.name,
      description: createForm.description || undefined,
      expires_at: createForm.expires_at ? new Date(createForm.expires_at).toISOString() : undefined,
    })
    createdToken.value = result.token
    showCreateModal.value = false
    showTokenModal.value = true
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { creating.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('token')) return
  try { await deleteAPIToken(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

function copyToken() {
  navigator.clipboard.writeText(createdToken.value)
  message.success('Copied!')
}

onMounted(loadData)
</script>

<template>
  <div>
    <PageHeader :title="t('admin.tokens.title')">
      <NButton v-if="perm.canWrite('token')" type="primary" @click="openCreate">{{ t('admin.tokens.createToken') }}</NButton>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="tokens"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: APIToken) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create Token Modal -->
    <NModal v-if="showCreateModal" v-model:show="showCreateModal" :title="t('admin.tokens.createToken')" preset="card" style="width: 450px;">
      <NForm :model="createForm" label-placement="left" label-width="80px">
        <NFormItem :label="t('admin.tokens.tokenName')"><NInput v-model:value="createForm.name" :disabled="!perm.canWrite('token')" /></NFormItem>
        <NFormItem :label="t('common.descriptions')"><NInput v-model:value="createForm.description" type="textarea" :disabled="!perm.canWrite('token')" /></NFormItem>
        <NFormItem :label="t('admin.tokens.expiresAt')"><NDatePicker v-model:value="createForm.expires_at" type="datetime" clearable style="width: 100%;" :disabled="!perm.canWrite('token')" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCreateModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="creating" :disabled="!perm.canWrite('token')" @click="handleCreate">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Token Created Modal -->
    <NModal v-if="showTokenModal" v-model:show="showTokenModal" :title="t('admin.tokens.tokenCreated')" preset="card" style="width: 500px;" :mask-closable="false">
      <NAlert type="warning" style="margin-bottom: 16px;">{{ t('admin.tokens.tokenWarning') }}</NAlert>
      <NInput :value="createdToken" type="textarea" :rows="3" readonly />
      <template #footer>
        <NSpace justify="end">
          <NButton type="primary" @click="copyToken">{{ t('admin.tokens.copyToken') }}</NButton>
          <NButton @click="showTokenModal = false">{{ t('common.confirm') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>
