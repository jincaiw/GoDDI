<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { deleteSession, listSessions, type SessionInfo } from '@/service/api/goddi/auth';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'AdminSessionsPage' });

const { t, locale } = useI18n();
const authStore = useAuthStore();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const revokingOthers = ref(false);

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(locale.value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  });
}

function isCurrentSession(row: SessionInfo) {
  return row.user_id === authStore.userInfo.userId;
}

const columns = computed<PrimaryTableCol[]>(() => [
  { colKey: 'id', title: t('sessions.sessionId'), width: 260, ellipsis: true },
  {
    colKey: 'user_id',
    title: t('sessions.user'),
    width: 150,
    cell: (h, { row }) => {
      const r = row as SessionInfo;
      return h(TTag, { size: 'small', theme: isCurrentSession(r) ? 'primary' : 'success', variant: 'light' }, () =>
        isCurrentSession(r) ? authStore.userInfo.userName || r.user_id : r.user_id
      );
    }
  },
  { colKey: 'ip_address', title: t('sessions.ip'), width: 150 },
  {
    colKey: 'created_at',
    title: t('sessions.createdAt'),
    width: 180,
    cell: (_h, { row }) => formatTime((row as SessionInfo).created_at)
  },
  {
    colKey: 'expires_at',
    title: t('sessions.expiresAt'),
    width: 180,
    cell: (_h, { row }) => formatTime((row as SessionInfo).expires_at)
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) =>
      h(TLink, { theme: 'danger', onClick: () => confirmRevoke((row as SessionInfo).id) }, () => t('sessions.revoke'))
  }
]);

/** The backend lists sessions one page at a time; the total drives the pager. */
async function fetchSessions(params: Record<string, unknown>) {
  const result = await listSessions(params);
  return { data: result.data, meta: { total: result.meta.total } };
}

function confirmRevoke(id: string) {
  const dlg = DialogPlugin.confirm({
    header: t('sessions.revoke'),
    body: t('sessions.revokeConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteSession(id);
        void MessagePlugin.success(t('sessions.revokeSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
      }
      dlg.hide();
    }
  });
}

function confirmRevokeOthers() {
  const dlg = DialogPlugin.confirm({
    header: t('sessions.revokeOthers'),
    body: t('sessions.revokeOthersConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      revokingOthers.value = true;
      try {
        // Keep the most recently created session (assumed to be the current one).
        // The list is paged server-side now, so walk every page: a person
        // clicking "revoke others" means all of them, not just the first page.
        const all: SessionInfo[] = [];
        const pageSize = 100;
        for (let page = 1; ; page += 1) {
          const result = await listSessions({ page, page_size: pageSize });
          all.push(...result.data);
          if (result.data.length < pageSize || all.length >= result.meta.total) break;
        }
        const sorted = [...all].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
        const keepId = sorted[0]?.id;
        for (const target of all.filter(s => s.id !== keepId)) {
          try {
            await deleteSession(target.id);
          } catch {
            // Continue revoking the remaining sessions.
          }
        }
        void MessagePlugin.success(t('sessions.revokeSuccess'));
        await tableRef.value?.reload();
      } finally {
        revokingOthers.value = false;
        dlg.hide();
      }
    }
  });
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('sessions.title') }}</h2>
      <TButton theme="primary" :loading="revokingOthers" @click="confirmRevokeOthers">
        {{ t('sessions.revokeOthers') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="fetchSessions" />
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
