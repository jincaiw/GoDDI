<script setup lang="ts">
import { ref, reactive, computed, h, onMounted } from 'vue'
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

// Per-protocol address hints (DoT/DoQ default to :853; DoH is a full URL).
const addressPlaceholders: Record<string, string> = {
  udp: '8.8.8.8:53',
  tcp: '8.8.8.8:53',
  dot: 'dns.quad9.net:853',
  doh: 'https://dns.quad9.net/dns-query',
  doq: 'dns.adguard-dns.com:853',
}
const addressPlaceholder = computed(() => addressPlaceholders[fwdForm.protocol] ?? '8.8.8.8:53')

const forwarderColumns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('dns.forwarders.protocol'), key: 'protocol', render: (row: DNSForwarder) => h(NTag, { size: 'small' }, { default: () => row.protocol.toUpperCase() }) },
  { title: () => t('dns.forwarders.address'), key: 'address' },
  { title: () => t('common.priority'), key: 'priority', width: 80 },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: DNSForwarder) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dns'), onUpdateValue: async (enabled: boolean) => {
    if (!perm.canWrite('dns')) return
    try { await updateDNSForwarder(row.id, { name: row.name, protocol: row.protocol, address: row.address, priority: row.priority, enabled }); await loadForwarders() }
    catch (err) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  } }) },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: DNSForwarder) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('dns'), onClick: () => { if (!perm.canWrite('dns')) return; editingFwd.value = row; Object.assign(fwdForm, row); showFwdModal.value = true } }, { default: () => t('common.edit') }),
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
  if (!perm.canWrite('dns')) return
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
  if (!perm.canWrite('dns')) return
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
  if (!perm.canDelete('dns')) return
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
  if (!perm.canWrite('dns')) return
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
  if (!perm.canDelete('dns')) return
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

<template>
  <div>
    <PageHeader :title="t('dns.forwarders.title')">
      <NButton v-if="perm.canWrite('dns')" type="primary" @click="openCreateForwarder">{{ t('dns.forwarders.createForwarder') }}</NButton>
    </PageHeader>

    <NTabs type="card">
      <NTabPane name="forwarders" :tab="t('dns.forwarders.title')">
        <NDataTable :columns="forwarderColumns" :data="forwarders" :loading="loading" :row-key="(row: DNSForwarder) => row.id" />
      </NTabPane>
      <NTabPane name="conditional" :tab="t('dns.forwarders.conditionalForwarders')">
        <NSpace style="margin-bottom: 12px;">
          <NButton v-if="perm.canWrite('dns')" type="primary" @click="showCondModal = true">{{ t('dns.forwarders.createConditional') }}</NButton>
        </NSpace>
        <NDataTable :columns="condColumns" :data="conditionals" :loading="condLoading" :row-key="(row: ConditionalForwarder) => row.id" />
      </NTabPane>
    </NTabs>

    <!-- Forwarder Modal -->
    <NModal v-if="showFwdModal" v-model:show="showFwdModal" preset="card" :title="editingFwd ? t('common.edit') : t('dns.forwarders.createForwarder')" style="width: 450px;">
      <NForm :model="fwdForm" label-placement="left" label-width="80px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="fwdForm.name" /></NFormItem>
        <NFormItem :label="t('dns.forwarders.protocol')">
          <NSelect v-model:value="fwdForm.protocol" :options="protocolOptions" />
        </NFormItem>
        <NFormItem :label="t('dns.forwarders.address')"><NInput v-model:value="fwdForm.address" :placeholder="addressPlaceholder" /></NFormItem>
        <NFormItem :label="t('common.enabled')"><NSwitch v-model:value="fwdForm.enabled" /></NFormItem>
        <NFormItem :label="t('common.priority')"><NInputNumber v-model:value="fwdForm.priority" :min="0" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showFwdModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('dns')" @click="handleFwdSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Conditional Forwarder Modal -->
    <NModal v-if="showCondModal" v-model:show="showCondModal" preset="card" :title="t('dns.forwarders.createConditional')" style="width: 450px;">
      <NForm :model="condForm" label-placement="left" label-width="80px">
        <NFormItem :label="t('dns.forwarders.domain')"><NInput v-model:value="condForm.domain" placeholder="example.com" /></NFormItem>
        <NFormItem :label="t('common.enabled')"><NSwitch v-model:value="condForm.enabled" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCondModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="condSubmitting" :disabled="!perm.canWrite('dns')" @click="handleCondSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
