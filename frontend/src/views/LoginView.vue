<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../auth'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const submitting = ref(false)

async function submit() {
  error.value = ''
  if (!username.value.trim() || !password.value) {
    error.value = '请输入账号和密码'
    return
  }
  submitting.value = true
  try {
    await login(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect || '/')
  } catch (err) {
    error.value = err instanceof Error ? err.message : '登录失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="card login-card">
    <h2>登录</h2>
    <p class="hint">识别工作台需要登录。账号在 .env 或 config.toml 里配置。</p>
    <form class="form" @submit.prevent="submit">
      <label class="field">
        <span>账号</span>
        <input v-model="username" name="username" autocomplete="username" autofocus />
      </label>
      <label class="field">
        <span>密码</span>
        <input v-model="password" name="password" type="password" autocomplete="current-password" />
      </label>
      <p v-if="error" class="err">{{ error }}</p>
      <button class="btn" type="submit" :disabled="submitting">
        {{ submitting ? '登录中…' : '进入工作台' }}
      </button>
    </form>
  </section>
</template>

<style scoped>
.login-card {
  max-width: 420px;
  margin: 48px auto 0;
  padding: 28px 26px 24px;
}

.form {
  display: grid;
  gap: 14px;
  margin-top: 20px;
}

.field {
  display: grid;
  gap: 6px;
  color: var(--muted);
  font-size: 13px;
}

.field input {
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 10px 12px;
  background: #fff;
  color: var(--ink);
}

.field input:focus {
  outline: 2px solid var(--accent-soft);
  border-color: var(--accent);
}

.err {
  color: var(--bad);
  margin: 0;
  font-size: 13px;
}

.btn {
  justify-self: start;
  margin-top: 4px;
}
</style>
