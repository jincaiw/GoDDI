<template>
  <div>
    <page-header :title="t('dns.security.title')" />

    <n-tabs type="card">
      <!-- Block Lists Tab -->
      <n-tab-pane name="blocklists" :tab="t('dns.security.blockLists')">
        <n-space style="margin-bottom: 12px;">
          <n-button v-if="perm.canWrite('dns')" type="primary" @click="openCreateBlockList">{{ t('dns.security.createBlockList') }}</n-button>
        </n-space>
        <n-data-table :columns="blockListColumns" :data="blockLists" :loading="blockListLoading" :row-key="(row: BlockList) => row.id" />

        <!-- Block Rules for selected list -->
        <n-card v-if="selectedBlockList" :title="`${selectedBlockList.name} - Rules`" style="margin-top: 16px;">
          <template #header-extra>
            <n-button v-if="perm.canWrite('dns')" size="small" @click="showBlockRuleModal = true">{{ t('dns.security.addBlockRule') }}</n-button>
          </template>
          <n-data-table :columns="blockRuleColumns" :data="blockRules" :loading="blockRuleLoading" :row-key="(row: BlockRule) => row.id" size="small" />
        </n-card>
      </n-tab-pane>

      <!-- Allow Rules Tab -->
      <n-tab-pane name="allow" :tab="t('dns.security.allowRules')">
        <n-space style="margin-bottom: 12px;">
          <n-button v-if="perm.canWrite('dns')" type="primary" @click="showAllowRuleModal = true">{{ t('dns.security.addAllowRule') }}</n-button>
        </n-space>
        <n-data-table :columns="allowRuleColumns" :data="allowRules" :loading="allowRuleLoading" :row-key="(row: AllowRule) => row.id" />
      </n-tab-pane>

      <!-- Client Policies Tab -->
      <n-tab-pane name="policies" :tab="t('dns.security.clientPolicies')">
        <n-space style="margin-bottom: 12px;">
          <n-button v-if="perm.canWrite('dns')" type="primary" @click="showPolicyModal = true">{{ t('dns.security.createPolicy') }}</n-button>
        </n-space>
        <n-data-table :columns="policyColumns" :data="policies" :loading="policyLoading" :row-key="(row: ClientPolicy) => row.id" />
      </n-tab-pane>
    </n-tabs>

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

    <!-- Allow Rule Modal -->
    <n-modal v-if="showAllowRuleModal" v-model:show="showAllowRuleModal" preset="card" :title="t('dns.security.addAllowRule')" style="width: 450px;">
      <n-form :model="allowRuleForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('dns.security.pattern')"><n-input v-model:value="allowRuleForm.pattern" /></n-form-item>
        <n-form-item :label="t('dns.security.matchType')"><n-select v-model:value="allowRuleForm.match_type" :options="[{ label: 'Exact', value: 'exact' }, { label: 'Suffix', value: 'suffix' }, { label: 'Wildcard', value: 'wildcard' }, { label: 'Regex', value: 'regex' }]" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="allowRuleForm.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAllowRuleModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleAddAllowRule">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Policy Modal -->
    <n-modal v-if="showPolicyModal" v-model:show="showPolicyModal" preset="card" :title="t('dns.security.createPolicy')" style="width: 450px;">
      <n-form :model="policyForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('common.name')"><n-input v-model:value="policyForm.name" /></n-form-item>
        <n-form-item :label="t('dns.security.sourceCidr')"><n-input v-model:value="policyForm.source_cidr" placeholder="192.168.1.0/24" /></n-form-item>
        <n-form-item :label="t('dns.security.action')"><n-select v-model:value="policyForm.action" :options="[{ label: 'Allow', value: 'allow' }, { label: 'Block', value: 'block' }, { label: 'Apply Lists', value: 'apply_lists' }]" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="policyForm.enabled" /></n-form-item>
        <n-form-item :label="t('common.priority')"><n-input-number v-model:value="policyForm.priority" :min="0" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showPolicyModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleCreatePolicy">{{ t('common.save') }}</n-button>
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
  listBlockLists, createBlockList, deleteBlockList, listBlockRules, addBlockRule, deleteBlockRule,
  listAllowRules, addAllowRule, deleteAllowRule,
  listClientPolicies, createClientPolicy, deleteClientPolicy,
  type BlockList, type BlockRule, type AllowRule, type ClientPolicy,
} from '@/api/dns'

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
const blockListForm = reactive({ name: '', type: 'custom', url: '', enabled: true })
const blockRuleForm = reactive({ pattern: '', match_type: 'suffix', response_type: 'NXDOMAIN', response_data: '', enabled: true })

