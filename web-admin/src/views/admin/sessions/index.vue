<template>
  <div>
    <page-header :title="t('sessions.title')">
      <n-button type="primary" :loading="revokingOthers" @click="showRevokeOthersConfirm = true">
        {{ t('sessions.revokeOthers') }}
      </n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="sessions"
      :loading="loading"
      :row-key="(row: SessionInfo) => row.id"
    />

    <confirm-dialog
      :show="showRevokeConfirm"
      :message="t('sessions.revokeConfirm')"
      @confirm="handleRevoke"
      @cancel="showRevokeConfirm = false"
    />
    <confirm-dialog
      :show="showRevokeOthersConfirm"
      :message="t('sessions.revokeOthersConfirm')"
      @confirm="handleRevokeOthers"
      @cancel="showRevokeOthersConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { listSessions, deleteSession, type SessionInfo } from '@/service/api/goddi/auth'
import { useAuthStore } from '@/store/modules/auth'

const { t, locale } = useI18n()
const message = useMessage()
const authStore = useAuthStore()

const loading = ref(false)
const revokingOthers = ref(false)
const sessions = ref<SessionInfo[]>([])
const showRevokeConfirm = ref(false)
const showRevokeOthersConfirm = ref(false)
const revokingId = ref('')

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale.value, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false,
  })
}

function isCurrentSession(row: SessionInfo) {
  return row.user_id === authStore.userInfo.userId
}

const columns = computed(() => [
  { title: () => t('sessions.sessionId'), key: 'id', width: 260, ellipsis: { tooltip: true } },
  {
    title: () => t('sessions.user'), key: 'user_id', width: 140,
    render: (row: SessionInfo) => h(NTag, { size: 'small', type: isCurrentSession(row) ? 'info' : 'success', bordered: false }, {
      default: () => (isCurrentSession(row) ? authStore.userInfo.userName || row.user_id : row.user_id),
    }),
  },
  { title: () => t('sessions.ip'), key: 'ip', width: 150 },
  { title: () => t('sessions.createdAt'), key: 'created_at', width: 170, render: (row: SessionInfo) => formatTime(row.created_at) },
  { title: () => t('sessions.expiresAt'), key: 'expires_at', width: 170, render: (row: SessionInfo) => formatTime(row.expires_at) },
  {
    title: () => t('common.actions'), key: 'actions', width: 100,
    render: (row: SessionInfo) => h(NSpace, null, {
      default: () => [
        h(NButton, { size: 'small', text: true, type: 'error', quaternary: true, onClick: () => { revokingId.value = row.id; showRevokeConfirm.value = true } }, { default: () => t('sessions.revoke') }),
      ],
    }),
  },
])

async function loadData() {
  loading.value = true
  try {
    sessions.value = await listSessions()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    loading.value = false
  }
}

async function handleRevoke() {
  try {
    await deleteSession(revokingId.value)
    message.success(t('sessions.revokeSuccess'))
    loadData()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
  showRevokeConfirm.value = false
}

async function handleRevokeOthers() {
  revokingOthers.value = true
  try {
    // Keep the most recently created session (assumed to be the current one).
    const sorted = [...sessions.value].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    const keepId = sorted[0]?.id
    const targets = sessions.value.filter((s) => s.id !== keepId)
    for (const target of targets) {
      try {
        await deleteSession(target.id)
      } catch {
        // Continue revoking the remaining sessions
      }
    }
    message.success(t('sessions.revokeSuccess'))
    loadData()
  } finally {
    revokingOthers.value = false
    showRevokeOthersConfirm.value = false
  }
}

onMounted(loadData)
</script>
