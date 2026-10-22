import { onMounted, onBeforeUnmount, ref } from 'vue'

let openDialogs = 0
let savedOverflow = ''

export function useModal() {
  const dialog = ref<HTMLElement | null>(null)
  let previous: HTMLElement | null = null
  const trapFocus = (event: KeyboardEvent) => {
    if (event.key !== 'Tab' || !dialog.value) return
    const controls = [...dialog.value.querySelectorAll<HTMLElement>(
      'button:not([disabled]), a[href], input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled]), summary, [tabindex="0"]',
    )].filter(element => element.getClientRects().length > 0)
    const first = controls[0], last = controls[controls.length - 1]
    if (!first || !last) { event.preventDefault(); dialog.value.focus(); return }
    if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.value)) {
      event.preventDefault(); last.focus()
    } else if (!event.shiftKey && (document.activeElement === last || document.activeElement === dialog.value)) {
      event.preventDefault(); first.focus()
    }
  }
  onMounted(() => {
    previous = document.activeElement as HTMLElement | null
    if (openDialogs++ === 0) {
      savedOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }
    dialog.value?.addEventListener('keydown', trapFocus)
    dialog.value?.focus({ preventScroll: true })
  })
  onBeforeUnmount(() => {
    dialog.value?.removeEventListener('keydown', trapFocus)
    if (--openDialogs === 0) document.body.style.overflow = savedOverflow
    if (previous?.isConnected) previous.focus({ preventScroll: true })
  })
  return dialog
}
