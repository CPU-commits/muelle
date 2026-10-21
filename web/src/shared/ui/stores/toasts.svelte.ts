export type ToastTone = 'success' | 'error' | 'info'

export interface ToastItem {
	id: string
	tone: ToastTone
	title: string
	description: string | null
}

const TOAST_TTL_MS = 4000

interface Countdown {
	remaining: number
	startedAt: number
	handle: ReturnType<typeof setTimeout> | undefined
}

// Los avisos se van solos a los 4 s; los errores se quedan hasta que se cierran (hay que
// poder leerlos y actuar). Con el puntero o el foco encima, la cuenta se pausa.
class ToastStore {
	items = $state<ToastItem[]>([])

	#countdowns = new Map<string, Countdown>()
	#paused = false

	push(item: ToastItem, ttl: number | null = TOAST_TTL_MS): void {
		this.items.push(item)
		if (ttl === null) return
		this.#countdowns.set(item.id, { remaining: ttl, startedAt: 0, handle: undefined })
		if (!this.#paused) this.#start(item.id)
	}

	dismiss(id: string): void {
		const countdown = this.#countdowns.get(id)
		if (countdown) clearTimeout(countdown.handle)
		this.#countdowns.delete(id)
		this.items = this.items.filter((toast) => toast.id !== id)
	}

	pause(): void {
		if (this.#paused) return
		this.#paused = true
		const now = Date.now()
		for (const countdown of this.#countdowns.values()) {
			clearTimeout(countdown.handle)
			countdown.handle = undefined
			countdown.remaining = Math.max(0, countdown.remaining - (now - countdown.startedAt))
		}
	}

	resume(): void {
		if (!this.#paused) return
		this.#paused = false
		for (const id of this.#countdowns.keys()) this.#start(id)
	}

	#start(id: string): void {
		const countdown = this.#countdowns.get(id)
		if (!countdown) return
		countdown.startedAt = Date.now()
		countdown.handle = setTimeout(() => this.dismiss(id), countdown.remaining)
	}
}

export const toasts = new ToastStore()

// Un contador y no `crypto.randomUUID`: ese solo existe en un contexto seguro, y el panel
// por HTTP en una IP de la red local no lo es. El id solo tiene que ser único en la pestaña.
let lastId = 0

function show(tone: ToastTone, title: string, description?: string): void {
	lastId += 1
	const id = `toast-${lastId}`
	toasts.push(
		{ id, tone, title, description: description ?? null },
		tone === 'error' ? null : TOAST_TTL_MS,
	)
}

export const toast = {
	success: (title: string, description?: string) => show('success', title, description),
	/** No se va sola: se cierra con su botón. */
	error: (title: string, description?: string) => show('error', title, description),
	info: (title: string, description?: string) => show('info', title, description),
}
