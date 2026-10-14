// Un `EventSource` convertido en un iterable asíncrono, para consumirlo con `for await`
// y cortarlo con un AbortSignal. Es lo único del módulo que conoce SSE.
//
// - La sesión viaja sola: la URL es del mismo origen y el navegador manda la cookie.
// - Se emite `open` al conectar y cada evento de `types` en el orden en que llegó.
// - Un `error` del EventSource (no abrió, el servidor cerró, se cayó la red) termina el
//   iterable con `EventStreamError`, DESPUÉS de los eventos que ya habían llegado. No se
//   deja que el EventSource reconecte solo: quien consume decide (polling, reconectar).
// - Al terminar, por lo que sea, se cierra el EventSource.

export interface StreamMessage {
	readonly kind: 'message'
	/** El `event:` del mensaje (`log`, `status`). */
	readonly type: string
	readonly data: string
	/** El `id:` del evento (el offset siguiente, en el log de un deploy). */
	readonly lastEventId: string
}

export type StreamItem = { readonly kind: 'open' } | StreamMessage

export class EventStreamError extends Error {
	/** Si llegó a conectar antes del error. */
	readonly opened: boolean

	constructor(opened: boolean) {
		super(opened ? 'el stream se cortó' : 'el stream no abrió')
		this.name = 'EventStreamError'
		this.opened = opened
	}
}

type QueueItem = StreamItem | { readonly kind: 'error' }

const ERROR_ITEM: QueueItem = { kind: 'error' }

export async function* eventStream(
	url: string,
	types: readonly string[],
	signal: AbortSignal,
): AsyncGenerator<StreamItem, void, undefined> {
	if (signal.aborted) return
	// Sin EventSource (un navegador muy viejo, node sin polyfill) es lo mismo que un stream
	// que no abre: el log de un deploy sigue por polling.
	const Source: typeof EventSource | undefined = globalThis.EventSource
	if (typeof Source !== 'function') throw new EventStreamError(false)

	const source = new Source(url)
	const queue: QueueItem[] = []
	let opened = false
	let wake: (() => void) | null = null

	const push = (item: QueueItem): void => {
		queue.push(item)
		wake?.()
		wake = null
	}
	const onOpen = (): void => {
		opened = true
		push({ kind: 'open' })
	}
	const onMessage = (event: Event): void => {
		const message = event as MessageEvent<unknown>
		push({
			kind: 'message',
			type: message.type,
			data: typeof message.data === 'string' ? message.data : String(message.data),
			lastEventId: message.lastEventId,
		})
	}
	const onError = (): void => {
		// Corta ya, antes de que el EventSource reintente por su cuenta.
		source.close()
		push(ERROR_ITEM)
	}
	const onAbort = (): void => {
		wake?.()
		wake = null
	}

	source.addEventListener('open', onOpen)
	source.addEventListener('error', onError)
	for (const type of types) source.addEventListener(type, onMessage)
	signal.addEventListener('abort', onAbort)

	try {
		while (!signal.aborted) {
			const item = queue.shift()
			if (!item) {
				await new Promise<void>((resolve) => (wake = resolve))
				continue
			}
			if (item.kind === 'error') throw new EventStreamError(opened)
			yield item
		}
	} finally {
		source.close()
		source.removeEventListener('open', onOpen)
		source.removeEventListener('error', onError)
		for (const type of types) source.removeEventListener(type, onMessage)
		signal.removeEventListener('abort', onAbort)
	}
}
