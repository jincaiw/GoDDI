<script setup lang="ts">
import { reactive, ref } from 'vue';
import type { FormInstanceFunctions, FormRule } from 'tdesign-vue-next';
import { MessagePlugin } from 'tdesign-vue-next';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';

defineOptions({ name: 'LoginPage' });

/**
 * TDesign login page — first fully migrated page of the staged console
 * migration. It keeps the exact authStore.login contract used by the previous
 * page (username / password / optional totp_code -> JWT + CSRF in
 * sessionStorage -> /auth/me -> redirect), so the router guard, RBAC and
 * request interceptors need no changes.
 */

const authStore = useAuthStore();

const formRef = ref<FormInstanceFunctions | null>(null);
const loading = ref(false);
const errorMsg = ref('');
const showTotp = ref(false);

const formData = reactive({
  username: '',
  password: '',
  totp_code: ''
});

const rules: Record<string, FormRule[]> = {
  username: [{ required: true, message: $t('auth.usernameRequired'), type: 'error' }],
  password: [{ required: true, message: $t('auth.passwordRequired'), type: 'error' }]
};

async function handleLogin() {
  if (loading.value) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  loading.value = true;
  errorMsg.value = '';

  try {
    await authStore.login(formData.username, formData.password, formData.totp_code || undefined);
    await MessagePlugin.success($t('auth.loginSuccess'));
  } catch (err: unknown) {
    const errMessage = err instanceof Error ? err.message : $t('auth.loginFailed');
    const lower = errMessage.toLowerCase();
    if (lower.includes('totp') || lower.includes('2fa') || lower.includes('two-factor') || lower.includes('验证码')) {
      showTotp.value = true;
    }
    errorMsg.value = errMessage;
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo"><TIcon name="logo-github-filled" size="30px" /></div>
        <h1>{{ $t('system.title') }}</h1>
        <p>Enterprise DDI Management Console · Vue 3 + TDesign</p>
      </div>

      <TForm ref="formRef" :data="formData" :rules="rules" label-align="top" @submit="handleLogin">
        <TFormItem name="username" :label-width="0">
          <TInput
            v-model="formData.username"
            size="large"
            clearable
            autocomplete="username"
            :placeholder="$t('auth.username')"
          >
            <template #prefix-icon><TIcon name="user" /></template>
          </TInput>
        </TFormItem>

        <TFormItem name="password" :label-width="0">
          <TInput
            v-model="formData.password"
            size="large"
            type="password"
            autocomplete="current-password"
            :placeholder="$t('auth.password')"
            @enter="handleLogin"
          >
            <template #prefix-icon><TIcon name="lock-on" /></template>
          </TInput>
        </TFormItem>

        <TFormItem v-if="showTotp" name="totp_code" :label-width="0">
          <TInput
            v-model="formData.totp_code"
            size="large"
            :placeholder="$t('auth.totpCode')"
            @enter="handleLogin"
          >
            <template #prefix-icon><TIcon name="secured" /></template>
          </TInput>
        </TFormItem>

        <TButton theme="primary" size="large" block :loading="loading" type="submit">
          {{ $t('auth.login') }}
        </TButton>
      </TForm>

      <TAlert v-if="errorMsg" theme="error" :message="errorMsg" class="login-error" close />
    </div>
  </div>
</template>

<style scoped>
.login-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 24px 16px;
  box-sizing: border-box;
  background: radial-gradient(ellipse at 50% 0%, #0052d910, transparent 65%), #f5f7fa;
}
.login-card {
  width: 420px;
  max-width: 100%;
  box-sizing: border-box;
  padding: 40px;
  background: #fff;
  border: 1px solid #e7e7e7;
  border-radius: 12px;
  box-shadow: 0 20px 70px rgb(0 0 0 / 6%);
}
.login-header {
  text-align: center;
  margin-bottom: 32px;
}
.login-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  color: #fff;
  background: linear-gradient(155deg, #4a90ff, #0052d9);
  border-radius: 14px;
  margin-bottom: 20px;
}
.login-header h1 {
  margin: 0;
  font-size: 28px;
  font-weight: 600;
}
.login-header p {
  margin: 8px 0 0;
  color: #6b7280;
  font-size: 13px;
}
.login-error {
  margin-top: 16px;
}
@media (max-width: 480px) {
  .login-card {
    padding: 32px 24px;
  }
}
</style>
