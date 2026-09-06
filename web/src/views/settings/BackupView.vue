<template>
  <div>
    <page-header :title="t('settings.backup.title')">
      <n-button type="primary" @click="handleCreate">{{ t('settings.backup.createBackup') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="backups"
      :loading="loading"
      :row-key="(row: Backup) => row.id"
    />

    <confirm-dialog
      :show="showRestoreConfirm"
      :message="restoreTarget ? `${t('common.confirmRestore')}\n\n${t('settings.backup.backupName')}: ${backupName(restoreTarget)}\n${t('common.createdAt')}: ${restoreTarget.created_at}\n${t('settings.backup.backupSize')}: ${formatSize(restoreTarget.size_bytes)}` : t('common.confirmRestore')"
      @confirm="handleRestore"
      @cancel="showRestoreConfirm = false"
    />

    <confirm-dialog
      :show="showDeleteConfirm"
      :message="t('common.deleteConfirm')"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { listBackups, createBackup, restoreBackup, deleteBackup, downloadBackup, type Backup } from '@/api/backup'

const { t } = useI18n()
const message = useMessage()

const loading = ref(false)
const backups = ref<Backup[]>([])
const showRestoreConfirm = ref(false)
const showDeleteConfirm = ref(false)
const actionId = ref('')
const restoreTarget = ref<Backup | null>(null)

const columns = [
  { title: () => t('settings.backup.backupName'), key: 'description', render: (row: Backup) => backupName(row) },
  { title: () => t('settings.backup.backupSize'), key: 'size_bytes', render: (row: Backup) => formatSize(row.size_bytes) },
  { title: () => t('settings.backup.backupType'), key: 'type', render: (row: Backup) => h(NTag, { size: 'small' }, { default: () => row.type }) },
  { title: () => t('settings.backup.backupStatus'), key: 'status', render: (row: Backup) => h(NTag, { size: 'small', type: row.status === 'completed' ? 'success' : row.status === 'failed' ? 'error' : 'warning' }, { default: () => row.status }) },
  { title: () => t('common.createdAt'), key: 'created_at', width: 160 },
  { title: () => t('common.actions'), key: 'actions', width: 260, render: (row: Backup) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: row.status !== 'completed', onClick: () => handleDownload(row) }, { default: () => t('settings.backup.downloadBackup') }),
      h(NButton, { size: 'small', text: true, type: 'warning', onClick: () => { actionId.value = row.id; restoreTarget.value = row; showRestoreConfirm.value = true } }, { default: () => t('settings.backup.restoreBackup') }),
      h(NButton, { size: 'small', text: true, type: 'error', onClick: () => { actionId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

function formatSize(bytes: number | undefined | null): string {
  if (bytes == null || isNaN(bytes)) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function backupName(row: Backup): string {
  return row.description || `${row.type}-${row.id.slice(0, 8)}`
}

async function handleDownload(row: Backup) {
  try {
    const { blob, fileName } = await downloadBackup(row.id)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function loadData() {
  loading.value = true
  try { backups.value = (await listBackups()).data } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

async function handleCreate() {
  // The backend requires a description. We use a timestamped default so the
  // user is never blocked by a prompt, while still producing a meaningful
  // name in the backup list. A future UX improvement could open a dialog
  // here to let the user customise the description.
  const description = `manual-${new Date().toISOString().replace(/[:.]/g, '-')}`
  try { await createBackup(description); message.success(t('common.createSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleRestore() {
  try { await restoreBackup(actionId.value); message.success(t('common.success')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showRestoreConfirm.value = false
  restoreTarget.value = null
}

async function handleDelete() {
  try { await deleteBackup(actionId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>
