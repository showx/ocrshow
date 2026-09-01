<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { exportUrl, fileUrl, getJob, updateRecords } from '../api'
import { STATUS_LABEL, categoryName, columnsFor, type Job } from '../types'

const INT_KEYS = new Set(['rank', 'excel_row', 'col_group'])

type DraftRow = {
  payload: Record<string, unknown>
}

const props = defineProps<{ id: string }>()
const job = ref<Job | null>(null)
const drafts = ref<DraftRow[]>([])
const savedSnapshot = ref('[]')
const seeded = ref(false)
const error = ref('')
const saveError = ref('')
const saveHint = ref('')
const saving = ref(false)
const showLog = ref(false)
const previewIndex = ref<number | null>(null)
const editIndex = ref<number | null>(null)
const editIsNew = ref(false)
const editForm = ref<Record<string, string>>({})
const editFirstInput = ref<HTMLInputElement | null>(null)
let timer = 0
let overlayBound = false

const sheetType = computed(() => {
  const types = job.value?.detected_types?.filter(Boolean) || []
  if (types.length) return types[0]
  if (job.value?.category && job.value.category !== 'auto') return job.value.category
  return 'generic'
})

const columns = computed(() => columnsFor(sheetType.value))
const live = computed(() => job.value?.status === 'pending' || job.value?.status === 'running')
const editable = computed(() => Boolean(job.value) && !live.value)

const dirty = computed(() => serialize(drafts.value) !== savedSnapshot.value)
const previewFile = computed(() => {
  if (previewIndex.value === null || !job.value?.files) return null
  return job.value.files[previewIndex.value] ?? null
})
const canPrev = computed(() => previewIndex.value !== null && previewIndex.value > 0)
const canNext = computed(() => {
  const files = job.value?.files
  if (previewIndex.value === null || !files) return false
  return previewIndex.value < files.length - 1
})
const editOpen = computed(() => editIndex.value !== null)
const editImageFile = computed(() => {
  const files = job.value?.files
  if (!files?.length) return null
  const name = String(editForm.value.image || '')
  if (name) {
    const hit = files.find((f) => f.stored_name === name || f.filename === name)
    if (hit) return hit
  }
  return files.length === 1 ? files[0] : null
})
const editTitle = computed(() => {
  if (!editOpen.value) return ''
  if (editIsNew.value) return '新增记录'
  const rank = editForm.value.rank
  const name = editForm.value.app_name
  if (name) return `编辑 · ${name}`
  if (rank) return `编辑第 ${rank} 行`
  return '编辑记录'
})

function serialize(rows: DraftRow[]) {
  return JSON.stringify(rows.map((row) => coercePayload(row.payload)))
}

function coerceValue(key: string, value: unknown): unknown {
  if (value === null || value === undefined) return value
  if (typeof value === 'number' || typeof value === 'boolean') return value
  const s = String(value).trim()
  if (s === '') return ''
  if (INT_KEYS.has(key)) {
    const n = Number(s)
    return Number.isFinite(n) ? Math.round(n) : s
  }
  if (/^-?\d+(\.\d+)?$/.test(s)) {
    const n = Number(s)
    if (Number.isFinite(n)) return n
  }
  return s
}

function coercePayload(payload: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = { ...payload }
  for (const col of columns.value) {
    if (col.key in out) out[col.key] = coerceValue(col.key, out[col.key])
  }
  return out
}

function display(row: Record<string, unknown>, key: string) {
  const value = row[key]
  if (value === null || value === undefined) return ''
  return String(value)
}

function hydrate(next: Job) {
  drafts.value = (next.records || []).map((r) => ({ payload: { ...(r.payload || {}) } }))
  savedSnapshot.value = serialize(drafts.value)
  seeded.value = true
}

function blankForm(): Record<string, string> {
  const last = drafts.value[drafts.value.length - 1]?.payload
  const form: Record<string, string> = {}
  for (const col of columns.value) {
    if (col.key === 'rank') form.rank = String(drafts.value.length + 1)
    else if (col.key === 'image' && last?.image) form.image = String(last.image)
    else if (col.key === 'date' && last?.date) form.date = String(last.date)
    else form[col.key] = ''
  }
  return form
}

