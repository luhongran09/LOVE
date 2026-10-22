import { ref, watch } from 'vue'

function savedTheme(): 'dark' | 'light' {
  try { return localStorage.getItem('footprint-theme') === 'light' ? 'light' : 'dark' }
  catch { return 'dark' }
}

export const theme = ref<'dark' | 'light'>(savedTheme())
watch(theme, value => {
  document.documentElement.dataset.theme = value
  document.documentElement.style.colorScheme = value
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', value === 'dark' ? '#0b0e14' : '#f8f6f0')
  try { localStorage.setItem('footprint-theme', value) } catch { /* Browsing remains available without storage. */ }
}, { immediate: true })

export function toggleTheme() { theme.value = theme.value === 'dark' ? 'light' : 'dark' }
