// Un EventSource mínimo sobre `fetch`, solo para los tests de node: node no trae
// EventSource, y así el stream pasa por MSW (`server`) como pasaría en el navegador.
// Implementa lo que usa el datasource: `open`, `error`, eventos con nombre, `id:` →
// `lastEventId` y `close()`. No reconecta: el panel cierra el EventSource en el primer
// `error`, así que la reconexión automática de uno real nunca llega a pasar.
//
//   vi.stubGlobal('EventSource', FetchEventSource)

export class FetchEventSource extends EventTarget {
	static readonly CONNECTING = 0
	static readonly OPEN = 1
	static readonly CLOSED = 2
	/** Todas las conexiones abiertas en el test, para verificarlas. */
	static readonly instances: FetchEventSource[] = []

	readonly url: string
	readonly withCredentials = false
	readyState = FetchEventSource.CONNECTING
	readonly #controller = new AbortController()

	constructor(url: string | URL) {
		super()
		this.url = String(url)
		FetchEventSource.instances.push(this)
		void this.#run()
	}

	close(): void {
		this.readyState = FetchEventSource.CLOSED
		this.#controller.abort()
	}

	#fail(): void {
		if (this.readyState === FetchEventSource.CLOSED) return
		this.readyState = FetchEventSource.CLOSED
		this.dispatchEvent(new Event('error'))
	}

	async #run(): Promise<void> {
		let response: Response
		try {
			response = await fetch(this.url, {
				headers: { accept: 'text/event-stream' },
				signal: this.#controller.signal,
			})
		} catch {
			this.#fail()
			return
		}
		const type = response.headers.get('content-type') ?? ''
		if (!response.ok || !type.startsWith('text/event-stream') || !response.body) {
			this.#fail()
			return
		}
		if (this.readyState === FetchEventSource.CLOSED) return
		this.readyState = FetchEventSource.OPEN
		this.dispatchEvent(new Event('open'))

		const reader = response.body.pipeThrough(new TextDecoderStream()).getReader()
		let buffer = ''
		let event = 'message'
		let data: string[] = []
		let lastEventId = ''
		try {
			for (;;) {
				const { value, done } = await reader.read()
				if (done) break
				buffer += value
				let newline = buffer.indexOf('\n')
				while (newline >= 0) {
					const line = buffer.slice(0, newline).replace(/\r$/, '')
					buffer = buffer.slice(newline + 1)
					newline = buffer.indexOf('\n')
					if (line === '') {
						if (data.length > 0 && this.readyState === FetchEventSource.OPEN) {
							this.dispatchEvent(
								new MessageEvent(event, { data: data.join('\n'), lastEventId }),
							)
						}
						event = 'message'
						data = []
						continue
					}
					if (line.startsWith(':')) continue
					const colon = line.indexOf(':')
					const field = colon < 0 ? line : line.slice(0, colon)
					const raw = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /, '')
					if (field === 'event') event = raw
					else if (field === 'data') data.push(raw)
					else if (field === 'id') lastEventId = raw
				}
			}
		} catch {
			// Abortado con close(), o el stream se rompió: lo mismo que el servidor cerrando.
		}
		// Un EventSource real, al cerrarse el stream, avisa con `error` y reconecta.
		this.#fail()
	}
}
