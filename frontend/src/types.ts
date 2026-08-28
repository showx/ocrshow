import { ref } from 'vue'

export type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export interface JobFile {
  id: number
  job_id: string
  filename: string
  stored_name: string
  date?: string
}

export interface RecordRow {
  id: number
  job_id: string
  sheet_type: string
  image: string
  date: string
  rank: number
  excel_row: number
  col_group: number
  app_name: string
  payload: Record<string, unknown>
}

export interface Job {
  id: string
  category: string
  skip_vl: boolean
  status: JobStatus
  error?: string
  log?: string
  image_count: number
  record_count: number
  detected_types?: string[]
  summary?: unknown
  created_at: string
  started_at?: string
  finished_at?: string
  files?: JobFile[]
  records?: RecordRow[]
}

export interface Column {
  key: string
  label: string
}

export interface Category {
  id: string
  name: string
  desc: string
  columns?: Column[]
}

export const DEFAULT_COLUMNS: Column[] = [
  { key: 'rank', label: '序号' },
  { key: 'app_name', label: '名称' },
  { key: 'image', label: '图片' },
]

export const CATEGORIES: Category[] = [
  {
    id: 'auto',
    name: '自动识别',
    desc: '按表头匹配已安装的版式模块；没有模块时走通用表格',
    columns: DEFAULT_COLUMNS,
  },
]

export const categories = ref<Category[]>(CATEGORIES)

export function setCategories(list: Category[]) {
  if (Array.isArray(list) && list.length) categories.value = list
}

export const STATUS_LABEL: Record<JobStatus, string> = {
  pending: '排队中',
  running: '识别中',
  succeeded: '已完成',
  failed: '失败',
}

export function categoryName(id: string): string {
  return categories.value.find((c) => c.id === id)?.name || id
}

export function columnsFor(sheetType: string): Column[] {
  const hit = categories.value.find((c) => c.id === sheetType)
  if (hit?.columns?.length) return hit.columns
  const generic = categories.value.find((c) => c.id === 'generic')
  if (generic?.columns?.length) return generic.columns
  return DEFAULT_COLUMNS
}
