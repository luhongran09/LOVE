import { ref } from 'vue'

// Chinese source messages are the default catalog. Other languages remain a
// migration task; keeping interpolation here lets the current UI run reliably.
export const locale = ref<'zh-CN'>('zh-CN')

export function t(message: string, values: readonly unknown[] = []): string {
  return message.replace(/\{(\d+)\}/g, (placeholder, index: string) => {
    const position = Number(index)
    return position < values.length ? String(values[position] ?? '') : placeholder
  })
}
