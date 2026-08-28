<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { createJob, deleteJob, listJobs } from '../api'
import { STATUS_LABEL, categories, categoryName, type Job } from '../types'

const router = useRouter()
const jobs = ref<Job[]>([])
const files = ref<File[]>([])
const category = ref('auto')
const skipVL = ref(true)
const dragging = ref(false)
const submitting = ref(false)
const error = ref('')

async function refresh() {
  jobs.value = await listJobs()
}

onMounted(refresh)

watch(
  categories,
  (list) => {
    if (!list.some((item) => item.id === category.value)) category.value = 'auto'
  },
  { deep: true },
)

function pickFiles(list: FileList | File[] | null) {
  if (!list) return
  const next = Array.from(list).filter((f) => f.type.startsWith('image/'))
  files.value = [...files.value, ...next].slice(0, 12)
}

function onDrop(ev: DragEvent) {
  dragging.value = false
  pickFiles(ev.dataTransfer?.files || null)
}

function removeFile(index: number) {
  files.value.splice(index, 1)
}

async function submit() {
  error.value = ''
  if (!files.value.length) {
    error.value = '请先选择图片'
    return
  }
  submitting.value = true
  try {
    const job = await createJob(files.value, category.value, skipVL.value)
    files.value = []
    await refresh()
    router.push(`/jobs/${job.id}`)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '提交失败'
  } finally {
    submitting.value = false
  }
}

async function removeJob(job: Job) {
  if (!confirm(`删除任务 ${job.id}？`)) return
  await deleteJob(job.id)
  await refresh()
}

function fmtTime(value?: string) {
  if (!value) return '—'
  return value.replace('T', ' ')
}
</script>

<template>
  <section class="card upload">
    <div class="head">
      <div>
        <h2>上传并识别</h2>
        <p class="hint">默认自动识别。内部版式模块装上后会出现在这里，可手动指定。</p>
      </div>
    </div>

    <div class="cats">
      <button
        v-for="item in categories"
        :key="item.id"
        type="button"
        class="cat"
        :class="{ on: category === item.id }"
        @click="category = item.id"
      >
        <b>{{ item.name }}</b>
        <span>{{ item.desc }}</span>
      </button>
    </div>

    <label
      class="drop"
      :class="{ on: dragging }"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
    >
      <input type="file" accept="image/*" multiple hidden @change="pickFiles(($event.target as HTMLInputElement).files)" />
      <strong>把图片拖到这里，或点击选择</strong>
      <span>支持 jpg / png / webp，单次最多 12 张</span>
    </label>

    <ul v-if="files.length" class="picked">
      <li v-for="(file, i) in files" :key="file.name + i">
        <span>{{ file.name }}</span>
        <em>{{ (file.size / 1024).toFixed(0) }} KB</em>
        <button class="btn ghost tiny" type="button" @click="removeFile(i)">去掉</button>
      </li>
    </ul>

    <div class="actions">
      <label class="check">
        <input v-model="skipVL" type="checkbox" />
        跳过 Qwen3-VL（更快，只跑 PaddleOCR）
      </label>
      <button class="btn" type="button" :disabled="submitting" @click="submit">
        {{ submitting ? '提交中…' : '开始识别' }}
      </button>
    </div>
    <p v-if="error" class="err">{{ error }}</p>
  </section>

  <section class="history">
    <div class="head">
      <h2>最近任务</h2>
      <button class="btn ghost tiny" type="button" @click="refresh">刷新</button>
    </div>
    <div v-if="!jobs.length" class="card empty">还没有任务。上传一张截图即可。</div>
    <div v-else class="card table-wrap">
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>类别</th>
            <th>状态</th>
            <th>图片</th>
            <th>记录</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="job in jobs" :key="job.id">
            <td>{{ fmtTime(job.created_at) }}</td>
            <td>{{ categoryName(job.category) }}</td>
            <td><span class="badge" :class="job.status">{{ STATUS_LABEL[job.status] }}</span></td>
            <td>{{ job.image_count }}</td>
            <td>{{ job.record_count }}</td>
            <td class="row-actions">
              <router-link class="btn tiny" :to="`/jobs/${job.id}`">查看</router-link>
              <button class="btn ghost tiny" type="button" @click="removeJob(job)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.upload {
  padding: 22px;
}

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.cats {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}

.cat {
  text-align: left;
  border: 1px solid var(--line);
  background: #fff;
  border-radius: 12px;
  padding: 12px;
  cursor: pointer;
  min-height: 92px;
}

.cat b {
  display: block;
  margin-bottom: 6px;
}

.cat span {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.4;
}

.cat.on {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.drop {
  display: grid;
  place-items: center;
  gap: 6px;
  min-height: 140px;
  border: 1.5px dashed #cbbba4;
  border-radius: 14px;
  background: #fff;
  cursor: pointer;
  color: var(--muted);
}

.drop strong {
  color: var(--ink);
}

.drop.on {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.picked {
  list-style: none;
  padding: 0;
  margin: 12px 0 0;
}

.picked li {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--line);
}

.picked em {
  color: var(--muted);
  font-style: normal;
  font-size: 12px;
  margin-left: auto;
}

.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
  gap: 12px;
}

.check {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--muted);
  font-size: 13px;
}

.err {
  color: var(--bad);
  margin: 10px 0 0;
}

.history {
  margin-top: 28px;
}

.table-wrap {
  overflow: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th,
td {
  text-align: left;
  padding: 12px 14px;
  border-bottom: 1px solid var(--line);
  font-size: 14px;
}

th {
  color: var(--muted);
  font-weight: 600;
  font-size: 12px;
}

.row-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

@media (max-width: 900px) {
  .cats {
    grid-template-columns: 1fr 1fr;
  }
  .actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
