<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import SectionTitle from '@/components/ipam/SectionTitle.vue';
import { ApiError } from '@/service/api/goddi/client';
import { getIPAMPoolPlan, type DHCPScopePlan, type PlanAddress, type ScopeDraft } from '@/service/api/goddi/ipam';
import { createDHCPScope } from '@/service/api/goddi/dhcp';

defineOptions({ name: 'IPAMPoolWizardDialog' });

const props = defineProps<{
  subnetId: string;
  subnetLabel: string;
}>();

const emit = defineEmits<{
  (event: 'created'): void;
}>();

// v-model:show is a model ref, the same idiom RemoteTable uses, instead of a
// hand-written props/emit getter-setter pair.
const show = defineModel<boolean>('show', { required: true });

const { t } = useI18n();
const perm = usePermission();

const step = ref<'plan' | 'confirm'>('plan');
const loading = ref(false);
const creating = ref(false);
const plan = ref<DHCPScopePlan | null>(null);
const draft = ref<ScopeDraft | null>(null);
const name = ref('');
const loadFailed = ref('');
/** True when the create was refused because the plan no longer describes disk. */
const stale = ref(false);

/** A conflicting plan cannot advance in the UI; the API enforces this too. */
const hasBlockingErrors = computed(() => (plan.value?.conflicts.length ?? 0) > 0);

const routerSourceText = computed(() => {
  switch (plan.value?.router_source) {
    case 'gateway':
      return t('ipam.pool.routerFromGateway');
    case 'conventional':
      return t('ipam.pool.routerFromConvention');
    default:
      return t('ipam.pool.routerFromNothing');
  }
});

/** The API returns backend-authored prose. Localize the one stable warning
 *  by shape; unknown future warnings stay visible verbatim rather than being
 *  hidden behind a guess. */
function planMessage(text: string) {
  let m = text.match(/^no address is marked gateway, so the plan proposes (\S+) /);
  if (m) return t('ipam.pool.gatewayConventionWarning', { ip: m[1] });
  m = text.match(/^scope "(.+)" already hands out (\S+)\.\.(\S+), which overlaps the proposed (\S+)\.\.(\S+)$/);
  if (m) {
    return t('ipam.pool.scopeOverlapConflict', {
      name: m[1], start: m[2], end: m[3], proposedStart: m[4], proposedEnd: m[5]
    });
  }
  m = text.match(/^scope "(.+)" is disabled but its pool (\S+)\.\.(\S+) overlaps/);
  if (m) {
    return t('ipam.pool.scopeDisabledOverlapWarning', { name: m[1], start: m[2], end: m[3] });
  }
  m = text.match(/^scope "(.+)" already serves subnet (\S+) with a pool that does not overlap this one \((\S+)\.\.(\S+)\)/);
  if (m) {
    return t('ipam.pool.scopeSameSubnetWarning', { name: m[1], subnet: m[2], start: m[3], end: m[4] });
  }
  return text;
}

/** Enough of a fingerprint to compare two of them by eye; the whole value
 *  goes back on the wire. */
function shortFingerprint(value: string) {
  return value.length > 12 ? value.slice(0, 12) : value;
}

const excludedColumns: PrimaryTableCol[] = [
  { colKey: 'ip', title: t('common.name'), width: 150 },
  {
    colKey: 'status',
    title: t('common.status'),
    width: 110,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as PlanAddress).status))
  },
  {
    colKey: 'hostname',
    title: t('ipam.addresses.hostname'),
    cell: (_h, { row }) => (row as PlanAddress).hostname || '—'
  },
  {
    colKey: 'owner',
    title: t('ipam.addresses.owner'),
    cell: (_h, { row }) => (row as PlanAddress).owner || '—'
  }
];

function close() {
  show.value = false;
}

function reset() {
  step.value = 'plan';
  plan.value = null;
  draft.value = null;
  name.value = '';
  loadFailed.value = '';
  stale.value = false;
}

async function load() {
  if (!props.subnetId) return;
  loading.value = true;
  loadFailed.value = '';
  stale.value = false;
  try {
    const result = await getIPAMPoolPlan(props.subnetId);
    plan.value = result.plan;
    draft.value = result.draft;
    // The draft's name is the subnet's; a poor identifier once two scopes
    // come from the same subnet.
    if (!name.value) name.value = result.draft?.name || result.plan.subnet_name;
  } catch (err: unknown) {
    // A plan that failed to load must not look like a plan with nothing in it.
    plan.value = null;
    draft.value = null;
    loadFailed.value = err instanceof Error ? err.message : t('common.failed');
  } finally {
    loading.value = false;
  }
}

async function create() {
  if (!perm.canWrite('dhcp') || !plan.value || !draft.value) return;
  creating.value = true;
  stale.value = false;
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
      // The fingerprint the operator approved; the server recomputes and
      // refuses a mismatch so an address allocated mid-dialog cannot be
      // handed out by a scope built before it existed.
      plan: { subnet_id: plan.value.subnet_id, fingerprint: plan.value.fingerprint }
    });
    void MessagePlugin.success(t('ipam.pool.created'));
    emit('created');
    close();
  } catch (err: unknown) {
    // 409 means the plan is no longer about the world it was built from.
    if (err instanceof ApiError && err.status === 409) {
      stale.value = true;
      step.value = 'plan';
      void MessagePlugin.warning(t('ipam.pool.stale'));
      return;
    }
    void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
  } finally {
    creating.value = false;
  }
}

watch(show, visible => {
  if (visible) {
    reset();
    void load();
  }
});
</script>

