<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { exportUrl, fileUrl, getJob } from '../api'
import { STATUS_LABEL, categoryName, columnsFor, type Job } from '../types'

const props = defineProps<{ id: string }>()
const job = ref<Job | null>(null)
const error = ref('')
const showLog = ref(false)
let timer = 0

const sheetType = computed(() => {
  const types = job.value?.detected_types?.filter(Boolean) || []
  if (types.length) return types[0]
  if (job.value?.category && job.value.category !== 'auto') return job.value.category
  return 'generic'
})

const columns = computed(() => columnsFor(sheetType.value))

const rows = computed(() => job.value?.records?.map((r) => r.payload) || [])

function cell(row: Record<string, unknown>, key: string) {
  const value = row[key]
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'number') return Number.isInteger(value) ? String(value) : value.toFixed(2)
  return String(value)
}

async function load() {
  try {
    job.value = await getJob(props.id)
    error.value = ''
    const live = job.value.status === 'pending' || job.value.status === 'running'
    showLog.value = live || Boolean(job.value.error)
    if (!live && timer) {
      window.clearInterval(timer)
      timer = 0
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  }
}

onMounted(async () => {
  await load()
  timer = window.setInterval(load, 2000)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <div v-if="error" class="card empty">{{ error }}</div>
  <div v-else-if="!job" class="card empty">加载任务…</div>
  <template v-else>
    <div class="nav">
      <router-link class="btn ghost tiny" to="/">← 返回</router-link>
      <a class="btn ghost tiny" :href="exportUrl(job.id)">导出 JSON</a>
    </div>

    <section class="card meta">
      <div>
        <h2>{{ categoryName(job.category) }}</h2>
        <p class="hint">任务 {{ job.id }} · {{ job.image_count }} 张图 · {{ job.record_count }} 条记录</p>
      </div>
      <span class="badge" :class="job.status">{{ STATUS_LABEL[job.status] }}</span>
    </section>

    <section v-if="job.files?.length" class="thumbs">
      <a v-for="file in job.files" :key="file.id" class="card thumb" :href="fileUrl(job.id, file.stored_name)" target="_blank">
        <img :src="fileUrl(job.id, file.stored_name)" :alt="file.filename" />
        <span>{{ file.filename }}</span>
      </a>
    </section>

    <p v-if="job.error" class="err">{{ job.error }}</p>

    <section class="card table-wrap">
      <div v-if="job.status === 'running' || job.status === 'pending'" class="empty">
        正在识别，通常需要 1～3 分钟（首次加载模型会更久）。页面会自动刷新。
      </div>
      <div v-else-if="!rows.length" class="empty">没有解析出记录。</div>
      <table v-else>
        <thead>
          <tr>
            <th v-for="col in columns" :key="col.key">{{ col.label }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, i) in rows" :key="i">
            <td v-for="col in columns" :key="col.key" :class="{ link: cell(row, col.key).startsWith('http') }">
              <a v-if="cell(row, col.key).startsWith('http')" :href="cell(row, col.key)" target="_blank">
                {{ cell(row, col.key) }}
              </a>
              <template v-else>{{ cell(row, col.key) }}</template>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="job.log" class="log-wrap">
      <button class="btn ghost tiny" type="button" @click="showLog = !showLog">
        {{ showLog ? '收起日志' : '查看识别日志' }}
      </button>
      <pre v-if="showLog" class="card log">{{ job.log }}</pre>
    </section>
  </template>
</template>

<style scoped>
.nav {
  display: flex;
  justify-content: space-between;
  margin-bottom: 14px;
}

.meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 18px 20px;
  margin-bottom: 16px;
}

.thumbs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.thumb {
  overflow: hidden;
  padding: 8px;
}

.thumb img {
  width: 100%;
  height: 110px;
  object-fit: cover;
  border-radius: 8px;
  background: #ddd2c2;
}

.thumb span {
  display: block;
  margin-top: 6px;
  font-size: 12px;
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.table-wrap {
  overflow: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
  min-width: 720px;
}

th,
td {
  text-align: left;
  padding: 10px 12px;
  border-bottom: 1px solid var(--line);
  font-size: 13px;
  vertical-align: top;
}

th {
  position: sticky;
  top: 0;
  background: #fffaf3;
  color: var(--muted);
  font-size: 12px;
}

td.link {
  max-width: 280px;
  word-break: break-all;
}

.err {
  color: var(--bad);
}

.log-wrap {
  margin-top: 16px;
}

.log {
  margin-top: 10px;
  padding: 14px;
  font-size: 12px;
  white-space: pre-wrap;
  max-height: 320px;
  overflow: auto;
  background: #1c1917;
  color: #fed7aa;
}
</style>
