<template>
  <n-modal
    :show="show"
    preset="card"
    :title="t('ipam.import.title')"
    style="width: 760px;"
    @update:show="emit('update:show', $event)"
  >
    <n-alert v-if="subnetLabel" type="default" :show-icon="false" style="margin-bottom: 14px;">
      {{ t('ipam.subnets.title') }}: {{ subnetLabel }}
    </n-alert>

    <!-- Step 1: pick. The backend takes the file's text as a JSON field, so
         this reads the file rather than uploading it. -->
    <template v-if="step === 'pick'">
      <n-upload
        :default-upload="false"
        :show-file-list="false"
        accept=".csv,.txt"
        @change="handleFileChange"
      >
        <n-button>{{ t('ipam.import.chooseFile') }}</n-button>
      </n-upload>
      <p v-if="fileName" class="hint">{{ fileName }}</p>
      <n-input
        v-model:value="csvText"
        type="textarea"
        :rows="10"
        style="margin-top: 12px;"
        :placeholder="t('ipam.import.pastePlaceholder')"
      />
    </template>

    <!-- Step 2 and 3: the report. A preview and an import answer with the same
         shape on purpose; only `applied` and the heading differ. -->
    <template v-else>
      <n-alert v-if="step === 'preview'" type="info" :show-icon="false" style="margin-bottom: 14px;">
        {{ t('ipam.import.previewTitle') }}
      </n-alert>
      <n-alert v-else type="success" :show-icon="false" style="margin-bottom: 14px;">
        {{ t('ipam.import.appliedTitle') }}
      </n-alert>

      <n-alert v-if="rejected" type="error" :title="t('ipam.import.rejected')" style="margin-bottom: 14px;">
        {{ t('ipam.import.rejectedHint') }}
      </n-alert>

      <report-summary v-if="report" :report="report" />

      <template v-if="report && report.errors.length > 0">
        <section-title :text="t('ipam.import.errors')" :count="report.errors.length" />
        <n-alert type="error" :show-icon="false">
          <ul style="margin: 0; padding-left: 18px;">
            <li v-for="(line, index) in report.errors" :key="index">{{ line }}</li>
          </ul>
        </n-alert>
      </template>

      <template v-if="report && report.changes.length > 0">
        <section-title :text="t('ipam.import.changes')" :count="report.changes.length" />
        <n-data-table
          size="small"
          :bordered="false"
          :single-line="false"
          :max-height="240"
          :row-key="(row: ImportPlan) => `${row.line}-${row.ip}`"
          :columns="changeColumns"
          :data="report.changes"
        />
        <p v-if="report.truncated" class="hint">{{ t('ipam.import.truncated') }}</p>
      </template>
    </template>

    <template #footer>
      <n-space justify="end">
        <n-button v-if="step === 'pick'" @click="close">{{ t('common.cancel') }}</n-button>
        <n-button v-else-if="step === 'preview'" :disabled="importing" @click="backToPick">
          {{ t('ipam.import.back') }}
        </n-button>
        <n-button v-else @click="close">{{ t('ipam.import.close') }}</n-button>

        <n-button v-if="step === 'pick'" type="primary" :loading="previewing" :disabled="!canPreview" @click="handlePreview">
          {{ t('ipam.import.preview') }}
        </n-button>
        <n-button
          v-else-if="step === 'preview'"
          type="primary"
          :loading="importing"
          :disabled="hasBlockingErrors"
          @click="handleImport"
        >
          {{ t('ipam.import.confirm') }}
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
import ReportSummary from '@/components/ipam/ImportReportSummary.vue'
import { ApiError } from '@/service/api/goddi/client'
import { importIPAMData, previewIPAMImport, type ImportPlan, type ImportReport } from '@/service/api/goddi/ipam'

const props = defineProps<{
  show: boolean
  subnetId: string
  subnetLabel: string
}>()

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'imported'): void
}>()

const { t } = useI18n()
const message = useMessage()

