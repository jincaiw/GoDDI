<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { formatAPITime } from '@/utils/datetime';
import { deleteSession, listSessions, type SessionInfo } from '@/service/api/goddi/auth';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'AdminSessionsPage' });

/** Starter CRUD page: TCard header + shared remote TTable + TDialog/TForm. */

const { t } = useI18n();
const authStore = useAuthStore();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const revokingOthers = ref(false);

function formatTime(value: string | null | undefined): string {
  return formatAPITime(value);
}

function isCurrentSession(row: SessionInfo) {
  return Boolean(authStore.currentSessionId) && row.id === authStore.currentSessionId;
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
      h(
        TLink,
        {
          theme: 'danger',
          disabled: isCurrentSession(row as SessionInfo),
          onClick: () => confirmRevoke((row as SessionInfo).id)
        },
        () => t('sessions.revoke')
      )
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
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
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
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
    onConfirm: async () => {
      revokingOthers.value = true;
      try {
        // Keep the session that owns the current access token. Walking every
        // page still matters: one click means all other sessions, not page one.
        const all: SessionInfo[] = [];
        const pageSize = 100;
        for (let page = 1; ; page += 1) {
          const result = await listSessions({ page, page_size: pageSize });
          all.push(...result.data);
          if (result.data.length < pageSize || all.length >= result.meta.total) break;
        }
        const keepId = authStore.currentSessionId;
        if (!keepId || !all.some(s => s.id === keepId)) {
          void MessagePlugin.error(t('common.failed'));
          return;
        }
        const targets = all.filter(s => s.id !== keepId);
        let succeeded = 0;
        for (const target of targets) {
          try {
            await deleteSession(target.id);
            succeeded += 1;
          } catch {
            // Keep going: one failed revoke must not hide the others' status.
          }
        }
        const failed = targets.length - succeeded;
        if (failed === 0) {
          void MessagePlugin.success(t('sessions.revokeSuccess'));
        } else if (succeeded === 0) {
          void MessagePlugin.error(t('sessions.revokeFailed'));
        } else {
          void MessagePlugin.warning(t('sessions.revokePartial', { ok: succeeded, failed }));
        }
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
      <div class="td-page-title">
        <h2>{{ t('sessions.title') }}</h2>
      </div>
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
  gap: 16px;
  padding: 16px 20px;
}
</style>