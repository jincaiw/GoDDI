<template>
  <div>
    <page-header :title="t('dns.security.blockedTitle')">
      <n-space>
        <n-button v-if="perm.canDelete('dns')" type="error" @click="showFlushConfirm = true">{{ t('dns.security.flush') }}</n-button>
        <n-button v-if="perm.canWrite('dns')" type="primary" @click="openCreateBlockList">{{ t('dns.security.createBlockList') }}</n-button>
      </n-space>
    </page-header>

    <!-- Temporary disable blocking banner -->
    <n-alert v-if="blockingStatus && blockingStatus.disabled_until" type="warning" style="margin-bottom: 12px;" closable>
      <n-space align="center">
        <span>{{ t('dns.security.blockingDisabledUntil') }} <strong>{{ formatTime(blockingStatus.disabled_until) }}</strong></span>
        <n-button v-if="perm.canWrite('dns')" size="small" @click="handleResumeBlocking">{{ t('dns.security.resumeBlocking') }}</n-button>
      </n-space>
    </n-alert>
    <n-card v-else size="small" style="margin-bottom: 12px;">
      <n-space align="center">
        <span>{{ t('dns.security.temporaryDisable') }}</span>
        <n-select v-model:value="disableMinutes" :options="disableMinutesOptions" style="width: 140px;" />
        <n-button v-if="perm.canWrite('dns')" size="small" type="warning" @click="handleTemporaryDisable">{{ t('dns.security.disableNow') }}</n-button>
      </n-space>
    </n-card>

    <n-card>
      <n-data-table :columns="blockListColumns" :data="blockLists" :loading="blockListLoading" :row-key="(row: BlockList) => row.id" />

      <n-card v-if="selectedBlockList" :title="`${selectedBlockList.name} - Rules`" style="margin-top: 16px;">
        <template #header-extra>
          <n-button v-if="perm.canWrite('dns')" size="small" @click="showBlockRuleModal = true">{{ t('dns.security.addBlockRule') }}</n-button>
        </template>
        <n-data-table :columns="blockRuleColumns" :data="blockRules" :loading="blockRuleLoading" :row-key="(row: BlockRule) => row.id" size="small" />
      </n-card>
    </n-card>

    <!-- Block List Modal -->
    <n-modal v-if="showBlockListModal" v-model:show="showBlockListModal" preset="card" :title="t('dns.security.createBlockList')" style="width: 450px;">
      <n-form :model="blockListForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('common.name')"><n-input v-model:value="blockListForm.name" /></n-form-item>
        <n-form-item :label="t('common.type')"><n-select v-model:value="blockListForm.type" :options="[{ label: 'Custom', value: 'custom' }, { label: 'External', value: 'external' }]" /></n-form-item>
        <n-form-item v-if="blockListForm.type === 'external'" label="URL"><n-input v-model:value="blockListForm.url" placeholder="https://example.com/blocklist.txt" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="blockListForm.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showBlockListModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="blockListSubmitting" @click="handleCreateBlockList">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Block Rule Modal -->
    <n-modal v-if="showBlockRuleModal" v-model:show="showBlockRuleModal" preset="card" :title="t('dns.security.addBlockRule')" style="width: 450px;">
      <n-form :model="blockRuleForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('dns.security.pattern')"><n-input v-model:value="blockRuleForm.pattern" /></n-form-item>
        <n-form-item :label="t('dns.security.matchType')"><n-select v-model:value="blockRuleForm.match_type" :options="[{ label: 'Exact', value: 'exact' }, { label: 'Suffix', value: 'suffix' }, { label: 'Wildcard', value: 'wildcard' }, { label: 'Regex', value: 'regex' }]" /></n-form-item>
        <n-form-item :label="t('dns.security.responseType')"><n-select v-model:value="blockRuleForm.response_type" :options="[{ label: 'NXDOMAIN', value: 'NXDOMAIN' }, { label: 'NODATA', value: 'NODATA' }, { label: 'REFUSED', value: 'REFUSED' }, { label: 'DROP', value: 'DROP' }, { label: 'Custom IP', value: 'CUSTOM_IP' }]" /></n-form-item>
        <n-form-item v-if="blockRuleForm.response_type === 'CUSTOM_IP'" label="IP"><n-input v-model:value="blockRuleForm.response_data" placeholder="0.0.0.0" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="blockRuleForm.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showBlockRuleModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleAddBlockRule">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog
      :show="showFlushConfirm"
      :message="t('dns.security.flushConfirm')"
      @confirm="handleFlush"
      @cancel="showFlushConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import {
  listBlockLists, createBlockList, deleteBlockList, listBlockRules, addBlockRule, deleteBlockRule,
  flushBlockLists,
  getBlockingStatus, temporaryDisableBlocking, refreshBlockList,
  type BlockList, type BlockRule, type BlockingStatus,
} from '@/service/api/goddi/dns'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

// Block Lists
const blockListLoading = ref(false)
const blockListSubmitting = ref(false)
const blockLists = ref<BlockList[]>([])
const selectedBlockList = ref<BlockList | null>(null)
const showBlockListModal = ref(false)
const showBlockRuleModal = ref(false)
const showFlushConfirm = ref(false)
const blockListForm = reactive({ name: '', type: 'custom', url: '', enabled: true })
const blockRuleForm = reactive({ pattern: '', match_type: 'suffix', response_type: 'NXDOMAIN', response_data: '', enabled: true })

