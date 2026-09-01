import { ref } from 'vue'
import { getMe, login as apiLogin, logout as apiLogout, type AuthUser } from './api'

export const currentUser = ref<AuthUser | null>(null)

let loaded = false
let pending: Promise<AuthUser | null> | null = null

export async function ensureUser(): Promise<AuthUser | null> {
  if (loaded) return currentUser.value
  if (pending) return pending
  pending = getMe()
    .then((user) => {
      currentUser.value = user
      loaded = true
      return user
    })
    .catch(() => {
      currentUser.value = null
      loaded = true
      return null
    })
    .finally(() => {
      pending = null
    })
  return pending
}

export async function login(username: string, password: string) {
  const user = await apiLogin(username, password)
  currentUser.value = user
  loaded = true
  return user
}

export async function logout() {
  try {
    await apiLogout()
  } finally {
    currentUser.value = null
    loaded = true
  }
}

export function clearUser() {
  currentUser.value = null
  loaded = true
}
