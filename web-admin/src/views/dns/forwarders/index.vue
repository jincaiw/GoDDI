<template>
  <div>
    <page-header :title="t('dns.forwarders.title')">
      <n-button v-if="perm.canWrite('dns')" type="primary" @click="openCreateForwarder">{{ t('dns.forwarders.createForwarder') }}</n-button>
    </page-header>

    <n-tabs type="card">
      <n-tab-pane name="forwarders" :tab="t('dns.forwarders.title')">
        <n-data-table :columns="forwarderColumns" :data="forwarders" :loading="loading" :row-key="(row: DNSForwarder) => row.id" />
      </n-tab-pane>
      <n-tab-pane name="conditional" :tab="t('dns.forwarders.conditionalForwarders')">
        <n-space style="margin-bottom: 12px;">
          <n-button v-if="perm.canWrite('dns')" type="primary" @click="showCondModal = true">{{ t('dns.forwarders.createConditional') }}</n-button>
        </n-space>
        <n-data-table :columns="condColumns" :data="conditionals" :loading="condLoading" :row-key="(row: ConditionalForwarder) => row.id" />
      </n-tab-pane>
    </n-tabs>

    <!-- Forwarder Modal -->
    <n-modal v-if="showFwdModal" v-model:show="showFwdModal" preset="card" :title="editingFwd ? t('common.edit') : t('dns.forwarders.createForwarder')" style="width: 450px;">
      <n-form :model="fwdForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('common.name')"><n-input v-model:value="fwdForm.name" /></n-form-item>
        <n-form-item :label="t('dns.forwarders.protocol')">
          <n-select v-model:value="fwdForm.protocol" :options="protocolOptions" />
        </n-form-item>
        <n-form-item :label="t('dns.forwarders.address')"><n-input v-model:value="fwdForm.address" placeholder="8.8.8.8:53" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="fwdForm.enabled" /></n-form-item>
        <n-form-item :label="t('common.priority')"><n-input-number v-model:value="fwdForm.priority" :min="0" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showFwdModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleFwdSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Conditional Forwarder Modal -->
    <n-modal v-if="showCondModal" v-model:show="showCondModal" preset="card" :title="t('dns.forwarders.createConditional')" style="width: 450px;">
      <n-form :model="condForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('dns.forwarders.domain')"><n-input v-model:value="condForm.domain" placeholder="example.com" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="condForm.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showCondModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="condSubmitting" @click="handleCondSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { usePermission } from '@/composables/usePermission'
import {
  listDNSForwarders, createDNSForwarder, updateDNSForwarder, deleteDNSForwarder,
  listConditionalForwarders, createConditionalForwarder, deleteConditionalForwarder,
  type DNSForwarder, type ConditionalForwarder,
} from '@/service/api/goddi/dns'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const forwarders = ref<DNSForwarder[]>([])
const showFwdModal = ref(false)
const editingFwd = ref<DNSForwarder | null>(null)

const condLoading = ref(false)
const condSubmitting = ref(false)
const conditionals = ref<ConditionalForwarder[]>([])
const showCondModal = ref(false)

const protocolOptions = [
  { label: 'UDP', value: 'udp' },
  { label: 'TCP', value: 'tcp' },
  { label: 'DoT', value: 'dot' },
  { label: 'DoH', value: 'doh' },
  { label: 'DoQ', value: 'doq' },
]

const fwdForm = reactive({ name: '', protocol: 'udp', address: '', enabled: true, priority: 0 })
const condForm = reactive({ domain: '', enabled: true })

const forwarderColumns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('dns.forwarders.protocol'), key: 'protocol', render: (row: DNSForwarder) => h(NTag, { size: 'small' }, { default: () => row.protocol.toUpperCase() }) },
  { title: () => t('dns.forwarders.address'), key: 'address' },
  { title: () => t('common.priority'), key: 'priority', width: 80 },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: DNSForwarder) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dns'), onUpdateValue: async (enabled: boolean) => {
    try { await updateDNSForwarder(row.id, { name: row.name, protocol: row.protocol, address: row.address, priority: row.priority, enabled }); await loadForwarders() }
    catch (err) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  } }) },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: DNSForwarder) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, onClick: () => { editingFwd.value = row; Object.assign(fwdForm, row); showFwdModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteFwd(row.id) }, { default: () => t('common.delete') }),
    ],
  }) },
]

const condColumns = [
  { title: () => t('dns.forwarders.domain'), key: 'domain' },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: ConditionalForwarder) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: () => t('common.actions'), key: 'actions', width: 100, render: (row: ConditionalForwarder) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteCond(row.id) }, { default: () => t('common.delete') }) },
]

function openCreateForwarder() {
  editingFwd.value = null
  Object.assign(fwdForm, { name: '', protocol: 'udp', address: '', enabled: true, priority: 0 })
  showFwdModal.value = true
}

async function loadForwarders() {
  loading.value = true
  try {
    const result = await listDNSForwarders()
    forwarders.value = result.data
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    loading.value = false
  }
}

async function handleFwdSubmit() {
  submitting.value = true
  try {
    if (editingFwd.value) {
      await updateDNSForwarder(editingFwd.value.id, fwdForm)
    } else {
      await createDNSForwarder(fwdForm)
    }
    message.success(t('common.success'))
    showFwdModal.value = false
    loadForwarders()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    submitting.value = false
  }
}

async function handleDeleteFwd(id: string) {
  try {
    await deleteDNSForwarder(id)
    message.success(t('common.deleteSuccess'))
    loadForwarders()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function loadConditionals() {
  condLoading.value = true
  try {
    const result = await listConditionalForwarders()
    conditionals.value = result.data
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    condLoading.value = false
  }
}

async function handleCondSubmit() {
  condSubmitting.value = true
  try {
    await createConditionalForwarder(condForm)
    message.success(t('common.createSuccess'))
    showCondModal.value = false
    loadConditionals()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    condSubmitting.value = false
  }
}

async function handleDeleteCond(id: string) {
  try {
    await deleteConditionalForwarder(id)
    message.success(t('common.deleteSuccess'))
    loadConditionals()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

onMounted(() => {
  loadForwarders()
  loadConditionals()
})
</script>
