import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createQuery, type CreateQueryResult } from '@tanstack/svelte-query'
import { pendingPhase, type RoadmapPhase } from '@/shared/domain'
import { appStatusQuery, deploysQuery, STATUS_REFRESH_MS } from '../../../data'
import {
	allTasks,
	type AppHealth,
	type AppKind,
	type AppRef,
	type AppStatus,
	appHealth,
	currentDeploy,
	type Deploy,
	type DeployId,
	imageOf,
	isCurrentTask,
	latestDeploy,
	rollbackTarget,
	type ServiceImage,
	type ServiceStatus,
	type ServiceTask,
	stackName,
	stageNetwork,
	swarmServiceName,
} from '../../../domain'

/** Los deploys que guarda el backend por app (spec §5.2 regla 7). */
const HISTORY_LIMIT = 20

export interface SummaryProps {
	readonly project: string
	readonly stage: string
	readonly app: string
	/**
	 * Del catálogo (lo conecta F2): el `kind` de la app. Muestra "Servicio" o "Compose" en el
	 * encabezado y le dice al diálogo de deploy si manda `image` o `images`. Sin él, el tipo
	 * no se muestra y el diálogo lo deduce de los servicios.
	 */
	readonly kind?: AppKind | null
	/** Del catálogo (F2): `pending_changes` de la app → la marca "Cambios sin desplegar". */
	readonly pendingChanges?: boolean
	/** A dónde lleva esa marca (ya resuelto con `resolve()`); sin él, no es un link. */
	readonly pendingChangesHref?: string
}

export interface TaskRow {
	readonly key: string
	readonly service: string
	readonly task: ServiceTask
	readonly current: boolean
}

function isVisible(): boolean {
	return typeof document === 'undefined' || document.visibilityState === 'visible'
}

// El Resumen de una app (App-Resumen.dc.html): el encabezado grande (nombre, estado,
// tipo, stack y red, Rollback y Desplegar), réplicas, CPU/RAM, imagen y digest, y las
// tareas. El estado se refresca cada 5 s, y solo con la pestaña visible.
export class SummaryViewModel {
	/** `document.visibilityState === 'visible'`: con la pestaña oculta no se refresca nada. */
	visible = $state(isVisible())
	deployOpen = $state(false)
	rollbackOpen = $state(false)

	readonly #props: () => SummaryProps

	/** Cada cuánto se refrescan el estado y el historial: 5 s, o nunca con la pestaña oculta. */
	readonly refreshInterval: number | false = $derived(this.visible ? STATUS_REFRESH_MS : false)

	// Se crean en el constructor: las queries leen `#props`, que un inicializador de campo
	// todavía no tendría, y `tasks` lee la query.
	readonly status: CreateQueryResult<AppStatus, Error>
	readonly deploys: CreateQueryResult<readonly Deploy[], Error>
	readonly tasks: TaskRow[]

	constructor(props: () => SummaryProps) {
		this.#props = props
		this.status = createQuery(() => ({
			...appStatusQuery(this.ref),
			refetchInterval: this.refreshInterval,
			refetchIntervalInBackground: false,
		}))
		this.deploys = createQuery(() => ({
			...deploysQuery(this.ref),
			refetchInterval: this.refreshInterval,
			refetchIntervalInBackground: false,
		}))
		this.tasks = $derived(
			this.status.data
				? allTasks(this.status.data).map(({ service, task }) => ({
						key: `${service}/${task.id}`,
						service,
						task,
						current: isCurrentTask(task),
					}))
				: [],
		)
		$effect(() => {
			const onVisibility = (): void => {
				const visible = isVisible()
				this.visible = visible
				// Al volver a la pestaña, el dato puede tener minutos: se pide ya.
				if (visible) this.refresh()
			}
			document.addEventListener('visibilitychange', onVisibility)
			return () => document.removeEventListener('visibilitychange', onVisibility)
		})
	}

	get ref(): AppRef {
		const { project, stage, app } = this.#props()
		return { project, stage, app }
	}

	get kind(): AppKind | null {
		return this.#props().kind ?? null
	}

	get pendingChanges(): boolean {
		return this.#props().pendingChanges ?? false
	}

	get pendingChangesHref(): string | null {
		return this.#props().pendingChangesHref ?? null
	}

	get stack(): string {
		return stackName(this.ref)
	}

	get network(): string {
		return stageNetwork(this.ref)
	}

	/** La fase que falta si el backend todavía no tiene estos endpoints. */
	get pendingPhase(): RoadmapPhase | null {
		return pendingPhase(this.status.error) ?? pendingPhase(this.deploys.error)
	}

	get isLoading(): boolean {
		return this.status.isPending
	}

	/** Un error de verdad (no "llega con la Fase 2"). */
	get loadError(): unknown {
		return this.pendingPhase === null ? this.status.error : null
	}

	get health(): AppHealth | null {
		return this.status.data ? appHealth(this.status.data) : null
	}

	get services(): readonly ServiceStatus[] {
		return this.status.data?.services ?? []
	}

	get serviceNames(): string[] {
		return this.services.map((service) => service.name)
	}

	get history(): readonly Deploy[] {
		return this.deploys.data ?? []
	}

	get current(): Deploy | null {
		return currentDeploy(this.history)
	}

	get last(): Deploy | null {
		return latestDeploy(this.history)
	}

	get rollbackTarget(): Deploy | null {
		return rollbackTarget(this.history)
	}

	get historyComplete(): boolean {
		return this.deploys.isSuccess && this.history.length < HISTORY_LIMIT
	}

	/** La imagen actual de cada servicio (para el placeholder del diálogo). */
	get currentImages(): Record<string, string> {
		return Object.fromEntries(
			(this.current?.images ?? []).map((image) => [image.service, image.ref]),
		)
	}

	get runningTasks(): number {
		return this.tasks.filter((row) => row.current && row.task.state === 'running').length
	}

	get pastTasks(): number {
		return this.tasks.filter((row) => !row.current).length
	}

	/** Se puede desplegar o hacer rollback cuando el backend tiene la Fase 2. */
	get actionsEnabled(): boolean {
		return this.status.isSuccess
	}

	imageOf = (service: string): ServiceImage | null => {
		const current = this.current
		return current ? imageOf(current, service) : null
	}

	swarmName = (service: string): string => swarmServiceName(this.ref, service)

	get deploysHref(): string {
		const { project: p, stage: s, app: a } = this.ref
		return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', { p, s, a })
	}

	/** Deploys con ese deploy elegido (`?deploy=144`). */
	deployHref = (id: DeployId): string => `${this.deploysHref}?deploy=${id}`

	refresh = (): void => {
		void this.status.refetch()
		void this.deploys.refetch()
	}

	openDeploy = (): void => {
		this.deployOpen = true
	}

	closeDeploy = (): void => {
		this.deployOpen = false
	}

	openRollback = (): void => {
		this.rollbackOpen = true
	}

	closeRollback = (): void => {
		this.rollbackOpen = false
	}

	/** Encolado: a Deploys, con su log abierto. */
	onQueued = async (id: DeployId): Promise<void> => {
		this.deployOpen = false
		this.rollbackOpen = false
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- deployHref es resolve() + `?deploy=`; la regla no acepta query strings
		await goto(this.deployHref(id))
	}
}
