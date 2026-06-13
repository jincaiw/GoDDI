<template>
  <n-modal
    v-model:show="showModal"
    preset="card"
    :title="title"
    type="warning"
    style="width: 420px;"
  >
    <p>{{ message }}</p>
    <template #footer>
      <n-space justify="end">
        <n-button @click="handleCancel">{{ cancelLabel }}</n-button>
        <n-button type="primary" @click="handleConfirm">{{ confirmLabel }}</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{
  show: boolean
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
}>(), {
  confirmText: '',
  cancelText: ''
})

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const { t } = useI18n()
const showModal = ref(props.show)

const confirmLabel = computed(() => props.confirmText || t('common.confirm'))
const cancelLabel = computed(() => props.cancelText || t('common.cancel'))

watch(() => props.show, (val) => { showModal.value = val })
watch(showModal, (val) => { emit('update:show', val) })

function handleConfirm() {
  emit('confirm')
  showModal.value = false
}

function handleCancel() {
  emit('cancel')
  showModal.value = false
}
</script>