<template>
  <TDialog v-model:visible="show" :header="t('ipam.pool.title')" width="780px" :footer="false">
    <TAlert v-if="subnetLabel" theme="info" :message="`${t('ipam.subnets.title')}: ${subnetLabel}`" class="mb-4" />

    <TLoading :loading="loading" size="small">
      <!-- A plan that could not be read is not an empty plan. -->
      <TAlert v-if="loadFailed" theme="error" :title="t('ipam.pool.planFailed')" :message="loadFailed" />

      <template v-else-if="plan">
        <TAlert
          v-if="!perm.canWrite('dhcp')"
          theme="warning"
          :title="t('ipam.pool.noDhcpPermission')"
          :message="t('ipam.pool.noDhcpPermissionHint')"
          class="mb-4"
        />
        <!-- World moved between plan and create; only useful action is look again. -->
        <TAlert v-if="stale" theme="warning" :title="t('ipam.pool.stale')" :message="t('ipam.pool.staleHint')" class="mb-4" />

        <TSteps :current="step === 'plan' ? 1 : 2" size="small" class="mb-4">
          <TStep :title="t('ipam.pool.stepPlan')" />
          <TStep :title="t('ipam.pool.stepConfirm')" />
        </TSteps>

        <!-- Step 1: what IPAM says about the pool this subnet would become. -->
        <template v-if="step === 'plan'">
          <TAlert v-if="plan.conflicts.length > 0" theme="error" :title="t('ipam.pool.conflicts')" class="mb-4">
            <ul class="list">
              <li v-for="(conflict, index) in plan.conflicts" :key="index">{{ planMessage(conflict) }}</li>
            </ul>
          </TAlert>
          <TAlert v-if="plan.warnings.length > 0" theme="warning" :title="t('ipam.pool.warnings')" class="mb-4">
            <ul class="list">
              <li v-for="(warning, index) in plan.warnings" :key="index">{{ planMessage(warning) }}</li>
            </ul>
          </TAlert>

          <TDescriptions :column="2" bordered size="small" class="mb-4">
            <TDescriptionsItem :label="t('ipam.subnets.cidr')" :span="2">{{ plan.cidr }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.range')" :span="2">
              {{ plan.start_ip }} – {{ plan.end_ip }}
            </TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.detail.router')">{{ plan.router || '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.routerSource')">{{ routerSourceText }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.leaseTime')">{{ draft?.lease_time ?? '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.fingerprint')">
              <TTag variant="light">{{ shortFingerprint(plan.fingerprint) }}</TTag>
            </TDescriptionsItem>
          </TDescriptions>

          <SectionTitle :text="t('ipam.pool.excluded')" :count="plan.excluded.length" />
          <!--
 A DHCP scope has no exclusion list: this is what the allocator
               will still hand out, not a safety net.
-->
          <p v-if="plan.excluded.length === 0" class="empty">{{ t('ipam.pool.excludedEmpty') }}</p>
          <template v-else>
            <TAlert theme="warning" :message="t('ipam.pool.excludedHint')" class="mb-2" />
            <TTable
              size="small"
              :columns="excludedColumns"
              :data="plan.excluded"
              row-key="ip"
              :max-height="220"
              stripe
              hover
            />
          </template>

          <TForm label-align="top" class="mt-4">
            <TFormItem :label="t('common.name')">
              <TInput v-model="name" :placeholder="draft?.name || plan.subnet_name" />
            </TFormItem>
          </TForm>
        </template>

        <!-- Step 2: the scope itself, which is what gets approved. -->
        <template v-else>
          <TAlert theme="info" :message="t('ipam.pool.confirmHint')" class="mb-4" />
          <TDescriptions :column="2" bordered size="small">
            <TDescriptionsItem :label="t('common.name')" :span="2">
              {{ name || draft?.name || plan.subnet_name }}
            </TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.subnets.cidr')" :span="2">{{ draft?.subnet || plan.cidr }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.range')" :span="2">
              {{ draft?.start_ip }} – {{ draft?.end_ip }}
            </TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.mask')">{{ draft?.subnet_mask || '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.detail.router')">{{ draft?.router || '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.leaseTime')">{{ draft?.lease_time ?? '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('ipam.pool.fingerprint')">
              <TTag variant="light">{{ shortFingerprint(plan.fingerprint) }}</TTag>
            </TDescriptionsItem>
          </TDescriptions>
        </template>
      </template>
    </TLoading>

    <div class="dialog-actions">
      <TButton v-if="step === 'plan'" variant="outline" @click="close">{{ t('common.cancel') }}</TButton>
      <TButton v-else variant="outline" :disabled="creating" @click="step = 'plan'">{{ t('ipam.pool.back') }}</TButton>

      <!-- Re-planning only when there is something to re-plan. -->
      <TButton v-if="stale || loadFailed" variant="outline" :loading="loading" @click="load()">
        {{ t('ipam.pool.replan') }}
      </TButton>

      <TButton
        v-if="step === 'plan'"
        theme="primary"
        :disabled="!plan || hasBlockingErrors || !!loadFailed"
        @click="step = 'confirm'"
      >
        {{ t('ipam.pool.next') }}
      </TButton>
      <TButton v-else theme="primary" :loading="creating" :disabled="!perm.canWrite('dhcp')" @click="create">
        {{ t('ipam.pool.create') }}
      </TButton>
    </div>
  </TDialog>
</template>

<style scoped>
.mb-4 {
  margin-bottom: 14px;
}
.mb-2 {
  margin-bottom: 10px;
}
.mt-4 {
  margin-top: 18px;
}
.empty {
  margin: 0 0 18px;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}
.list {
  margin: 0;
  padding-left: 18px;
}
.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 20px;
}
</style>