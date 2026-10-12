import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createQuery, type CreateQueryResult, useQueryClient } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { pendingPhase, type RoadmapPhase } from '@/shared/domain'
import {
	ACTIVE_DEPLOY_REFRESH_MS,
	appStatusQuery,
	deploysQuery,
	followDeployLog,
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
	type DeployStatus,
	EMPTY_LOG,
	flushLog,
	hasActiveDeploy,
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

	readonly #props: () => DeploysProps
	readonly #client = useQueryClient()
	#attempt = $state(0)

	// Se crean en el constructor: sus opciones leen `#props`, que un inicializador de campo
	// todavía no tendría.
	readonly deploys: CreateQueryResult<readonly Deploy[], Error>
	/** Solo para saber los servicios en el diálogo de deploy. */
	readonly status: CreateQueryResult<AppStatus, Error>

	readonly selected: Deploy | null = $derived.by(() => {
		const list = this.deploys.data ?? []
		const found = list.find((deploy) => deploy.id === this.selectedId)
		if (found) return found
		// Recién encolado: el historial todavía no lo trae. Se espera al refetch en vez de
		// abrir otro un momento.
		if (this.selectedId !== null && this.deploys.isFetching) return null
		return list[0] ?? null
	})

	// Lo único que reinicia el seguimiento del log: cambiar de deploy (no su estado).
	readonly #selectedKey: DeployId | null = $derived(this.selected?.id ?? null)

	constructor(props: () => DeploysProps) {
		this.#props = props
		this.selectedId = parseDeployId(props().deployParam)
		this.deploys = createQuery(() => ({
			...deploysQuery(this.ref),
			// Mientras haya uno en cola o corriendo, el historial se refresca solo.
			refetchInterval: (query: { state: { data?: readonly Deploy[] | undefined } }) =>
				hasActiveDeploy(query.state.data ?? []) ? ACTIVE_DEPLOY_REFRESH_MS : false,
		}))
		this.status = createQuery(() => appStatusQuery(this.ref))

		$effect(() => {
			const id = this.#selectedKey
			void this.#attempt
			if (id === null) return
			const controller = new AbortController()
			untrack(() => {
				const deploy = this.selected
				if (deploy) void this.#follow(deploy, controller.signal)
			})
			return () => controller.abort()
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

	select = (id: DeployId): void => {
		if (id === this.selected?.id) return
		this.selectedId = id
		this.follow = true
		const { project: p, stage: s, app: a } = this.ref
		const base = resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', { p, s, a })
		// Que la URL diga cuál está abierto (para compartirla o recargar).
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() + `?deploy=`; la regla no acepta query strings
		void goto(`${base}?deploy=${id}`, { replaceState: true, noScroll: true, keepFocus: true })
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

	async #follow(deploy: Deploy, signal: AbortSignal): Promise<void> {
		this.log = EMPTY_LOG
		this.liveStatus = null
		this.polling = false
		this.logError = null
		this.logLoading = true
		this.follow = true
		const live = isActiveStatus(deploy.status)
		try {
			for await (const event of followDeployLog(
				{ app: this.ref, id: deploy.id, live },
				signal,
			)) {
				if (signal.aborted) return
				this.logLoading = false
				if (event.kind === 'chunk') {
					this.log = appendLogChunk(this.log, event.text)
				} else if (event.kind === 'polling') {
					this.polling = true
				} else if (event.status !== this.liveStatus) {
					const changed = this.liveStatus !== null || event.status !== deploy.status
					this.liveStatus = event.status
					// El historial (y el Resumen) ya no dicen lo mismo que el stream.
					if (changed) void refreshDeploys(this.#client, this.ref)
				}
			}
			if (signal.aborted) return
			this.log = flushLog(this.log)
			this.logLoading = false
		} catch (error) {
			if (signal.aborted) return
			this.logError = error
			this.logLoading = false
		}
	}
}