function formFromPayload(payload: Record<string, unknown>): Record<string, string> {
  const form: Record<string, string> = {}
  for (const col of columns.value) {
    form[col.key] = display(payload, col.key)
  }
  return form
}

async function focusEdit() {
  await nextTick()
  editFirstInput.value?.focus()
  editFirstInput.value?.select()
}

async function openEdit(index: number) {
  const row = drafts.value[index]
  if (!row) return
  closePreview()
  editIsNew.value = false
  editIndex.value = index
  editForm.value = formFromPayload(row.payload)
  await focusEdit()
}

async function openCreate() {
  closePreview()
  editIsNew.value = true
  editIndex.value = drafts.value.length
  editForm.value = blankForm()
  await focusEdit()
}

function closeEdit() {
  editIndex.value = null
  editIsNew.value = false
  editForm.value = {}
}

function applyEdit() {
  if (editIndex.value === null) return
  const current = editIsNew.value ? {} : { ...(drafts.value[editIndex.value]?.payload || {}) }
  const payload: Record<string, unknown> = {
    ...current,
    sheet_type: current.sheet_type || sheetType.value,
  }
  if (editIsNew.value) payload.source = current.source || 'manual'
  for (const col of columns.value) {
    payload[col.key] = coerceValue(col.key, editForm.value[col.key] ?? '')
  }
  if (editIsNew.value) {
    drafts.value = [...drafts.value, { payload }]
  } else {
    const rows = drafts.value.slice()
    rows[editIndex.value] = { payload }
    drafts.value = rows
  }
  saveHint.value = ''
  closeEdit()
}

function setEditField(key: string, value: string) {
  editForm.value = { ...editForm.value, [key]: value }
}

function bindFirstInput(el: unknown, index: number) {
  if (index !== 0) return
  editFirstInput.value = (el as HTMLInputElement | null) || null
}

function removeRow(index: number) {
  drafts.value = drafts.value.filter((_, i) => i !== index)
  saveHint.value = ''
}

function discard() {
  if (!job.value) return
  hydrate(job.value)
  saveError.value = ''
  saveHint.value = '已还原'
}

async function save() {
  if (!job.value || saving.value) return false
  saving.value = true
  saveError.value = ''
  saveHint.value = ''
  try {
    const records = drafts.value.map((row) => coercePayload(row.payload))
    const updated = await updateRecords(job.value.id, records)
    job.value = updated
    hydrate(updated)
    saveHint.value = '已保存，导出将使用当前表格'
    return true
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : '保存失败'
    return false
  } finally {
    saving.value = false
  }
}

async function downloadExport() {
  if (!job.value) return
  if (dirty.value) {
    const ok = await save()
    if (!ok) return
  }
  try {
    const res = await fetch(exportUrl(job.value.id), { credentials: 'include' })
    if (!res.ok) throw new Error('导出失败')
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `ocr-${job.value.id}.json`
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : '导出失败'
  }
}

function applyJob(next: Job) {
  const isLive = next.status === 'pending' || next.status === 'running'
  job.value = next
  error.value = ''
  showLog.value = isLive || Boolean(next.error)
  if (!isLive && timer) {
    window.clearInterval(timer)
    timer = 0
  }
  if (isLive) return
  if (!seeded.value || !dirty.value) hydrate(next)
}

async function load() {
  try {
    applyJob(await getJob(props.id))
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  }
}

watch(
  () => props.id,
  async () => {
    closePreview()
    closeEdit()
    seeded.value = false
    drafts.value = []
    savedSnapshot.value = '[]'
    saveError.value = ''
    saveHint.value = ''
    await load()
  },
)

watch(columns, () => {
  if (!dirty.value) savedSnapshot.value = serialize(drafts.value)
})

function openPreview(index: number) {
  previewIndex.value = index
}

function closePreview() {
  previewIndex.value = null
}

function shiftPreview(delta: number) {
  const files = job.value?.files
  if (!files?.length || previewIndex.value === null) return
  const next = previewIndex.value + delta
  if (next < 0 || next >= files.length) return
  previewIndex.value = next
}

