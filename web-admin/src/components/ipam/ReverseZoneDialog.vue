<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import { usePermission } from '@/composables/usePermission'
import { createDNSZone } from '@/service/api/goddi/dns'
import { planReverseZone, type ReverseZonePlan } from '@/service/api/goddi/ipam'

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
const perm = usePermission()

const loading = ref(false)
const creating = ref(false)
const loadFailed = ref('')
const plan = ref<ReverseZonePlan | null>(null)
const selected = ref('')
const ttl = ref(3600)

const canCreate = computed(
  () => !!plan.value && !!selected.value && perm.canWrite('dns'),
)

function close() {
  emit('update:show', false)
}

function reset() {
  plan.value = null
  loadFailed.value = ''
  selected.value = ''
  ttl.value = 3600
}

async function load() {
  if (!props.subnetId) return
  loading.value = true
  loadFailed.value = ''
  try {
    const result = await planReverseZone(props.subnetId)
    plan.value = result
    // The most specific delegation is preselected because it is the one that
    // matches the subnet's own prefix. It is only a default: the alternatives
    // are on screen so it stays the operator's decision.
    selected.value = result.zone_name
  } catch (err: unknown) {
    plan.value = null
    loadFailed.value = err instanceof Error ? err.message : t('common.failed')
  } finally {
    loading.value = false
  }
}

async function create() {
  if (!perm.canWrite('dns')) return
  if (!plan.value || !selected.value) return
  creating.value = true
  try {
    // The write that the old button only pretended to make. Its refusal -- a
    // zone that already exists, most often -- is the answer the operator needs,
    // so it is shown rather than swallowed.
    await createDNSZone({ name: selected.value, type: 'primary', default_ttl: ttl.value })
    message.success(t('ipam.reverseZone.created'))
    emit('created')
    close()
  } catch (err: unknown) {
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

<template>
  <NModal
    :show="show"
    preset="card"
    :title="t('ipam.reverseZone.title')"
    style="width: 620px;"
    @update:show="emit('update:show', $event)"
  >
    <NAlert v-if="subnetLabel" type="default" :show-icon="false" style="margin-bottom: 14px;">
      {{ t('ipam.subnets.title') }}: {{ subnetLabel }}
    </NAlert>

    <NSpin :show="loading">
      <!--
 A plan that could not be read is not a plan with no options. The
           whole reason this dialog exists is that the previous version
           reported success without writing anything; an empty answer that
           cannot be told from a failed lookup would be the same mistake in a
           new place.
-->
      <NAlert v-if="loadFailed" type="error" :title="t('ipam.reverseZone.loadFailed')">
        {{ loadFailed }}
      </NAlert>

      <template v-else-if="plan">
        <NForm label-placement="left" label-width="140px">
          <NFormItem :label="t('ipam.reverseZone.cidr')">
            <NText code>{{ plan.cidr }}</NText>
          </NFormItem>

          <NFormItem :label="t('ipam.reverseZone.candidates')">
            <NRadioGroup v-model:value="selected">
              <NSpace vertical size="small">
                <NRadio v-for="name in plan.candidates" :key="name" :value="name">
                  {{ name }}
                  <NTag v-if="name === plan.zone_name" size="small" :bordered="false" style="margin-left: 8px;">
                    {{ t('ipam.reverseZone.mostSpecific') }}
                  </NTag>
                </NRadio>
              </NSpace>
            </NRadioGroup>
          </NFormItem>

          <NFormItem :label="t('ipam.reverseZone.ttl')">
            <NInputNumber v-model:value="ttl" :min="60" style="width: 100%;" />
          </NFormItem>
        </NForm>

        <NText depth="3" style="font-size: 13px;">{{ t('ipam.reverseZone.candidatesHint') }}</NText>

        <!--
 Creating a DNS zone needs dns:write, which is not the permission
             that got the operator to this page. Saying so beats a button that
             fails on click.
-->
        <NAlert v-if="!perm.canWrite('dns')" type="warning" style="margin-top: 14px;">
          {{ t('ipam.reverseZone.noPermission') }}
        </NAlert>
        <NAlert v-else type="info" style="margin-top: 14px;">
          {{ t('ipam.reverseZone.willCreate') }}
        </NAlert>
      </template>
    </NSpin>

    <template #footer>
      <NSpace justify="end">
        <NButton @click="close">{{ t('common.cancel') }}</NButton>
        <NButton type="primary" :loading="creating" :disabled="!canCreate" @click="create">
          {{ t('ipam.reverseZone.create') }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>
