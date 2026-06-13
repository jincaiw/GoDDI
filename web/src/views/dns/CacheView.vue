<template>
  <div>
    <page-header :title="t('dns.cache.title')">
      <n-button v-if="perm.canDelete('dns')" type="error" @click="handleFlushCache">{{ t('dns.cache.flushCache') }}</n-button>
    </page-header>

    <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.entries')" :value="stats.entries" />
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.maxEntries')" :value="stats.max_entries" />
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.hitRate')">
            <template #default>{{ ((stats.hit_rate ?? 0) * 100).toFixed(1) }}%</template>
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.missRate')">
            <template #default>{{ (((stats.miss_rate ?? 0)) * 100).toFixed(1) }}%</template>
          </n-statistic>
        </n-card>
      </n-gi>
    </n-grid>

    <confirm-dialog
      :show="showFlushConfirm"
      :message="t('dns.cache.flushConfirm')"
      @confirm="confirmFlush"
      @cancel="showFlushConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { getDNSCacheStats, flushDNSCache, type CacheStats } from '@/api/dns'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const stats = ref<CacheStats>({ entries: 0, max_entries: 0, hit_rate: 0, miss_rate: 0 })
const showFlushConfirm = ref(false)

async function loadStats() {
  try {
    stats.value = await getDNSCacheStats()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

function handleFlushCache() {
  showFlushConfirm.value = true
}

async function confirmFlush() {
  try {
    await flushDNSCache()
    message.success(t('common.success'))
    loadStats()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
  showFlushConfirm.value = false
}

onMounted(loadStats)
</script>
