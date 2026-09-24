<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { usePermission } from '@/composables/usePermission'
import {
  listClientPolicies, createClientPolicy, deleteClientPolicy,
  type ClientPolicy,
} from '@/service/api/goddi/dns'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

// Client Policies
const policyLoading = ref(false)
const policies = ref<ClientPolicy[]>([])
const showPolicyModal = ref(false)
const policyForm = reactive({ name: '', source_cidr: '', action: 'allow', block_list_ids: [] as string[], allow_rule_ids: [] as string[], enabled: true, priority: 0 })

const policyColumns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('dns.security.sourceCidr'), key: 'source_cidr' },
  { title: () => t('dns.security.action'), key: 'action', render: (row: ClientPolicy) => h(NTag, { size: 'small', type: row.action === 'allow' ? 'success' : 'error' }, { default: () => row.action }) },
  { title: () => t('common.priority'), key: 'priority', width: 80 },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: ClientPolicy) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: () => t('common.actions'), key: 'actions', width: 80, render: (row: ClientPolicy) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeletePolicy(row.id) }, { default: () => t('common.delete') }) },
]

async function loadPolicies() {
  policyLoading.value = true
  try { policies.value = (await listClientPolicies()).data } finally { policyLoading.value = false }
}

async function handleCreatePolicy() {
  if (!perm.canWrite('dns')) return
  try {
    await createClientPolicy(policyForm)
    message.success(t('common.createSuccess'))
    showPolicyModal.value = false
    loadPolicies()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleDeletePolicy(id: string) {
  if (!perm.canDelete('dns')) return
  try { await deleteClientPolicy(id); message.success(t('common.deleteSuccess')); loadPolicies() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(loadPolicies)
</script>

<template>
  <div>
    <PageHeader :title="t('dns.security.title')">
      <NSpace>
        <NButton v-if="perm.canWrite('dns')" type="primary" @click="showPolicyModal = true">{{ t('dns.security.createPolicy') }}</NButton>
      </NSpace>
    </PageHeader>

    <NCard>
      <NDataTable :columns="policyColumns" :data="policies" :loading="policyLoading" :row-key="(row: ClientPolicy) => row.id" />
    </NCard>

    <!-- Policy Modal -->
    <NModal v-if="showPolicyModal" v-model:show="showPolicyModal" preset="card" :title="t('dns.security.createPolicy')" style="width: 450px;">
      <NForm :model="policyForm" label-placement="left" label-width="80px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="policyForm.name" /></NFormItem>
        <NFormItem :label="t('dns.security.sourceCidr')"><NInput v-model:value="policyForm.source_cidr" placeholder="192.168.1.0/24" /></NFormItem>
        <NFormItem :label="t('dns.security.action')"><NSelect v-model:value="policyForm.action" :options="[{ label: 'Allow', value: 'allow' }, { label: 'Block', value: 'block' }, { label: 'Apply Lists', value: 'apply_lists' }]" /></NFormItem>
        <NFormItem :label="t('common.enabled')"><NSwitch v-model:value="policyForm.enabled" /></NFormItem>
        <NFormItem :label="t('common.priority')"><NInputNumber v-model:value="policyForm.priority" :min="0" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showPolicyModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :disabled="!perm.canWrite('dns')" @click="handleCreatePolicy">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
