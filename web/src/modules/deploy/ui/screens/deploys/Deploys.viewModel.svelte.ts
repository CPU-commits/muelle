import { createQuery, type CreateQueryResult, useQueryClient } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import type { Attachment } from 'svelte/attachments'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { pendingPhase, type RoadmapPhase } from '@/shared/domain'
import {
	appStatusQuery,
	deployQuery,
	followDeployLog,
	liveDeploysQuery,
	refreshDeploys,
} from '../../../data'
import {
	appendLogChunk,
	type AppKind,
	type AppRef,
	type AppStatus,
	currentDeploy,
	type Deploy,
	deployDurationMs,
	type DeployId,
	DeployNotFound,
	type DeployStatus,
	EMPTY_LOG,
	flushLog,
	isActiveStatus,
	isTerminalStatus,
	type LogBuffer,
	logText,
	parseDeployId,
} from '../../../domain'

export interface DeploysProps {
	readonly project: string
	readonly stage: string
	readonly app: string
	/** El `?deploy=144` de la URL: qué deploy abrir. Sin él, el más reciente. */
	readonly deployParam?: string | null
	/** Del catálogo (F2), para el diálogo de deploy. */
	readonly kind?: AppKind | null
}

/** Debajo de esto el historial y el log van uno arriba del otro (el `flex-wrap` del layout). */
const STACKED_MAX_WIDTH = 980

function onNextFrame(callback: () => void): () => void {
	if (typeof requestAnimationFrame === 'function') {
		const frame = requestAnimationFrame(callback)
		return () => cancelAnimationFrame(frame)
	}
	const timer = setTimeout(callback, 16)
	return () => clearTimeout(timer)
}

// Deploys (App-Deploys.dc.html): el historial a la izquierda y el log del elegido a la
// derecha. Uno en curso se sigue en vivo por SSE (con fallback a `?offset`); el
// autoscroll se pausa cuando quien lee sube.
export class DeploysViewModel {
	selectedId = $state<DeployId | null>(null)
	log = $state.raw<LogBuffer>(EMPTY_LOG)
	/** El estado que dijo el stream del log, más nuevo que el del historial. */
	liveStatus = $state<DeployStatus | null>(null)
	/** El SSE falló y el log llega por polling. */
	polling = $state(false)
	logLoading = $state(false)
	logError = $state<unknown>(null)
	/** Autoscroll: true mientras quien lee está al final del log. */
	follow = $state(true)
	deployOpen = $state(false)
	/** La hora, para la duración de uno que sigue corriendo. */
	now = $state(Date.now())
	/** Lo que se anuncia a un lector de pantalla cuando el deploy que se sigue termina. */
	announcement = $state('')

	readonly #props: () => DeploysProps
	readonly #client = useQueryClient()
	#attempt = $state(0)
	/** Sube cada vez que quien lee elige un deploy: en un teléfono, el log se trae a la vista. */
	#revealRequest = $state(0)
	// Los chunks que llegan entre dos frames se pintan juntos: un log rápido no repinta por línea.
	#pendingChunk = ''
	#cancelFrame: (() => void) | null = null

	// Se crean en el constructor: sus opciones leen `#props`.
	readonly deploys: CreateQueryResult<readonly Deploy[], Error>
	/** Solo para saber los servicios en el diálogo de deploy. */
	readonly status: CreateQueryResult<AppStatus, Error>
	/** El `?deploy=` que no está en el historial (más viejo que la retención, o borrado). */
	readonly linked: CreateQueryResult<Deploy, Error>

	readonly selected: Deploy | null
	// Lo único que reinicia el seguimiento del log: cambiar de deploy (no su estado).
	readonly #selectedKey: DeployId | null
	/** El `?deploy=` de la URL: cambia sin desmontar la pantalla (otro link a la misma ruta). */
	readonly #linkedId: DeployId | null

