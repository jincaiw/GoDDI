<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createBackup,
  deleteBackup,
  downloadBackup,
  listBackups,
  restoreBackup,
  type Backup
} from '@/service/api/goddi/backup';

defineOptions({ name: 'BackupSettingsPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const creating = ref(false);

function formatSize(bytes: number | undefined | null): string {
  if (bytes == null || isNaN(bytes)) return '-';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function backupName(row: Backup): string {
  return row.description || `${row.type}-${row.id.slice(0, 8)}`;
}

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

const columns: PrimaryTableCol[] = [
  { colKey: 'description', title: t('settings.backup.backupName'), cell: (_h, { row }) => backupName(row as Backup) },
  { colKey: 'size_bytes', title: t('settings.backup.backupSize'), width: 120, cell: (_h, { row }) => formatSize((row as Backup).size_bytes) },
  {
    colKey: 'type',
    title: t('settings.backup.backupType'),
    width: 120,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as Backup).type))
  },
  {
    colKey: 'status',
    title: t('settings.backup.backupStatus'),
    width: 120,
    cell: (h, { row }) => {
      const s = (row as Backup).status;
      const theme = s === 'completed' ? 'success' : s === 'failed' ? 'danger' : 'warning';
      return h(TTag, { size: 'small', theme, variant: 'light' }, () => String(s));
    }
  },
  { colKey: 'created_at', title: t('common.createdAt'), width: 180 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 260,
    cell: (h, { row }) => {
      const r = row as Backup;
      return h(TSpace, { size: 'small' }, () => [
        h(
          TLink,
          {
            theme: 'primary',
            disabled: !perm.canRead('backup') || r.status !== 'completed',
            onClick: () => void handleDownload(r)
          },
          () => t('settings.backup.downloadBackup')
        ),
        h(
          TLink,
          { theme: 'warning', disabled: !perm.canWrite('backup'), onClick: () => confirmRestore(r) },
          () => t('settings.backup.restoreBackup')
        ),
        h(
          TLink,
          { theme: 'danger', disabled: !perm.canWrite('backup'), onClick: () => confirmDelete(r.id) },
          () => t('common.delete')
        )
      ]);
    }
  }
];

async function handleDownload(row: Backup) {
  if (!perm.canRead('backup')) return;
  try {
    const { blob, fileName } = await downloadBackup(row.id);
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = fileName;
    link.click();
    URL.revokeObjectURL(url);
  } catch (err) {
    msgError(err);
  }
}

async function handleCreate() {
  if (!perm.canWrite('backup') || creating.value) return;
  // The backend requires a description; a timestamped default keeps the user
  // unblocked while still producing a meaningful name in the list.
  creating.value = true;
  try {
    await createBackup(`manual-${new Date().toISOString().replace(/[:.]/g, '-')}`);
    void MessagePlugin.success(t('common.createSuccess'));
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    creating.value = false;
  }
}

function confirmRestore(row: Backup) {
  if (!perm.canWrite('backup')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.confirmRestore'),
    body: `${t('settings.backup.backupName')}: ${backupName(row)}\n${t('common.createdAt')}: ${row.created_at}\n${t('settings.backup.backupSize')}: ${formatSize(row.size_bytes)}`,
    theme: 'warning',
    onConfirm: async () => {
      try {
        await restoreBackup(row.id);
        void MessagePlugin.success(t('common.success'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

function confirmDelete(id: string) {
  if (!perm.canWrite('backup')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteBackup(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('settings.backup.title') }}</h2>
      <TButton v-if="perm.canWrite('backup')" theme="primary" :loading="creating" @click="handleCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('settings.backup.createBackup') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listBackups" :show-pagination="false" />
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
</style>
