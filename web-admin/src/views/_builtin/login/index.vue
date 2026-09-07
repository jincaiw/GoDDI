<template>
  <div class="login-container" :class="{ dark: themeStore.darkMode }">
    <div class="login-card">
      <div class="login-header">
        <span class="login-logo">G</span>
        <h1>{{ $t('system.title') }}</h1>
        <p>Enterprise DDI Management Console</p>
      </div>
      <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top" :show-label="false">
        <n-form-item path="username">
          <n-input
            v-model:value="formData.username"
            :placeholder="$t('auth.username')"
            :input-props="{ autocomplete: 'username' }"
            @keydown.enter="handleLogin"
          />
        </n-form-item>
        <n-form-item path="password">
          <n-input
            v-model:value="formData.password"
            type="password"
            show-password-on="click"
            :placeholder="$t('auth.password')"
            :input-props="{ autocomplete: 'current-password' }"
            @keydown.enter="handleLogin"
          />
        </n-form-item>
        <n-form-item v-if="showTotp" path="totp_code">
          <n-input
            v-model:value="formData.totp_code"
            :placeholder="$t('auth.totpCode')"
            @keydown.enter="handleLogin"
          />
        </n-form-item>
        <n-button type="primary" block size="large" :loading="loading" @click="handleLogin">
          {{ $t('auth.login') }}
        </n-button>
      </n-form>
      <n-alert v-if="errorMsg" type="error" style="margin-top: 16px" closable>
        {{ errorMsg }}
      </n-alert>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '@/store/modules/auth';
import { useThemeStore } from '@/store/modules/theme';
import type { FormInst, FormRules } from 'naive-ui';

defineOptions({ name: 'LoginPage' });

const themeStore = useThemeStore();
const authStore = useAuthStore();
const { t } = useI18n();

const loading = ref(false);
const errorMsg = ref('');
const showTotp = ref(false);
const formRef = ref<FormInst | null>(null);

const formData = reactive({
  username: '',
  password: '',
  totp_code: ''
});

const rules: FormRules = {
  username: { required: true, message: t('auth.usernameRequired'), trigger: 'blur' },
  password: { required: true, message: t('auth.passwordRequired'), trigger: 'blur' }
};

async function handleLogin() {
  if (loading.value) return;
  if (formRef.value) {
    try {
      await formRef.value.validate();
    } catch {
      return;
    }
  }

  loading.value = true;
  errorMsg.value = '';

  try {
    // the store handles token persistence, user info and redirect on success
    await authStore.login(formData.username, formData.password, formData.totp_code || undefined);
  } catch (err: unknown) {
    const errMessage = err instanceof Error ? err.message : t('auth.loginFailed');
    // if TOTP is required, reveal the code field
    const lower = errMessage.toLowerCase();
    if (lower.includes('totp') || lower.includes('2fa') || lower.includes('two-factor')) {
      showTotp.value = true;
    }
    errorMsg.value = errMessage;
  } finally {
    loading.value = false;
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
  background: radial-gradient(ellipse at 50% 0%, #0a84ff10, transparent 65%), var(--app-page-background, #f5f5f7);
}

.login-card {
  width: 420px;
  max-width: 100%;
  box-sizing: border-box;
  padding: 40px;
  background: var(--app-surface, #fff);
  border: 1px solid var(--app-border, #e5e5ea);
  border-radius: 16px;
  box-shadow: 0 20px 70px rgb(0 0 0 / 6%);
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
  background: linear-gradient(155deg, #49a3ff, #0a84ff);
  border-radius: 16px;
  margin-bottom: 20px;
}

.login-header h1 {
  margin: 0;
  font-size: 30px;
  font-weight: 600;
  letter-spacing: -1px;
}

.login-header p {
  margin: 8px 0 0;
  color: var(--app-muted, #6e6e73);
  font-size: 13px;
}

@media (max-width: 480px) {
  .login-card {
    padding: 32px 24px;
  }
}
</style>
