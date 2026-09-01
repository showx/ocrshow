<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listCategories, setUnauthorizedHandler } from './api'
import { clearUser, currentUser, logout } from './auth'
import { setCategories } from './types'

const route = useRoute()
const router = useRouter()
const isLogin = computed(() => route.name === 'login')

async function loadCategories() {
  try {
    setCategories(await listCategories())
  } catch {
    /* keep built-in labels */
  }
}

onMounted(() => {
  setUnauthorizedHandler(() => {
    if (route.name === 'login') return
    clearUser()
    router.replace({ name: 'login', query: { redirect: route.fullPath } })
  })
  if (currentUser.value) loadCategories()
})

watch(currentUser, (user) => {
  if (user) loadCategories()
})

async function signOut() {
  await logout()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <div class="shell" :class="{ slim: isLogin }">
    <header class="top">
      <router-link class="brand" to="/">
        <span class="mark">OCR</span>
        <span>
          <strong>清单识别</strong>
          <small>上传截图 · 选择版式 · 自动结构化</small>
        </span>
      </router-link>
      <div v-if="currentUser && !isLogin" class="who">
        <span>{{ currentUser.username }}</span>
        <button class="btn ghost tiny" type="button" @click="signOut">退出</button>
      </div>
    </header>
    <main>
      <router-view />
    </main>
  </div>
</template>
