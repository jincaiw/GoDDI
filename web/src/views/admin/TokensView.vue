<template>
  <div>
    <page-header :title="t('admin.tokens.title')">
      <n-button v-if="perm.canWrite('token')" type="primary" @click="openCreate">{{ t('admin.tokens.createToken') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="tokens"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: APIToken) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create Token Modal -->
    <n-modal v-if="showCreateModal" v-model:show="showCreateModal" :title="t('admin.tokens.createToken')" preset="card" style="width: 450px;">
      <n-form :model="createForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('admin.tokens.tokenName')"><n-input v-model:value="createForm.name" /></n-form-item>
        <n-form-item :label="t('common.descriptions')"><n-input v-model:value="createForm.description" type="textarea" /></n-form-item>
        <n-form-item :label="t('admin.tokens.expiresAt')"><n-date-picker v-model:value="createForm.expires_at" type="datetime" clearable style="width: 100%;" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showCreateModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="creating" @click="handleCreate">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Token Created Modal -->
    <n-modal v-if="showTokenModal" v-model:show="showTokenModal" :title="t('admin.tokens.tokenCreated')" preset="card" style="width: 500px;" :mask-closable="false">
      <n-alert type="warning" style="margin-bottom: 16px;">{{ t('admin.tokens.tokenWarning') }}</n-alert>
      <n-input :value="createdToken" type="textarea" :rows="3" readonly />
      <template #footer>
        <n-space justify="end">
          <n-button type="primary" @click="copyToken">{{ t('admin.tokens.copyToken') }}</n-button>
          <n-button @click="showTokenModal = false">{{ t('common.confirm') }}</n-button>
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
import { listAPITokens, createAPIToken, deleteAPIToken, type APIToken } from '@/api/user'

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
  { title: t('admin.tokens.tokenName'), key: 'name' },
  { title: t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: t('admin.users.username'), key: 'username' },
  { title: t('admin.tokens.expiresAt'), key: 'expires_at', width: 160 },
  { title: t('admin.tokens.lastUsedAt'), key: 'last_used_at', width: 160 },
  { title: t('common.actions'), key: 'actions', width: 100, render: (row: APIToken) => h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('token'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }) },
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
  Object.assign(createForm, { name: '', description: '', expires_at: null })
  showCreateModal.value = true
}

async function handleCreate() {
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
  try { await deleteAPIToken(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

function copyToken() {
  navigator.clipboard.writeText(createdToken.value)
  message.success('Copied!')
}

onMounted(loadData)
</script>
