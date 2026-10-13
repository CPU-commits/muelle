import { createQuery, type CreateQueryResult } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { pendingPhase, type RoadmapPhase, UnauthorizedError } from '@/shared/domain'
import { appStatusQuery, streamAppLogs } from '../../../data'
import {
	advanceResumePoint,
	appendCapped,
	type AppLogLine,
	type AppRef,
	type AppStatus,
	DEFAULT_LOG_TAIL,
	isRepeatedAfterResume,
	LOG_LINE_CAP,
	LOG_TAILS,
	type LogTail,
	logTasks,
	matchesFilter,
	RECONNECT_BASE_MS,
	RECONNECT_MAX_MS,
	reconnectDelayMs,
	type ResumePoint,
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
 * - `retrying`: se cortó (o no abrió) y espera para volver a intentar.
 * - `asleep`: cerrado porque la pestaña lleva un rato oculta; sigue al volver.
 * - `stopped`: la sesión venció (el cliente HTTP ya mandó al login).
 */
export type LogConnection = 'idle' | 'connecting' | 'live' | 'retrying' | 'asleep' | 'stopped'

/** Por qué no está en vivo: se cortó estando abierto, o no llegó a abrir. */
export type LogProblem = 'dropped' | 'unreachable'

/** Las esperas de la pantalla (los tests las acortan). */
export interface LogTiming {
	/** La primera espera antes de reconectar; después se duplica. */
	readonly reconnectBaseMs: number
	readonly reconnectMaxMs: number
	/** Con la pestaña oculta más que esto, se cierra el stream. */
	readonly hiddenCloseMs: number
	/** Un stream que estuvo abierto al menos esto vuelve a empezar la espera desde 1 s. */
	readonly stableStreamMs: number
}

export const DEFAULT_LOG_TIMING: LogTiming = {
	reconnectBaseMs: RECONNECT_BASE_MS,
	reconnectMaxMs: RECONNECT_MAX_MS,
	hiddenCloseMs: 30_000,
	stableStreamMs: 10_000,
}

/** Cada cuánto se pintan las líneas que llegaron (un log ruidoso no repinta por línea). */
export const LOG_FLUSH_MS = 100

type ConnectOutcome = 'closed' | 'asleep' | 'unauthorized'

// Logs (App-Logs.dc.html): en vivo por SSE desde Docker, por servicio. Pausar congela lo
// que se ve y guarda lo que llega; reanudar lo suma. En memoria quedan las últimas
// LOG_LINE_CAP líneas (también en pausa). Si el stream se corta, vuelve a conectar solo
// (1, 2, 4… hasta 30 s) desde la hora de la última línea; con la pestaña oculta más de
// 30 s lo cierra y lo retoma al volver.
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
	/** Cómo terminó la última conexión, hasta que otra abre. */
	problem = $state<LogProblem | null>(null)
	lines = $state.raw<readonly KeyedLine[]>([])
	/** Lo que llegó en pausa, todavía sin mostrar. */
	pending = $state.raw<readonly KeyedLine[]>([])

	readonly tailOptions = LOG_TAILS
	readonly cap = LOG_LINE_CAP

	readonly #props: () => LogsProps
	readonly #timing: LogTiming
	#sequence = 0
	#incoming: KeyedLine[] = []
	#flushTimer: ReturnType<typeof setTimeout> | null = null
	/** La hora de la última línea y las que llegaron en ese instante: desde ahí se reconecta. */
	#resumePoint: ResumePoint | null = null
	#hiddenTimer: ReturnType<typeof setTimeout> | null = null
	#asleep = false
	/** Cierra el stream abierto (la pestaña lleva oculta un rato). */
	#closeForHidden: (() => void) | null = null
	/** Corta la espera en curso: el backoff, o la pestaña oculta. */
	#wake: (() => void) | null = null

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

	constructor(props: () => LogsProps, timing: Partial<LogTiming> = {}) {
		this.#props = props
		this.#timing = { ...DEFAULT_LOG_TIMING, ...timing }
		this.status = createQuery(() => appStatusQuery(this.ref))
		this.services = $derived((this.status.data?.services ?? []).map((service) => service.name))
		this.service = $derived(
			this.selectedService !== null && this.services.includes(this.selectedService)
				? this.selectedService
				: (this.services[0] ?? null),
		)

		// Otra app, otro servicio u otras líneas iniciales: otro stream, de cero.
		$effect(() => {
			const ref = this.ref
			const service = this.service
			const tail = this.tail
			if (service === null) return
			const controller = new AbortController()
			untrack(() => void this.#run(ref, service, tail, controller.signal))
			return () => {
				controller.abort()
				this.#cancelFlush()
			}
		})

		$effect(() => {
			untrack(this.#onVisibility)
			document.addEventListener('visibilitychange', this.#onVisibility)
			return () => {
				document.removeEventListener('visibilitychange', this.#onVisibility)
				this.#clearHiddenTimer()
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

	/**
	 * Para el lector de pantalla. No lleva la cuenta (cambia con cada línea): se anuncia una
	 * vez cuando empiezan a juntarse líneas en pausa.
	 */
	get announcement(): string {
		return this.paused && this.pending.length > 0 ? m.deploy_logs_new_lines_announce() : ''
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

	/** Vacía la vista. Al reconectar sigue desde la última línea: lo limpiado no vuelve. */
	clear = (): void => {
		this.#incoming = []
		this.lines = []
		this.pending = []
	}

	/** "Reconectar ahora": no espera lo que faltaba del backoff. */
	reconnect = (): void => {
		this.#wake?.()
	}

	retry = (): void => {
		void this.status.refetch()
	}

	async #run(ref: AppRef, service: string, tail: LogTail, signal: AbortSignal): Promise<void> {
		this.#incoming = []
		this.lines = []
		this.pending = []
		this.#resumePoint = null
		this.problem = null
		let attempt = 0
		while (!signal.aborted) {
			if (this.#asleep) {
				this.connection = 'asleep'
				await this.#waitForWake(null, signal)
				continue
			}
			const started = Date.now()
			const outcome = await this.#connect(ref, service, tail, signal)
			if (signal.aborted) return
			if (outcome === 'unauthorized') {
				this.connection = 'stopped'
				this.problem = null
				return
			}
			if (outcome === 'asleep') continue
			// Uno que anduvo un buen rato no hereda la espera larga de cortes anteriores.
			if (this.problem === 'dropped' && Date.now() - started >= this.#timing.stableStreamMs) {
				attempt = 0
			}
			this.connection = 'retrying'
			const { reconnectBaseMs, reconnectMaxMs } = this.#timing
			await this.#waitForWake(
				reconnectDelayMs(attempt, reconnectBaseMs, reconnectMaxMs),
				signal,
			)
			attempt += 1
		}
	}

	/** Una conexión, hasta que se corta, se cierra por la pestaña oculta o `outer` se aborta. */
	async #connect(
		ref: AppRef,
		service: string,
		tail: LogTail,
		outer: AbortSignal,
	): Promise<ConnectOutcome> {
		const controller = new AbortController()
		const abort = (): void => controller.abort()
		outer.addEventListener('abort', abort, { once: true })
		let asleep = false
		const closeForHidden = (): void => {
			asleep = true
			controller.abort()
		}
		this.#closeForHidden = closeForHidden
		// Con líneas ya vistas se sigue desde la última (`since` es inclusivo: lo de ese
		// instante que ya estaba se descarta). Sin ninguna, `tail` de nuevo.
		const resume = this.#resumePoint
		let opened = false
		this.connection = 'connecting'
		try {
			const input = { app: ref, service, tail, ...(resume ? { since: resume.since } : {}) }
			for await (const event of streamAppLogs(input, controller.signal)) {
				if (controller.signal.aborted) break
				if (event.kind === 'open') {
					opened = true
					this.connection = 'live'
					this.problem = null
				} else if (!resume || !isRepeatedAfterResume(event.line, resume)) {
					this.#enqueue(event.line)
				}
			}
		} catch (error) {
			if (error instanceof UnauthorizedError) return 'unauthorized'
		} finally {
			outer.removeEventListener('abort', abort)
			// El stream de antes termina de cerrar cuando el nuevo ya arrancó: no le borra el suyo.
			if (this.#closeForHidden === closeForHidden) this.#closeForHidden = null
			if (!outer.aborted) this.#flush()
		}
		if (asleep) return 'asleep'
		// "Se cortó" solo si estuvo abierto (en esta conexión o antes de la racha de intentos).
		if (!outer.aborted)
			this.problem = opened || this.problem === 'dropped' ? 'dropped' : 'unreachable'
		return 'closed'
	}

	/** Espera `ms` (o hasta que la despierten, si es null), `reconnect()` o el abort. */
	#waitForWake(ms: number | null, signal: AbortSignal): Promise<void> {
		return new Promise((resolve) => {
			if (signal.aborted) {
				resolve()
				return
			}
			let timer: ReturnType<typeof setTimeout> | null = null
			const done = (): void => {
				if (timer !== null) clearTimeout(timer)
				signal.removeEventListener('abort', done)
				if (this.#wake === done) this.#wake = null
				resolve()
			}
			if (ms !== null) timer = setTimeout(done, ms)
			signal.addEventListener('abort', done, { once: true })
			this.#wake = done
		})
	}

	#onVisibility = (): void => {
		if (document.visibilityState === 'hidden') {
			this.#hiddenTimer ??= setTimeout(this.#sleep, this.#timing.hiddenCloseMs)
			return
		}
		this.#clearHiddenTimer()
		// Lo que llegó con la pestaña oculta (los timers ahí van frenados) se pinta ya.
		this.#flush()
		if (this.#asleep) {
			this.#asleep = false
			this.#wake?.()
		}
	}

	#sleep = (): void => {
		this.#hiddenTimer = null
		this.#asleep = true
		this.#closeForHidden?.()
		// Si estaba esperando para reintentar, que vaya a dormir en vez de conectar.
		this.#wake?.()
	}

	#clearHiddenTimer(): void {
		if (this.#hiddenTimer !== null) clearTimeout(this.#hiddenTimer)
		this.#hiddenTimer = null
	}

	#enqueue(line: AppLogLine): void {
		this.#remember(line)
		this.#sequence += 1
		this.#incoming.push({ key: this.#sequence, line })
		// Con la pestaña oculta el timer se frena y esto podría crecer sin tope: se queda con
		// las últimas (recortar de a tandas, no en cada línea).
		if (this.#incoming.length > LOG_LINE_CAP * 2) {
			this.#incoming = this.#incoming.slice(-LOG_LINE_CAP)
		}
		this.#flushTimer ??= setTimeout(this.#flush, LOG_FLUSH_MS)
	}

	#remember(line: AppLogLine): void {
		this.#resumePoint = advanceResumePoint(this.#resumePoint, line)
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