function onPreviewKey(ev: KeyboardEvent) {
  if (editOpen.value) {
    if (ev.key === 'Escape') closeEdit()
    return
  }
  if (previewIndex.value === null) return
  if (ev.key === 'Escape') closePreview()
  if (ev.key === 'ArrowLeft') shiftPreview(-1)
  if (ev.key === 'ArrowRight') shiftPreview(1)
}

function lockOverlay(open: boolean) {
  document.body.style.overflow = open ? 'hidden' : ''
  if (open === overlayBound) return
  overlayBound = open
  if (open) window.addEventListener('keydown', onPreviewKey)
  else window.removeEventListener('keydown', onPreviewKey)
}

watch([previewIndex, editIndex], ([preview, edit]) => {
  lockOverlay(preview !== null || edit !== null)
})

function onBeforeUnload(ev: BeforeUnloadEvent) {
  if (!dirty.value) return
  ev.preventDefault()
  ev.returnValue = ''
}

onBeforeRouteLeave(() => {
  if (dirty.value && !window.confirm('有未保存的修改，确定离开？')) return false
  closePreview()
  closeEdit()
})

onMounted(async () => {
  await load()
  timer = window.setInterval(load, 2000)
  window.addEventListener('beforeunload', onBeforeUnload)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  window.removeEventListener('beforeunload', onBeforeUnload)
  lockOverlay(false)
})
</script>