	constructor(props: () => DeploysProps) {
		this.#props = props
		this.#linkedId = $derived(parseDeployId(this.#props().deployParam))
		this.selectedId = untrack(() => this.#linkedId)
		this.deploys = createQuery(() => liveDeploysQuery(this.ref))
		this.status = createQuery(() => appStatusQuery(this.ref))
		this.linked = createQuery(() =>
			deployQuery(this.ref, this.#linkedId, {
				enabled: this.deploys.isSuccess && !this.#inHistory(this.#linkedId),
			}),
		)

		// `?deploy=141` → `?deploy=142` en la misma ruta: abre el nuevo.
		$effect(() => {
			const id = this.#linkedId
			if (id === null) return
			untrack(() => {
				if (id === this.selectedId) return
				this.selectedId = id
				this.follow = true
			})
		})

		this.selected = $derived.by(() => {
			const list = this.deploys.data ?? []
			const found = list.find((deploy) => deploy.id === this.selectedId)
			if (found) return found
			if (this.selectedId !== null && this.selectedId === this.linked.data?.id) {
				return this.linked.data
			}
			// Recién encolado o buscando el del link: se espera en vez de abrir otro un momento.
			if (this.selectedId !== null && (this.deploys.isFetching || this.linked.isFetching)) {
				return null
			}
			return list[0] ?? null
		})
		this.#selectedKey = $derived(this.selected?.id ?? null)

		$effect(() => {
			const id = this.#selectedKey
			void this.#attempt
			if (id === null) return
			const controller = new AbortController()
			untrack(() => {
				const deploy = this.selected
				if (deploy) void this.#follow(deploy, controller.signal)
			})
			return () => {
				controller.abort()
				this.#dropPending()
			}
		})

		$effect(() => {
			if (!this.isActive) return
			const timer = setInterval(() => {
				this.now = Date.now()
			}, 1000)
			return () => clearInterval(timer)
		})
	}

	get ref(): AppRef {
		const { project, stage, app } = this.#props()
		return { project, stage, app }
	}

	get kind(): AppKind | null {
		return this.#props().kind ?? null
	}

	get pendingPhase(): RoadmapPhase | null {
		return pendingPhase(this.deploys.error)
	}

	get isLoading(): boolean {
		return this.deploys.isPending
	}

	get loadError(): unknown {
		return this.pendingPhase === null ? this.deploys.error : null
	}

	get history(): readonly Deploy[] {
		return this.deploys.data ?? []
	}

	get isEmpty(): boolean {
		return this.deploys.isSuccess && this.history.length === 0
	}

	/** El `?deploy=` apunta a uno que ya no existe: se avisa y se muestra el más reciente. */
	get linkedMissing(): DeployId | null {
		return this.linked.error instanceof DeployNotFound ? this.#linkedId : null
	}

	/** El estado a mostrar del elegido: el del stream si es más nuevo que el historial. */
	get selectedStatus(): DeployStatus | null {
		const deploy = this.selected
		if (!deploy) return null
		if (isTerminalStatus(deploy.status)) return deploy.status
		return this.liveStatus ?? deploy.status
	}

	get isActive(): boolean {
		const status = this.selectedStatus
		return status !== null && isActiveStatus(status)
	}

	get durationMs(): number | null {
		const deploy = this.selected
		return deploy ? deployDurationMs(deploy, this.now) : null
	}

	get logText(): string {
		return logText(this.log)
	}

	get services(): string[] {
		return (this.status.data?.services ?? []).map((service) => service.name)
	}

	get currentImages(): Record<string, string> {
		const current = currentDeploy(this.history)
		return Object.fromEntries(
			(current?.images ?? []).map((image) => [image.service, image.ref]),
		)
	}

	isSelected = (deploy: Deploy): boolean => deploy.id === this.selected?.id

	/** Elegir un deploy del historial. La URL no cambia: el `?deploy=` es para entrar. */
	select = (id: DeployId): void => {
		if (id === this.selected?.id) {
			this.#revealRequest += 1
			return
		}
		this.selectedId = id
		this.follow = true
		this.#revealRequest += 1
	}

