<template>
  <div>
    <page-header :title="t('dns.security.allowedTitle')">
      <n-space>
        <n-button v-if="perm.canWrite('dns')" @click="handleExport">{{ t('dns.security.exportRules') }}</n-button>
        <n-button v-if="perm.canWrite('dns')" @click="triggerImport">{{ t('dns.security.importRules') }}</n-button>
        <n-button v-if="perm.canDelete('dns')" type="error" @click="showFlushConfirm = true">{{ t('dns.security.flush') }}</n-button>
        <n-button v-if="perm.canWrite('dns')" type="primary" @click="showAddModal = true">{{ t('dns.security.addAllowRule') }}</n-button>
      </n-space>
    </page-header>

    <n-card>
      <n-data-table :columns="columns" :data="rules" :loading="loading" :row-key="(row: AllowRule) => row.id" />
    </n-card>

    <input ref="importInput" type="file" accept=".txt,.conf,text/plain" style="display: none;" @change="handleImportFile" />

    <n-modal v-if="showAddModal" v-model:show="showAddModal" preset="card" :title="t('dns.security.addAllowRule')" style="width: 450px;">
      <n-form :model="form" label-placement="left" label-width="80px">
        <n-form-item :label="t('dns.security.pattern')"><n-input v-model:value="form.pattern" /></n-form-item>
        <n-form-item :label="t('dns.security.matchType')"><n-select v-model:value="form.match_type" :options="[{ label: 'Exact', value: 'exact' }, { label: 'Suffix', value: 'suffix' }, { label: 'Wildcard', value: 'wildcard' }, { label: 'Regex', value: 'regex' }]" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="form.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAddModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleAdd">{{ t('common.save') }}</n-button>
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
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import {
  listAllowRules, addAllowRule, deleteAllowRule,
  flushAllowRules, importAllowRules, exportAllowRules,
  type AllowRule,
} from '@/service/api/goddi/dns'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const rules = ref<AllowRule[]>([])
const showAddModal = ref(false)
const showFlushConfirm = ref(false)
const importInput = ref<HTMLInputElement | null>(null)
const form = reactive({ pattern: '', match_type: 'exact', enabled: true })

const columns = [
  { title: () => t('dns.security.pattern'), key: 'pattern' },
  { title: () => t('dns.security.matchType'), key: 'match_type', width: 120, render: (row: AllowRule) => h(NTag, { size: 'small' }, { default: () => row.match_type }) },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: AllowRule) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: () => t('common.actions'), key: 'actions', width: 80, render: (row: AllowRule) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDelete(row.id) }, { default: () => t('common.delete') }) },
]

async function load() {
  loading.value = true
  try { rules.value = (await listAllowRules()).data ?? [] } finally { loading.value = false }
}

async function handleAdd() {
  try {
    await addAllowRule(form)
    message.success(t('common.createSuccess'))
    showAddModal.value = false
    load()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleDelete(id: string) {
  try { await deleteAllowRule(id); message.success(t('common.deleteSuccess')); load() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleFlush() {
  showFlushConfirm.value = false
  try {
    const res = await flushAllowRules()
    message.success(`${t('dns.security.flushSuccess')} (${res.flushed})`)
    load()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

function triggerImport() {
  importInput.value?.click()
}

async function handleImportFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const text = await file.text()
    const res = await importAllowRules(text)
    message.success(`${t('dns.security.importSuccess')} (${res.imported}, skipped ${res.skipped})`)
    load()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally {
    input.value = ''
  }
}

async function handleExport() {
  try {
    const response = await exportAllowRules()
    const url = window.URL.createObjectURL(new Blob([response.data as BlobPart]))
    const link = document.createElement('a')
    link.href = url
    link.download = 'allowlist.txt'
    link.click()
    window.URL.revokeObjectURL(url)
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(load)
</script>
