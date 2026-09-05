<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <span class="login-logo">G</span>
        <h1>GoDDI</h1>
        <p>Enterprise DDI Management Console</p>
      </div>
      <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top">
        <n-form-item path="username" :label="t('auth.username')">
          <n-input
            v-model:value="formData.username"
            @keydown.enter="handleLogin"
          >
            <template #prefix>
              <n-icon><person-outline /></n-icon>
            </template>
          </n-input>
        </n-form-item>
        <n-form-item path="password" :label="t('auth.password')">
          <n-input
            v-model:value="formData.password"
            type="password"
            show-password-on="click"
            @keydown.enter="handleLogin"
          >
            <template #prefix>
              <n-icon><lock-closed-outline /></n-icon>
            </template>
          </n-input>
        </n-form-item>
        <n-form-item v-if="showTotp" path="totp_code" :label="t('auth.totpCode')">
          <n-input
            v-model:value="formData.totp_code"
            :placeholder="t('auth.totpCode')"
            @keydown.enter="handleLogin"
          />
        </n-form-item>
        <n-button
          type="primary"
          block
          :loading="loading"
          @click="handleLogin"
          style="margin-top: 8px;"
        >
          {{ t('auth.login') }}
        </n-button>
      </n-form>
      <n-alert v-if="errorMsg" type="error" style="margin-top: 16px;" closable>
        {{ errorMsg }}
      </n-alert>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import { PersonOutline, LockClosedOutline } from '@vicons/ionicons5'
import { useAuthStore } from '@/stores/auth'
import type { FormInst, MessageApi } from 'naive-ui'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const authStore = useAuthStore()

// useMessage may fail if called before the provider is mounted.
// Fall back to a no-op message API so the component still renders.
let message: MessageApi
try {
  message = useMessage()
} catch {
  message = {
    success: () => {},
    error: () => {},
    warning: () => {},
    info: () => {},
    loading: () => {},
    destroyAll: () => {},
  } as unknown as MessageApi
}

const loading = ref(false)
const errorMsg = ref('')
const showTotp = ref(false)
const formRef = ref<FormInst | null>(null)

const formData = reactive({
  username: '',
  password: '',
  totp_code: '',
})

const rules = {
  username: { required: true, message: t('auth.usernameRequired'), trigger: 'blur' },
  password: { required: true, message: t('auth.passwordRequired'), trigger: 'blur' },
}

async function handleLogin() {
  if (formRef.value) {
    try {
      await formRef.value.validate()
    } catch {
      // Validation failed; n-form will display rule messages
      return
    }
  }

  loading.value = true
  errorMsg.value = ''

  try {
    await authStore.login({
      username: formData.username,
      password: formData.password,
      totp_code: formData.totp_code || undefined,
    })
    message.success(t('auth.loginSuccess'))
    const redirect = (route.query.redirect as string) || '/'
    if (redirect.startsWith('/') && !redirect.startsWith('//') && !redirect.includes('\\\\')) {
      router.push(redirect)
    } else {
      router.push('/')
    }
  } catch (err: unknown) {
    const errMessage = err instanceof Error ? err.message : t('auth.loginFailed')
    // If TOTP required, show the field
    if (errMessage.toLowerCase().includes('totp') || errMessage.toLowerCase().includes('2fa') || errMessage.toLowerCase().includes('two-factor')) {
      showTotp.value = true
    }
    errorMsg.value = errMessage
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  padding: 24px 16px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
  max-width: 100%;
  box-sizing: border-box;
  padding: 40px;
  background: var(--n-color);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.15);
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-logo {
  display: inline-block;
  width: 56px;
  height: 56px;
  line-height: 56px;
  font-size: 32px;
  font-weight: 700;
  color: #fff;
  background: #18a058;
  border-radius: 12px;
  margin-bottom: 12px;
}

.login-header h1 {
  margin: 0;
  font-size: 24px;
}

.login-header p {
  margin: 4px 0 0;
  color: var(--n-text-color-3);
  font-size: 14px;
}
</style>
