<script setup lang="ts">
import { onMounted } from 'vue'
import { listCategories } from './api'
import { setCategories } from './types'

onMounted(async () => {
  try {
    setCategories(await listCategories())
  } catch {
    /* keep built-in labels */
  }
})
</script>

<template>
  <div class="shell">
    <header class="top">
      <router-link class="brand" to="/">
        <span class="mark">OCR</span>
        <span>
          <strong>清单识别</strong>
          <small>上传截图 · 选择版式 · 自动结构化</small>
        </span>
      </router-link>
    </header>
    <main>
      <router-view />
    </main>
  </div>
</template>
