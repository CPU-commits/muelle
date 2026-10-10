export type ToastTone = 'success' | 'error' | 'info'

export interface ToastItem {
	id: string
	tone: ToastTone
	title: string
	description: string | null
}

const TOAST_TTL_MS = 4000

class ToastStore {
	items = $state<ToastItem[]>([])

	push(item: ToastItem): void {
		this.items.push(item)
	}

	dismiss(id: string): void {
		this.items = this.items.filter((toast) => toast.id !== id)
	}
}

export const toasts = new ToastStore()

function show(tone: ToastTone, title: string, description?: string): void {
	const id = crypto.randomUUID()
	toasts.push({ id, tone, title, description: description ?? null })
	setTimeout(() => toasts.dismiss(id), TOAST_TTL_MS)
}

export const toast = {
	success: (title: string, description?: string) => show('success', title, description),
	error: (title: string, description?: string) => show('error', title, description),
	info: (title: string, description?: string) => show('info', title, description),
}
