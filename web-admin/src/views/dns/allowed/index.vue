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
  if (!perm.canWrite('dns')) return
  try {
    await addAllowRule(form)
    message.success(t('common.createSuccess'))
    showAddModal.value = false
    load()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleDelete(id: string) {
  if (!perm.canDelete('dns')) return
  try { await deleteAllowRule(id); message.success(t('common.deleteSuccess')); load() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleFlush() {
  if (!perm.canDelete('dns')) return
  showFlushConfirm.value = false
  try {
    const res = await flushAllowRules()
    message.success(`${t('dns.security.flushSuccess')} (${res.flushed})`)
    load()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

function triggerImport() {
  if (!perm.canWrite('dns')) return
  importInput.value?.click()
}

async function handleImportFile(e: Event) {
  if (!perm.canWrite('dns')) return
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const text = await file.text()
    const res = await importAllowRules(text)
    const result = res.data
    message.success(`${t('dns.security.importSuccess')} (${result.imported}, skipped ${result.skipped})`)
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

<template>
  <div>
    <PageHeader :title="t('dns.security.allowedTitle')">
      <NSpace>
        <NButton v-if="perm.canRead('dns')" @click="handleExport">{{ t('dns.security.exportRules') }}</NButton>
        <NButton v-if="perm.canWrite('dns')" @click="triggerImport">{{ t('dns.security.importRules') }}</NButton>
        <NButton v-if="perm.canDelete('dns')" type="error" @click="showFlushConfirm = true">{{ t('dns.security.flush') }}</NButton>
        <NButton v-if="perm.canWrite('dns')" type="primary" @click="showAddModal = true">{{ t('dns.security.addAllowRule') }}</NButton>
      </NSpace>
    </PageHeader>

    <NCard>
      <NDataTable :columns="columns" :data="rules" :loading="loading" :row-key="(row: AllowRule) => row.id" />
    </NCard>

    <input ref="importInput" type="file" accept=".txt,.conf,text/plain" style="display: none;" @change="handleImportFile" />

    <NModal v-if="showAddModal" v-model:show="showAddModal" preset="card" :title="t('dns.security.addAllowRule')" style="width: 450px;">
      <NForm :model="form" label-placement="left" label-width="80px">
        <NFormItem :label="t('dns.security.pattern')"><NInput v-model:value="form.pattern" /></NFormItem>
        <NFormItem :label="t('dns.security.matchType')"><NSelect v-model:value="form.match_type" :options="[{ label: 'Exact', value: 'exact' }, { label: 'Suffix', value: 'suffix' }, { label: 'Wildcard', value: 'wildcard' }, { label: 'Regex', value: 'regex' }]" /></NFormItem>
        <NFormItem :label="t('common.enabled')"><NSwitch v-model:value="form.enabled" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showAddModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :disabled="!perm.canWrite('dns')" @click="handleAdd">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog
      :show="showFlushConfirm"
      :message="t('dns.security.flushConfirm')"
      @confirm="handleFlush"
      @cancel="showFlushConfirm = false"
    />
  </div>
</template>