const blockListColumns = [
  { title: t('common.name'), key: 'name' },
  { title: t('common.type'), key: 'type', render: (row: BlockList) => h(NTag, { size: 'small' }, { default: () => row.type }) },
  { title: 'Rules', key: 'entry_count', width: 80 },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: BlockList) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: t('common.actions'), key: 'actions', width: 160, render: (row: BlockList) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { selectedBlockList.value = row; loadBlockRules(row.id) } }, { default: () => 'Rules' }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteBlockList(row.id) }, { default: () => t('common.delete') }),
    ],
  }) },
]

// Block Rules
const blockRuleLoading = ref(false)
const blockRules = ref<BlockRule[]>([])

const blockRuleColumns = [
  { title: t('dns.security.pattern'), key: 'pattern' },
  { title: t('dns.security.matchType'), key: 'match_type', render: (row: BlockRule) => h(NTag, { size: 'small' }, { default: () => row.match_type }) },
  { title: t('dns.security.responseType'), key: 'response_type', render: (row: BlockRule) => h(NTag, { size: 'small', type: row.response_type === 'DROP' ? 'error' : 'warning' }, { default: () => row.response_type }) },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: BlockRule) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: t('common.actions'), key: 'actions', width: 80, render: (row: BlockRule) => h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteBlockRule(row.list_id, row.id) }, { default: () => t('common.delete') }) },
]

// Allow Rules
const allowRuleLoading = ref(false)
const allowRules = ref<AllowRule[]>([])
const showAllowRuleModal = ref(false)
const allowRuleForm = reactive({ pattern: '', match_type: 'exact', enabled: true })

const allowRuleColumns = [
  { title: t('dns.security.pattern'), key: 'pattern' },
  { title: t('dns.security.matchType'), key: 'match_type', render: (row: AllowRule) => h(NTag, { size: 'small' }, { default: () => row.match_type }) },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: AllowRule) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: t('common.actions'), key: 'actions', width: 80, render: (row: AllowRule) => h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeleteAllowRule(row.id) }, { default: () => t('common.delete') }) },
]

// Client Policies
const policyLoading = ref(false)
const policies = ref<ClientPolicy[]>([])
const showPolicyModal = ref(false)
const policyForm = reactive({ name: '', source_cidr: '', action: 'allow', block_list_ids: [] as string[], allow_rule_ids: [] as string[], enabled: true, priority: 0 })

const policyColumns = [
  { title: t('common.name'), key: 'name' },
  { title: t('dns.security.sourceCidr'), key: 'source_cidr' },
  { title: t('dns.security.action'), key: 'action', render: (row: ClientPolicy) => h(NTag, { size: 'small', type: row.action === 'allow' ? 'success' : 'error' }, { default: () => row.action }) },
  { title: t('common.priority'), key: 'priority', width: 80 },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: ClientPolicy) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: t('common.actions'), key: 'actions', width: 80, render: (row: ClientPolicy) => h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dns'), onClick: () => handleDeletePolicy(row.id) }, { default: () => t('common.delete') }) },
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

async function loadAllowRules() {
  allowRuleLoading.value = true
  try { allowRules.value = (await listAllowRules()).data } finally { allowRuleLoading.value = false }
}

async function loadPolicies() {
  policyLoading.value = true
  try { policies.value = (await listClientPolicies()).data } finally { policyLoading.value = false }
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

async function handleAddAllowRule() {
  try {
    await addAllowRule(allowRuleForm)
    message.success(t('common.createSuccess'))
    showAllowRuleModal.value = false
    loadAllowRules()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleDeleteAllowRule(id: string) {
  try { await deleteAllowRule(id); message.success(t('common.deleteSuccess')); loadAllowRules() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleCreatePolicy() {
  try {
    await createClientPolicy(policyForm)
    message.success(t('common.createSuccess'))
    showPolicyModal.value = false
    loadPolicies()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleDeletePolicy(id: string) {
  try { await deleteClientPolicy(id); message.success(t('common.deleteSuccess')); loadPolicies() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => {
  loadBlockLists()
  loadAllowRules()
  loadPolicies()
})
</script>
