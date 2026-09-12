<template>
  <n-modal
    :show="show"
    preset="card"
    :title="t('ipam.pool.title')"
    style="width: 780px;"
    @update:show="emit('update:show', $event)"
  >
    <n-alert v-if="subnetLabel" type="default" :show-icon="false" style="margin-bottom: 14px;">
      {{ t('ipam.subnets.title') }}: {{ subnetLabel }}
    </n-alert>

    <n-spin :show="loading">
      <!-- A plan that could not be read is not an empty plan. Saying so is the
           difference between "nothing to report" and "we did not look". -->
      <n-alert v-if="loadFailed" type="error" :title="t('ipam.pool.planFailed')">
        {{ loadFailed }}
      </n-alert>

      <template v-else-if="plan">
        <!-- The world moved between the plan and the create. The operator is
             holding an approval of something that is no longer true, so the
             only useful action is to look again. -->
        <n-alert v-if="stale" type="warning" :title="t('ipam.pool.stale')" style="margin-bottom: 16px;">
          {{ t('ipam.pool.staleHint') }}
        </n-alert>

        <n-steps :current="step === 'plan' ? 1 : 2" size="small" style="margin-bottom: 18px;">
          <n-step :title="t('ipam.pool.stepPlan')" />
          <n-step :title="t('ipam.pool.stepConfirm')" />
        </n-steps>

        <!-- Step 1: what IPAM says about the pool this subnet would become. -->
        <template v-if="step === 'plan'">
          <n-alert v-if="plan.conflicts.length > 0" type="error" :title="t('ipam.pool.conflicts')" style="margin-bottom: 14px;">
            <ul style="margin: 0; padding-left: 18px;">
              <li v-for="(conflict, index) in plan.conflicts" :key="index">{{ conflict }}</li>
            </ul>
          </n-alert>

          <n-alert v-if="plan.warnings.length > 0" type="warning" :title="t('ipam.pool.warnings')" style="margin-bottom: 14px;">
            <ul style="margin: 0; padding-left: 18px;">
              <li v-for="(warning, index) in plan.warnings" :key="index">{{ warning }}</li>
            </ul>
          </n-alert>

          <n-descriptions :column="2" bordered size="small" label-placement="left" style="margin-bottom: 18px;">
            <n-descriptions-item :label="t('ipam.subnets.cidr')" :span="2">{{ plan.cidr }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.range')" :span="2">{{ plan.start_ip }} – {{ plan.end_ip }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.detail.router')">
              {{ plan.router || '—' }}
            </n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.routerSource')">{{ routerSourceText }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.leaseTime')">{{ draft?.lease_time ?? '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.fingerprint')">
              <n-text code>{{ shortFingerprint(plan.fingerprint) }}</n-text>
            </n-descriptions-item>
          </n-descriptions>

          <section-title :text="t('ipam.pool.excluded')" :count="plan.excluded.length" />
          <!-- A DHCP scope carries no exclusion list, so this is not a preview
               of what the scope will refuse: it is a list of addresses the
               allocator will still hand out. Reading it as a safety net is the
               mistake the warning below exists to prevent. -->
          <p v-if="plan.excluded.length === 0" class="empty">{{ t('ipam.pool.excludedEmpty') }}</p>
          <template v-else>
            <n-alert type="warning" :show-icon="false" style="margin-bottom: 10px;">
              {{ t('ipam.pool.excludedHint') }}
            </n-alert>
            <n-data-table
              size="small"
              :bordered="false"
              :single-line="false"
              :max-height="220"
              :row-key="(row: PlanAddress) => row.ip"
              :columns="excludedColumns"
              :data="plan.excluded"
            />
          </template>

          <n-form label-placement="left" label-width="110" style="margin-top: 18px;">
            <n-form-item :label="t('common.name')">
              <n-input v-model:value="name" :placeholder="draft?.name || plan.subnet_name" />
            </n-form-item>
          </n-form>
        </template>

        <!-- Step 2: the scope itself, which is what gets approved. -->
        <template v-else>
          <n-alert type="info" :show-icon="false" style="margin-bottom: 14px;">
            {{ t('ipam.pool.confirmHint') }}
          </n-alert>

          <n-descriptions :column="2" bordered size="small" label-placement="left">
            <n-descriptions-item :label="t('common.name')" :span="2">{{ name || draft?.name || plan.subnet_name }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.subnets.cidr')" :span="2">{{ draft?.subnet || plan.cidr }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.range')" :span="2">{{ draft?.start_ip }} – {{ draft?.end_ip }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.mask')">{{ draft?.subnet_mask || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.detail.router')">{{ draft?.router || '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.leaseTime')">{{ draft?.lease_time ?? '—' }}</n-descriptions-item>
            <n-descriptions-item :label="t('ipam.pool.fingerprint')">
              <n-text code>{{ shortFingerprint(plan.fingerprint) }}</n-text>
            </n-descriptions-item>
          </n-descriptions>
        </template>
      </template>
    </n-spin>

    <template #footer>
      <n-space justify="end">
        <n-button v-if="step === 'plan'" @click="close">{{ t('common.cancel') }}</n-button>
        <n-button v-else :disabled="creating" @click="step = 'plan'">{{ t('ipam.pool.back') }}</n-button>

        <!-- Re-planning is offered only when there is something to re-plan:
             a stale approval, or a plan that never loaded. -->
        <n-button v-if="stale || loadFailed" :loading="loading" @click="load()">
          {{ t('ipam.pool.replan') }}
        </n-button>

        <n-button
          v-if="step === 'plan'"
          type="primary"
          :disabled="!plan || hasBlockingErrors || !!loadFailed"
          @click="step = 'confirm'"
        >
          {{ t('ipam.pool.next') }}
        </n-button>
        <n-button v-else type="primary" :loading="creating" @click="create">
          {{ t('ipam.pool.create') }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTag, useMessage } from 'naive-ui'
import SectionTitle from '@/components/ipam/SectionTitle.vue'
import { ApiError } from '@/service/api/goddi/client'
import { getIPAMPoolPlan, type DHCPScopePlan, type PlanAddress, type ScopeDraft } from '@/service/api/goddi/ipam'
import { createDHCPScope } from '@/service/api/goddi/dhcp'

const props = defineProps<{
  show: boolean
  subnetId: string
  subnetLabel: string
}>()

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'created'): void
}>()