// Temporary disable blocking
const blockingStatus = ref<BlockingStatus | null>(null)
const disableMinutes = ref<number>(10)
const disableMinutesOptions = [5, 10, 15, 30, 60, 120, 240].map(m => ({ label: `${m} min`, value: m }))
const refreshingId = ref<string | null>(null)
let statusTimer: ReturnType<typeof setInterval> | null = null

function formatTime(value: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  return isNaN(d.getTime()) ? value : d.toLocaleString()
}

async function loadBlockingStatus() {
  try { blockingStatus.value = await getBlockingStatus() } catch { /* ignore */ }
}

async function handleTemporaryDisable() {
  try {
    blockingStatus.value = await temporaryDisableBlocking(disableMinutes.value)
    message.success(t('dns.security.disableSuccess'))
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleResumeBlocking() {
  try {
    await temporaryDisableBlocking(0)
    blockingStatus.value = await getBlockingStatus()
    message.success(t('dns.security.resumeSuccess'))
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleRefreshBlockList(id: string) {
  refreshingId.value = id
  try {
    await refreshBlockList(id)
    message.success(t('dns.security.refreshSuccess'))
    loadBlockLists()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { refreshingId.value = null }
}

const blockListColumns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('common.type'), key: 'type', render: (row: BlockList) => h(NTag, { size: 'small' }, { default: () => row.type }) },
  { title: 'Rules', key: 'entry_count', width: 80 },
  { title: () => t('dns.security.lastFetch'), key: 'last_fetch', width: 140, render: (row: BlockList) => {
    if (row.type !== 'external') return '-'
    if (!row.last_fetch_at) return h(NTag, { size: 'small' }, { default: () => t('dns.security.neverFetched') })
    const status = row.last_fetch_status === 'ok'
      ? h(NTag, { size: 'small', type: 'success' }, { default: () => 'OK' })
      : h(NTag, { size: 'small', type: 'error' }, { default: () => t('dns.security.fetchFailed') })
    return h(NSpace, { size: 4, align: 'center' }, { default: () => [status, h('span', { style: 'font-size: 12px; color: var(--n-text-color-3, #888);' }, formatTime(row.last_fetch_at!))] })
  } },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: BlockList) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: () => t('common.actions'), key: 'actions', width: 220, render: (row: BlockList) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, onClick: () => { selectedBlockList.value = row; loadBlockRules(row.id) } }, { default: () => 'Rules' }),
      ...(row.type === 'external' ? [
        h(NButton, { size: 'small', text: true, type: 'primary', loading: refreshingId.value === row.id, disabled: !perm.canWrite('dns'), onClick: () => handleRefreshBlockList(row.id) }, { default: () => t('dns.security.refresh') }),
      ] : []),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteBlockList(row.id) }, { default: () => t('common.delete') }),
    ],
  }) },
]

// Block Rules
const blockRuleLoading = ref(false)
const blockRules = ref<BlockRule[]>([])

const blockRuleColumns = [
  { title: () => t('dns.security.pattern'), key: 'pattern' },
  { title: () => t('dns.security.matchType'), key: 'match_type', render: (row: BlockRule) => h(NTag, { size: 'small' }, { default: () => row.match_type }) },
  { title: () => t('dns.security.responseType'), key: 'response_type', render: (row: BlockRule) => h(NTag, { size: 'small', type: row.response_type === 'DROP' ? 'error' : 'warning' }, { default: () => row.response_type }) },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: BlockRule) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: () => t('common.actions'), key: 'actions', width: 80, render: (row: BlockRule) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteBlockRule(row.list_id, row.id) }, { default: () => t('common.delete') }) },
]

// Load functions
async function loadBlockLists() {
  blockListLoading.value = true
  try { blockLists.value = (await listBlockLists()).data } finally { blockListLoading.value = false }
}

async function loadBlockRules(listId: string) {
  blockRuleLoading.value = true
  try { blockRules.value = (await listBlockRules(listId)).data } finally { blockRuleLoading.value = false }
}

// Handlers
function openCreateBlockList() {
  Object.assign(blockListForm, { name: '', type: 'custom', url: '', enabled: true })
  showBlockListModal.value = true
}

async function handleCreateBlockList() {
  blockListSubmitting.value = true
  try {
    await createBlockList(blockListForm)
    message.success(t('common.createSuccess'))
    showBlockListModal.value = false
    loadBlockLists()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { blockListSubmitting.value = false }
}

async function handleDeleteBlockList(id: string) {
  try { await deleteBlockList(id); message.success(t('common.deleteSuccess')); loadBlockLists() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleAddBlockRule() {
  if (!selectedBlockList.value) return
  try {
    await addBlockRule(selectedBlockList.value.id, blockRuleForm)
    message.success(t('common.createSuccess'))
    showBlockRuleModal.value = false
    loadBlockRules(selectedBlockList.value.id)
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleDeleteBlockRule(listId: string, ruleId: string) {
  try { await deleteBlockRule(listId, ruleId); message.success(t('common.deleteSuccess')); if (selectedBlockList.value) loadBlockRules(selectedBlockList.value.id) } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleFlush() {
  showFlushConfirm.value = false
  try {
    const res = await flushBlockLists()
    message.success(`${t('dns.security.flushSuccess')} (${res.flushed})`)
    selectedBlockList.value = null
    loadBlockLists()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => {
  loadBlockLists()
  loadBlockingStatus()
  statusTimer = setInterval(loadBlockingStatus, 30000)
})

onUnmounted(() => {
  if (statusTimer) { clearInterval(statusTimer); statusTimer = null }
})
</script>
