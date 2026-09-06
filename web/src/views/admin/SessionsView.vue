<template>
  <div>
    <page-header :title="t('admin.sessions.title')" />

    <n-data-table
      :columns="columns"
      :data="sessions"
      :loading="loading"
      :bordered="false"
      size="small"
      :row-key="(row: SessionInfo) => row.id"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { listSessions, deleteSession, type SessionInfo } from '@/api/auth'

const { t, locale } = useI18n()
const message = useMessage()

const sessions = ref<SessionInfo[]>([])
const loading = ref(false)

const columns = computed(() => [
  {
    title: () => t('admin.sessions.ip'),
    key: 'ip_address',
    width: 160,
  },
  {
    title: () => t('admin.sessions.userAgent'),
    key: 'user_agent',
    ellipsis: { tooltip: true },
  },
  {
    title: () => t('common.createdAt'),
    key: 'created_at',
    width: 180,
    render: (row: SessionInfo) => formatTime(row.created_at),
  },
  {
    title: () => t('admin.sessions.expiresAt'),
    key: 'expires_at',
    width: 180,
    render: (row: SessionInfo) => formatTime(row.expires_at),
  },
  {
    title: () => t('common.actions'),
    key: 'actions',
    width: 100,
    render: (row: SessionInfo) =>
      h(NSpace, null, () =>
        h(
          NButton,
          { size: 'small', type: 'error', secondary: true, onClick: () => handleRevoke(row) },
          () => t('admin.sessions.revoke')
        )
      ),
  },
])

function formatTime(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

async function load() {
  loading.value = true
  try {
    sessions.value = await listSessions()
  } catch {
    message.error(t('common.failed'))
  } finally {
    loading.value = false
  }
}

async function handleRevoke(row: SessionInfo) {
  try {
    await deleteSession(row.id)
    message.success(t('admin.sessions.revoked'))
    await load()
  } catch {
    message.error(t('common.failed'))
  }
}

onMounted(load)
</script>