const step = ref<'pick' | 'preview' | 'done'>('pick')
const csvText = ref('')
const fileName = ref('')
const previewing = ref(false)
const importing = ref(false)
const report = ref<ImportReport | null>(null)
/** True when the server refused the batch and returned the report with the 400. */
const rejected = ref(false)

const canPreview = computed(() => csvText.value.trim().length > 0)

/**
 * A file the preview rejects is a file the import rejects: both run the same
 * planner. Offering the second button on a batch we already know is refused
 * would spend a round trip to learn nothing.
 */
const hasBlockingErrors = computed(() => (report.value?.errors.length ?? 0) > 0)

function close() {
  emit('update:show', false)
}

function reset() {
  step.value = 'pick'
  csvText.value = ''
  fileName.value = ''
  report.value = null
  rejected.value = false
}

function backToPick() {
  step.value = 'pick'
  report.value = null
  rejected.value = false
}

async function handleFileChange({ file }: { file: { file: File | null; name: string } }) {
  const raw = file.file
  if (!raw) return
  fileName.value = file.name
  csvText.value = await raw.text()
  report.value = null
}

function body() {
  return { type: 'addresses', format: 'csv', parent_id: props.subnetId, data: csvText.value }
}

/** A rejected batch carries its report on the error; without it the operator
 *  sees "import failed" and has to re-read the file by eye. */
function reportFromError(err: unknown): ImportReport | null {
  if (!(err instanceof ApiError)) return null
  const payload = err.payload as ImportReport | undefined
  if (!payload || !Array.isArray(payload.errors)) return null
  return payload
}

async function handlePreview() {
  previewing.value = true
  rejected.value = false
  try {
    report.value = await previewIPAMImport(body())
    step.value = 'preview'
  } catch (err: unknown) {
    const fromError = reportFromError(err)
    if (fromError) {
      report.value = fromError
      rejected.value = true
      step.value = 'preview'
    } else {
      message.error(err instanceof Error ? err.message : t('common.failed'))
    }
  } finally {
    previewing.value = false
  }
}

async function handleImport() {
  importing.value = true
  try {
    report.value = await importIPAMData(body())
    step.value = 'done'
    rejected.value = false
    message.success(t('ipam.import.appliedTitle'))
    emit('imported')
  } catch (err: unknown) {
    const fromError = reportFromError(err)
    if (fromError) {
      report.value = fromError
      rejected.value = true
      step.value = 'preview'
    } else {
      message.error(err instanceof Error ? err.message : t('common.failed'))
    }
  } finally {
    importing.value = false
  }
}

function actionLabel(action: string) {
  const map: Record<string, string> = {
    create: t('ipam.import.actionCreate'),
    update: t('ipam.import.actionUpdate'),
    unchanged: t('ipam.import.actionUnchanged'),
  }
  return map[action] || action
}

function actionType(action: string) {
  const map: Record<string, 'success' | 'info' | 'default' | 'warning'> = {
    create: 'success',
    update: 'info',
    unchanged: 'default',
  }
  return map[action] || 'default'
}

const changeColumns = [
  { title: () => t('ipam.import.line'), key: 'line', width: 70 },
  { title: () => t('ipam.import.ip'), key: 'ip', width: 160 },
  {
    title: () => t('ipam.import.action'),
    key: 'action',
    width: 100,
    render: (row: ImportPlan) =>
      h(NTag, { size: 'small', type: actionType(row.action), bordered: false }, { default: () => actionLabel(row.action) }),
  },
  {
    title: () => t('ipam.import.statusChange'),
    key: 'status',
    width: 170,
    render: (row: ImportPlan) => (row.previous_status ? `${row.previous_status} → ${row.status}` : row.status),
  },
  {
    title: () => t('ipam.import.changedFields'),
    key: 'changed_fields',
    render: (row: ImportPlan) => (row.changed_fields?.length ? row.changed_fields.join(', ') : '—'),
  },
]

watch(
  () => props.show,
  (show) => {
    if (show) reset()
  },
)
</script>

<style scoped>
.hint {
  margin: 8px 0 0;
  color: var(--text-color-3, #999);
  font-size: 12px;
}
</style>