<template>
  <div v-if="error" class="card empty">{{ error }}</div>
  <div v-else-if="!job" class="card empty">加载任务…</div>
  <template v-else>
    <div class="nav">
      <router-link class="btn ghost tiny" to="/">← 返回</router-link>
      <button class="btn ghost tiny" type="button" :disabled="!editable" @click="downloadExport">
        {{ dirty ? '保存并导出 JSON' : '导出 JSON' }}
      </button>
    </div>

    <section class="card meta">
      <div>
        <h2>{{ categoryName(job.category) }}</h2>
        <p class="hint">任务 {{ job.id }} · {{ job.image_count }} 张图 · {{ drafts.length || job.record_count }} 条记录</p>
      </div>
      <div class="meta-right">
        <span v-if="dirty" class="badge dirty">未保存</span>
        <span class="badge" :class="job.status">{{ STATUS_LABEL[job.status] }}</span>
      </div>
    </section>

    <section v-if="job.files?.length" class="thumbs">
      <button
        v-for="(file, i) in job.files"
        :key="file.id"
        class="card thumb"
        type="button"
        @click="openPreview(i)"
      >
        <img :src="fileUrl(job.id, file.stored_name)" :alt="file.filename" />
        <span>{{ file.filename }}</span>
      </button>
    </section>

    <Teleport to="body">
      <div
        v-if="previewFile && job"
        class="lightbox"
        role="dialog"
        aria-modal="true"
        :aria-label="previewFile.filename"
        @click.self="closePreview"
      >
        <button class="lightbox-close" type="button" aria-label="关闭预览" @click="closePreview">×</button>
        <button
          v-if="canPrev"
          class="lightbox-nav prev"
          type="button"
          aria-label="上一张"
          @click="shiftPreview(-1)"
        >
          ‹
        </button>
        <figure class="lightbox-figure">
          <img :src="fileUrl(job.id, previewFile.stored_name)" :alt="previewFile.filename" />
          <figcaption>{{ previewFile.filename }}</figcaption>
        </figure>
        <button
          v-if="canNext"
          class="lightbox-nav next"
          type="button"
          aria-label="下一张"
          @click="shiftPreview(1)"
        >
          ›
        </button>
      </div>
    </Teleport>

    <Teleport to="body">
      <div
        v-if="editOpen && job"
        class="dialog-backdrop"
        role="dialog"
        aria-modal="true"
        :aria-label="editTitle"
      >
        <form class="dialog" @submit.prevent="applyEdit">
          <header class="dialog-head">
            <div>
              <h3>{{ editTitle }}</h3>
              <p class="hint">改完点确定写回表格，再点页面上的保存才会真正存下来。</p>
            </div>
            <button class="dialog-x" type="button" aria-label="关闭" @click="closeEdit">×</button>
          </header>
          <div class="dialog-body" :class="{ solo: !editImageFile }">
            <aside v-if="editImageFile" class="dialog-shot">
              <img :src="fileUrl(job.id, editImageFile.stored_name)" :alt="editImageFile.filename" />
              <span>{{ editImageFile.filename }}</span>
            </aside>
            <div class="dialog-fields">
              <label v-for="(col, i) in columns" :key="col.key">
                <span>{{ col.label }}</span>
                <input
                  :ref="(el) => bindFirstInput(el, i)"
                  type="text"
                  spellcheck="false"
                  :value="editForm[col.key] ?? ''"
                  :name="col.key"
                  @input="setEditField(col.key, ($event.target as HTMLInputElement).value)"
                />
              </label>
            </div>
          </div>
          <footer class="dialog-foot">
            <button class="btn ghost" type="button" @click="closeEdit">取消</button>
            <button class="btn" type="submit">确定</button>
          </footer>
        </form>
      </div>
    </Teleport>

    <p v-if="job.error" class="err">{{ job.error }}</p>

    <section class="card table-wrap">
      <div v-if="live" class="empty">
        正在识别，通常需要 1～3 分钟（首次加载模型会更久）。页面会自动刷新。
      </div>
      <template v-else>
        <div class="table-bar">
          <div>
            <strong>识别结果</strong>
            <p class="hint">表格只展示结果。要改某行，点右侧「编辑」在弹框里修改，确定后再保存。</p>
          </div>
          <div class="table-actions">
            <button class="btn ghost tiny" type="button" @click="openCreate">新增一行</button>
            <button class="btn ghost tiny" type="button" :disabled="!dirty || saving" @click="discard">撤销</button>
            <button class="btn tiny" type="button" :disabled="!dirty || saving" @click="save">
              {{ saving ? '保存中…' : '保存修改' }}
            </button>
          </div>
        </div>
        <p v-if="saveError" class="err pad">{{ saveError }}</p>
        <p v-else-if="saveHint" class="ok pad">{{ saveHint }}</p>
        <div v-if="!drafts.length" class="empty">
          没有解析出记录。
          <button class="btn tiny" type="button" @click="openCreate">手动添加一行</button>
        </div>
        <table v-else>
          <thead>
            <tr>
              <th class="act">操作</th>
              <th v-for="col in columns" :key="col.key">{{ col.label }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, i) in drafts" :key="i">
              <td class="act">
                <span class="act-btns">
                  <button class="btn tiny" type="button" @click="openEdit(i)">编辑</button>
                  <button class="btn ghost tiny" type="button" @click="removeRow(i)">删除</button>
                </span>
              </td>
              <td v-for="col in columns" :key="col.key" :class="{ link: display(row.payload, col.key).startsWith('http') }">
                <a
                  v-if="display(row.payload, col.key).startsWith('http')"
                  :href="display(row.payload, col.key)"
                  target="_blank"
                >
                  {{ display(row.payload, col.key) }}
                </a>
                <template v-else>{{ display(row.payload, col.key) || '—' }}</template>
              </td>
            </tr>
          </tbody>
        </table>
      </template>
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

.meta-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.badge.dirty {
  background: var(--warn-soft);
  color: var(--warn);
}

.thumbs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.thumb {
  appearance: none;
  overflow: hidden;
  padding: 8px;
  width: 100%;
  display: block;
  text-align: left;
  cursor: pointer;
  font: inherit;
  color: inherit;
}

.thumb:hover {
  border-color: #d6c4ae;
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

.table-bar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 16px 0;
}

.table-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  flex-shrink: 0;
}

.pad {
  margin: 8px 16px 0;
}

.ok {
  color: var(--ok);
  font-size: 13px;
}

table {
  width: 100%;
  border-collapse: collapse;
  min-width: 720px;
}

th,
td {
  text-align: left;
  padding: 8px 10px;
  border-bottom: 1px solid var(--line);
  font-size: 13px;
  vertical-align: middle;
}

th {
  position: sticky;
  top: 0;
  background: #fffaf3;
  color: var(--muted);
  font-size: 12px;
}

