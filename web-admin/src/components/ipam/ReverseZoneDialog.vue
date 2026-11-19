<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import { createDNSZone } from '@/service/api/goddi/dns';
import { planReverseZone, type ReverseZonePlan } from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMReverseZoneDialog' });

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

const loading = ref(false);
const creating = ref(false);
const loadFailed = ref('');
const plan = ref<ReverseZonePlan | null>(null);
const selected = ref('');
const ttl = ref(3600);

const candidateOptions = computed(() =>
  (plan.value?.candidates || []).map(name => ({
    label: name === plan.value?.zone_name ? `${name} (${t('ipam.reverseZone.mostSpecific')})` : name,
    value: name
  }))
);

const canCreate = computed(() => !!plan.value && !!selected.value && perm.canWrite('dns'));

function close() {
  show.value = false;
}

function reset() {
  plan.value = null;
  loadFailed.value = '';
  selected.value = '';
  ttl.value = 3600;
}

async function load() {
  if (!props.subnetId) return;
  loading.value = true;
  loadFailed.value = '';
  try {
    const result = await planReverseZone(props.subnetId);
    plan.value = result;
    // Most specific delegation is preselected, but alternatives stay visible
    // so the choice remains the operator's.
    selected.value = result.zone_name;
  } catch (err: unknown) {
    plan.value = null;
    loadFailed.value = err instanceof Error ? err.message : t('common.failed');
  } finally {
    loading.value = false;
  }
}

async function create() {
  if (!perm.canWrite('dns') || !plan.value || !selected.value) return;
  creating.value = true;
  try {
    await createDNSZone({ name: selected.value, type: 'primary', default_ttl: ttl.value });
    void MessagePlugin.success(t('ipam.reverseZone.created'));
    emit('created');
    close();
  } catch (err: unknown) {
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
  <TDialog
    v-model:visible="show"
    :header="t('ipam.reverseZone.title')"
    :confirm-btn="{ loading: creating, disabled: !canCreate, content: t('ipam.reverseZone.create') }"
    :cancel-btn="t('common.cancel')"
    width="620px"
    @confirm="create"
  >
    <TAlert v-if="subnetLabel" theme="info" :message="`${t('ipam.subnets.title')}: ${subnetLabel}`" class="mb-4" />

    <TLoading :loading="loading" size="small">
      <!-- A plan that failed to load must not look like a plan with no options. -->
      <TAlert v-if="loadFailed" theme="error" :title="t('ipam.reverseZone.loadFailed')" :message="loadFailed" />
      <template v-else-if="plan">
        <TForm label-align="top">
          <TFormItem :label="t('ipam.reverseZone.cidr')">
            <TTag variant="light">{{ plan.cidr }}</TTag>
          </TFormItem>
          <TFormItem :label="t('ipam.reverseZone.candidates')">
            <TSelect v-model="selected" :options="candidateOptions" />
          </TFormItem>
          <TFormItem :label="t('ipam.reverseZone.ttl')">
            <TInputNumber v-model="ttl" :min="60" style="width: 100%" />
          </TFormItem>
        </TForm>

        <p class="hint">{{ t('ipam.reverseZone.candidatesHint') }}</p>
        <!-- Creating a zone needs dns:write; say so instead of failing on click. -->
        <TAlert
          v-if="!perm.canWrite('dns')"
          theme="warning"
          :message="t('ipam.reverseZone.noPermission')"
          class="mt-4"
        />
        <TAlert v-else theme="info" :message="t('ipam.reverseZone.willCreate')" class="mt-4" />
      </template>
    </TLoading>
  </TDialog>
</template>

<style scoped>
.mb-4 {
  margin-bottom: 14px;
}
.mt-4 {
  margin-top: 14px;
}
.ml-2 {
  margin-left: 8px;
}
.candidate-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.hint {
  margin: 10px 0 0;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}
</style>