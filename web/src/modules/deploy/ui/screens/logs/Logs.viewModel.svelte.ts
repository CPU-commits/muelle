import { createQuery, type CreateQueryResult } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { pendingPhase, type RoadmapPhase } from '@/shared/domain'
import { appStatusQuery, streamAppLogs } from '../../../data'
import {
	appendCapped,
	type AppLogLine,
	type AppRef,
	type AppStatus,
	DEFAULT_LOG_TAIL,
	LOG_LINE_CAP,
	LOG_TAILS,
	type LogTail,
	logTasks,
	matchesFilter,
} from '../../../domain'

export interface LogsProps {
	readonly project: string
	readonly stage: string
	readonly app: string
}

/** Una línea con su key estable (las líneas no tienen id propio). */
export interface KeyedLine {
	readonly key: number
	readonly line: AppLogLine
}

/**
 * - `idle`: todavía no hay servicio que seguir.
 * - `connecting`: abriendo el stream.
 * - `live`: conectado.
 * - `lost`: se cortó (o no abrió): se ofrece reconectar.
 */
export type LogConnection = 'idle' | 'connecting' | 'live' | 'lost'

/** Cada cuánto se pintan las líneas que llegaron (un log ruidoso no repinta por línea). */
export const LOG_FLUSH_MS = 100

// Logs (App-Logs.dc.html): en vivo por SSE desde Docker, por servicio. Pausar congela lo
// que se ve y guarda lo que llega; reanudar lo suma. En memoria quedan las últimas
// LOG_LINE_CAP líneas (también mientras está en pausa).
export class LogsViewModel {
	selectedService = $state<string | null>(null)
	tail = $state<LogTail>(DEFAULT_LOG_TAIL)
	/** La tarea elegida (`api.1`); vacío = todas. */
	task = $state('')
	query = $state('')
	paused = $state(false)
	/** Autoscroll: true mientras quien lee está al final. */
	follow = $state(true)
	connection = $state<LogConnection>('idle')
	lines = $state.raw<readonly KeyedLine[]>([])
	/** Lo que llegó en pausa, todavía sin mostrar. */
	pending = $state.raw<readonly KeyedLine[]>([])

	readonly tailOptions = LOG_TAILS
	readonly cap = LOG_LINE_CAP

	readonly #props: () => LogsProps
	#session = $state(0)
	/** La próxima conexión es una reconexión: mantiene las líneas y pide `tail=0`. */
	#resume = false
	#sequence = 0
	#incoming: KeyedLine[] = []
	#flushTimer: ReturnType<typeof setTimeout> | null = null

	// Se crean en el constructor: la query lee `#props`, que un inicializador de campo
	// todavía no tendría, y los derivados leen la query.
	readonly status: CreateQueryResult<AppStatus, Error>
	readonly services: string[]
	/** El servicio que se sigue: el elegido, o el primero. */
	readonly service: string | null

	readonly tasks: string[] = $derived(logTasks(this.lines.map((keyed) => keyed.line)))

	readonly visibleLines: readonly KeyedLine[] = $derived.by(() => {
		const filter = { query: this.query, task: this.task }
		if (!filter.query.trim() && !filter.task) return this.lines
		return this.lines.filter((keyed) => matchesFilter(keyed.line, filter))
	})

	constructor(props: () => LogsProps) {
		this.#props = props
		this.status = createQuery(() => appStatusQuery(this.ref))
		this.services = $derived((this.status.data?.services ?? []).map((service) => service.name))
		this.service = $derived(
			this.selectedService !== null && this.services.includes(this.selectedService)
				? this.selectedService
				: (this.services[0] ?? null),
		)
		$effect(() => {
			const service = this.service
			const tail = this.tail
			void this.#session
			if (service === null) return
			const controller = new AbortController()
			untrack(() => {
				const resume = this.#resume
				this.#resume = false
				void this.#run(service, resume ? 0 : tail, resume, controller.signal)
			})
			return () => {
				controller.abort()
				this.#cancelFlush()
			}
		})
	}

	get ref(): AppRef {
		const { project, stage, app } = this.#props()
		return { project, stage, app }
	}

	get pendingPhase(): RoadmapPhase | null {
		return pendingPhase(this.status.error)
	}

	get isLoading(): boolean {
		return this.status.isPending
	}

	get loadError(): unknown {
		return this.pendingPhase === null ? this.status.error : null
	}

	get noServices(): boolean {
		return this.status.isSuccess && this.services.length === 0
	}

	get isFiltering(): boolean {
		return this.query.trim() !== '' || this.task !== ''
	}

	/** Se llegó al tope: las más viejas se están descartando. */
	get isCapped(): boolean {
		return this.lines.length >= LOG_LINE_CAP
	}

	selectService = (service: string): void => {
		if (service === this.service) return
		this.selectedService = service
		this.task = ''
	}

	setTail = (raw: string): void => {
		const value = Number(raw)
		const tail = LOG_TAILS.find((option) => option === value)
		if (tail !== undefined) this.tail = tail
	}

	setTask = (task: string): void => {
		this.task = task
	}

	setFollow = (follow: boolean): void => {
		this.follow = follow
	}

	togglePause = (): void => {
		if (!this.paused) {
			this.paused = true
			return
		}
		this.#flush()
		this.lines = appendCapped(this.lines, this.pending, LOG_LINE_CAP)
		this.pending = []
		this.paused = false
		this.follow = true
	}

	clear = (): void => {
		this.#incoming = []
		this.lines = []
		this.pending = []
	}

	/** Vuelve a conectar sin perder lo que ya se ve ni repetirlo (`tail=0`). */
	reconnect = (): void => {
		this.#resume = true
		this.#session += 1
	}

	retry = (): void => {
		void this.status.refetch()
	}

	async #run(
		service: string,
		tail: LogTail,
		resume: boolean,
		signal: AbortSignal,
	): Promise<void> {
		if (!resume) {
			this.#incoming = []
			this.lines = []
			this.pending = []
		}
		this.connection = 'connecting'
		try {
			for await (const event of streamAppLogs({ app: this.ref, service, tail }, signal)) {
				if (signal.aborted) return
				if (event.kind === 'open') this.connection = 'live'
				else this.#enqueue(event.line)
			}
			// El servidor cerró el stream: para seguir hay que reconectar.
			if (!signal.aborted) this.connection = 'lost'
		} catch {
			if (!signal.aborted) this.connection = 'lost'
		} finally {
			if (!signal.aborted) this.#flush()
		}
	}

	#enqueue(line: AppLogLine): void {
		this.#sequence += 1
		this.#incoming.push({ key: this.#sequence, line })
		this.#flushTimer ??= setTimeout(this.#flush, LOG_FLUSH_MS)
	}

	#flush = (): void => {
		this.#cancelFlush()
		const batch = this.#incoming
		if (batch.length === 0) return
		this.#incoming = []
		if (this.paused) this.pending = appendCapped(this.pending, batch, LOG_LINE_CAP)
		else this.lines = appendCapped(this.lines, batch, LOG_LINE_CAP)
	}

	#cancelFlush(): void {
		if (this.#flushTimer !== null) clearTimeout(this.#flushTimer)
		this.#flushTimer = null
	}
}