th.act,
td.act {
  width: 128px;
  text-align: left;
  white-space: nowrap;
  position: sticky;
  left: 0;
  background: #fffaf3;
  z-index: 1;
}

th.act {
  z-index: 2;
}

.act-btns {
  display: inline-flex;
  gap: 6px;
}

td.link {
  max-width: 280px;
  word-break: break-all;
}

.empty {
  display: grid;
  justify-items: center;
  gap: 12px;
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

@media (max-width: 720px) {
  .table-bar {
    flex-direction: column;
  }
}

.lightbox {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 64px 36px;
  background: rgba(28, 25, 23, 0.82);
  backdrop-filter: blur(6px);
}

.lightbox-figure {
  margin: 0;
  max-width: min(96vw, 1100px);
  text-align: center;
}

.lightbox-figure img {
  display: block;
  max-width: min(96vw, 1100px);
  max-height: calc(100vh - 96px);
  object-fit: contain;
  border-radius: 10px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.35);
  background: #1c1917;
}

.lightbox-figure figcaption {
  margin-top: 12px;
  color: #f5e6d3;
  font-size: 13px;
}

.lightbox-close,
.lightbox-nav {
  position: absolute;
  border: 0;
  background: rgba(255, 247, 237, 0.12);
  color: #fff7ed;
  cursor: pointer;
  font: inherit;
}

.lightbox-close {
  top: 16px;
  right: 16px;
  width: 40px;
  height: 40px;
  border-radius: 999px;
  font-size: 24px;
  line-height: 1;
}

.lightbox-nav {
  top: 50%;
  width: 44px;
  height: 44px;
  border-radius: 999px;
  font-size: 32px;
  line-height: 1;
  transform: translateY(-50%);
}

.lightbox-nav.prev {
  left: 16px;
}

.lightbox-nav.next {
  right: 16px;
}

.lightbox-close:hover,
.lightbox-nav:hover {
  background: rgba(255, 247, 237, 0.22);
}

@media (max-width: 720px) {
  .lightbox {
    padding: 56px 16px 24px;
  }

  .lightbox-nav.prev {
    left: 8px;
  }

  .lightbox-nav.next {
    right: 8px;
  }
}

.dialog-backdrop {
  position: fixed;
  inset: 0;
  z-index: 90;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(28, 25, 23, 0.45);
  backdrop-filter: blur(4px);
}

.dialog {
  width: min(920px, 100%);
  max-height: min(88vh, 820px);
  display: flex;
  flex-direction: column;
  background: var(--card);
  border: 1px solid var(--line);
  border-radius: 16px;
  box-shadow: 0 24px 60px rgba(41, 24, 8, 0.18);
}

.dialog-head,
.dialog-foot {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px;
}

.dialog-head {
  border-bottom: 1px solid var(--line);
}

.dialog-head h3 {
  margin: 0 0 4px;
  font-size: 16px;
}

.dialog-x {
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--ink);
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  flex-shrink: 0;
}

.dialog-x:hover {
  background: var(--accent-soft);
}

.dialog-body {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 16px;
  padding: 16px 18px;
  overflow: auto;
}

.dialog-body.solo {
  grid-template-columns: 1fr;
}

.dialog-shot {
  display: grid;
  align-content: start;
  gap: 8px;
}

.dialog-shot img {
  width: 100%;
  border-radius: 10px;
  background: #ddd2c2;
  object-fit: contain;
  max-height: 280px;
}

.dialog-shot span {
  font-size: 12px;
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 14px;
}

.dialog-fields label {
  display: grid;
  gap: 6px;
}

.dialog-fields label span {
  font-size: 12px;
  color: var(--muted);
}

.dialog-fields input {
  width: 100%;
  border: 1px solid var(--line);
  background: #fff;
  border-radius: 8px;
  padding: 8px 10px;
}

.dialog-fields input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.dialog-foot {
  justify-content: flex-end;
  border-top: 1px solid var(--line);
}

@media (max-width: 720px) {
  .dialog-body {
    grid-template-columns: 1fr;
  }

  .dialog-fields {
    grid-template-columns: 1fr;
  }
}
</style>