const { t } = useI18n()
const message = useMessage()

const step = ref<'plan' | 'confirm'>('plan')
const loading = ref(false)
const creating = ref(false)
const plan = ref<DHCPScopePlan | null>(null)
const draft = ref<ScopeDraft | null>(null)
const name = ref('')
const loadFailed = ref('')
/** True when the create was refused because the plan no longer describes disk. */
const stale = ref(false)

/**
 * A plan with conflicts is a plan the operator should not approve, and the
 * refusal to offer "next" is the point of asking IPAM in the first place. The
 * backend would still accept the create -- it checks the fingerprint, not the
 * conflicts -- so nothing else stops this.
 */
const hasBlockingErrors = computed(() => (plan.value?.conflicts.length ?? 0) > 0)

const routerSourceText = computed(() => {
  switch (plan.value?.router_source) {
    case 'gateway':
      return t('ipam.pool.routerFromGateway')
    case 'conventional':
      return t('ipam.pool.routerFromConvention')
    default:
      return t('ipam.pool.routerFromNothing')
  }
})

/** Enough of a fingerprint to compare two of them by eye, which is all the
 *  operator can do with one; the whole thing goes back on the wire. */
function shortFingerprint(value: string) {
  return value.length > 12 ? value.slice(0, 12) : value
}

const excludedColumns = [
  { title: () => t('common.name'), key: 'ip', width: 150 },
  {
    title: () => t('common.status'),
    key: 'status',
    width: 110,
    render: (row: PlanAddress) => h(NTag, { size: 'small', bordered: false }, { default: () => row.status }),
  },
  { title: () => t('ipam.addresses.hostname'), key: 'hostname', render: (row: PlanAddress) => row.hostname || '—' },
  { title: () => t('ipam.addresses.owner'), key: 'owner', render: (row: PlanAddress) => row.owner || '—' },
]

function close() {
  emit('update:show', false)
}

function reset() {
  step.value = 'plan'
  plan.value = null
  draft.value = null
  name.value = ''
  loadFailed.value = ''
  stale.value = false
}

async function load() {
  if (!props.subnetId) return
  loading.value = true
  loadFailed.value = ''
  stale.value = false
  try {
    const result = await getIPAMPoolPlan(props.subnetId)
    plan.value = result.plan
    draft.value = result.draft
    // The draft's name is the subnet's, which is a sensible default and a
    // poor identifier once two scopes come from the same subnet.
    if (!name.value) name.value = result.draft?.name || result.plan.subnet_name
  } catch (err: unknown) {
    // A plan that failed to load must not look like a plan with nothing in
    // it: the operator would approve an empty answer.
    plan.value = null
    draft.value = null
    loadFailed.value = err instanceof Error ? err.message : t('common.failed')
  } finally {
    loading.value = false
  }
}

async function create() {
  if (!plan.value || !draft.value) return
  creating.value = true
  stale.value = false
  try {
    await createDHCPScope({
      name: name.value || draft.value.name || plan.value.subnet_name,
      subnet: draft.value.subnet || plan.value.cidr,
      start_ip: draft.value.start_ip || plan.value.start_ip,
      end_ip: draft.value.end_ip || plan.value.end_ip,
      subnet_mask: draft.value.subnet_mask,
      router: draft.value.router,
      dns_servers: draft.value.dns_servers,
      ntp_servers: draft.value.ntp_servers,
      domain_name: draft.value.domain_name,
      lease_time: draft.value.lease_time,
      max_lease_time: draft.value.max_lease_time,
      enabled: draft.value.enabled ?? true,
      ping_check_enabled: draft.value.ping_check_enabled,
      dns_updates: draft.value.dns_updates,
      comment: draft.value.comment,
      // The fingerprint the operator approved. The server recomputes the plan
      // and refuses a mismatch, so an address allocated while this dialog was
      // open cannot be handed to a client by a scope built before it existed.
      plan: { subnet_id: plan.value.subnet_id, fingerprint: plan.value.fingerprint },
    })
    message.success(t('ipam.pool.created'))
    emit('created')
    close()
  } catch (err: unknown) {
    // 409 is the server saying the plan is no longer about the world it was
    // built from. Showing the refusal is the feature; retrying silently would
    // apply a scope nobody approved.
    if (err instanceof ApiError && err.status === 409) {
      stale.value = true
      step.value = 'plan'
      message.warning(t('ipam.pool.stale'))
      return
    }
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    creating.value = false
  }
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      reset()
      load()
    }
  },
)
</script>

<style scoped>
.empty {
  margin: 0 0 18px;
  color: var(--text-color-3, #999);
  font-size: 13px;
}
</style>