	/**
	 * Para el panel del log: si historial y log van uno arriba del otro (un teléfono),
	 * al elegir un deploy el log se trae a la vista.
	 */
	revealOnSelect: Attachment<HTMLElement> = (element) => {
		if (this.#revealRequest === 0) return
		const stacked = window.matchMedia(`(max-width: ${STACKED_MAX_WIDTH}px)`).matches
		const top = element.getBoundingClientRect().top
		if (!stacked || (top >= 0 && top < window.innerHeight * 0.5)) return
		const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
		element.scrollIntoView({ block: 'start', behavior: reduce ? 'auto' : 'smooth' })
	}

	setFollow = (follow: boolean): void => {
		this.follow = follow
	}

	retry = (): void => {
		void this.deploys.refetch()
	}

	retryLog = (): void => {
		this.#attempt += 1
	}

	openDeploy = (): void => {
		this.deployOpen = true
	}

	closeDeploy = (): void => {
		this.deployOpen = false
	}

	onDeployed = (id: DeployId): void => {
		this.deployOpen = false
		this.select(id)
	}

	#inHistory(id: DeployId | null): boolean {
		return id === null || (this.deploys.data ?? []).some((deploy) => deploy.id === id)
	}

	#queueChunk(text: string): void {
		this.#pendingChunk += text
		this.#cancelFrame ??= onNextFrame(this.#flushChunks)
	}

	#flushChunks = (): void => {
		this.#cancelFrame = null
		if (!this.#pendingChunk) return
		const text = this.#pendingChunk
		this.#pendingChunk = ''
		this.log = appendLogChunk(this.log, text)
	}

	#dropPending(): void {
		this.#cancelFrame?.()
		this.#cancelFrame = null
		this.#pendingChunk = ''
	}

	#announce(deploy: Deploy, status: DeployStatus): void {
		if (status === 'succeeded')
			this.announcement = m.deploy_announce_succeeded({ id: deploy.id })
		else if (status === 'failed')
			this.announcement = m.deploy_announce_failed({ id: deploy.id })
	}

	async #follow(deploy: Deploy, signal: AbortSignal): Promise<void> {
		this.#dropPending()
		this.log = EMPTY_LOG
		this.liveStatus = null
		this.polling = false
		this.logError = null
		this.logLoading = true
		this.follow = true
		this.announcement = ''
		const live = isActiveStatus(deploy.status)
		try {
			for await (const event of followDeployLog(
				{ app: this.ref, id: deploy.id, live },
				signal,
			)) {
				if (signal.aborted) return
				this.logLoading = false
				if (event.kind === 'chunk') {
					this.#queueChunk(event.text)
				} else if (event.kind === 'polling') {
					this.polling = true
				} else if (event.kind === 'reset') {
					// El backend no reconoció el offset: llega de nuevo desde el principio.
					this.#dropPending()
					this.log = EMPTY_LOG
				} else if (event.status !== this.liveStatus) {
					const changed = this.liveStatus !== null || event.status !== deploy.status
					this.liveStatus = event.status
					// El historial (y el Resumen) ya no dicen lo mismo que el stream.
					if (changed) void refreshDeploys(this.#client, this.ref)
					// Uno que se estaba siguiendo en vivo terminó: se anuncia.
					if (live && isTerminalStatus(event.status)) this.#announce(deploy, event.status)
				}
			}
			if (signal.aborted) return
			this.#dropPendingInto()
			this.log = flushLog(this.log)
			this.logLoading = false
		} catch (error) {
			if (signal.aborted) return
			this.#dropPendingInto()
			this.logError = error
			this.logLoading = false
		}
	}

	// Al terminar, lo que quedaba esperando el frame entra ya.
	#dropPendingInto(): void {
		this.#cancelFrame?.()
		this.#flushChunks()
	}
}
