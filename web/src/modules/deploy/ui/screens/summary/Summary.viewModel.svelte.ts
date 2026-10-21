import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createQuery, type CreateQueryResult } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { withQuery } from '@/shared/ui'
import { type App as CatalogApp, appQuery, appRefFrom } from '@/modules/catalog'
import { appStatusQuery, deploysQuery, STATUS_REFRESH_MS } from '../../../data'
import { healthcheckText } from '../../components/labels/deployLabels'
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
	deployableServices,
	imageOf,
	isCurrentTask,
	latestDeploy,
	type PinnedImages,
	pinnedImages,
	rollbackTarget,
	type ServiceImage,
	type ServiceStatus,
	type ServiceTask,
	splitImageRef,
	stackName,
	stageNetwork,
	swarmServiceName,
} from '../../../domain'

export interface SummaryProps {
	readonly project: string
	readonly stage: string
	readonly app: string
	/**
	 * El `kind` de la app: muestra "Servicio" o "Compose" en el encabezado y le dice al
	 * diálogo de deploy si manda `image` o `images`. Sin él, el de la app en el catálogo
	 * (`appQuery`); si el catálogo no carga, el tipo no se muestra y el diálogo lo deduce de
	 * los servicios.
	 */
	readonly kind?: AppKind | null
	/**
	 * `pending_changes` de la app → la marca "Cambios sin desplegar". Sin él, el de la app en
	 * el catálogo (`appQuery`).
	 */
	readonly pendingChanges?: boolean
	/** A dónde lleva esa marca (ya resuelto con `resolve()`). Sin él, a Configuración. */
	readonly pendingChangesHref?: string
	/**
	 * El healthcheck del spec, ya como texto ("GET /health · cada 10 s"). Sin él, el del spec
	 * de la app en el catálogo; en una compose, o con `none`, no se muestra la métrica.
	 */
	readonly healthcheck?: string | null
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
	/**
	 * La app en el catálogo: el kind y los servicios de la configuración (contrato §2.12), los
	 * cambios sin desplegar y el healthcheck del spec.
	 */
	readonly #catalogApp: CreateQueryResult<CatalogApp, Error>
	/** El deploy por el que ya se volvió a pedir la app al catálogo (una vez por deploy). */
	#catalogRefreshedFor: DeployId | null = null

	constructor(props: () => SummaryProps) {
		this.#props = props
		this.status = createQuery(() =>
			appStatusQuery(this.ref, { refreshEvery: this.refreshInterval }),
		)
		this.deploys = createQuery(() =>
			deploysQuery(this.ref, { refreshEvery: this.refreshInterval }),
		)
		this.#catalogApp = createQuery(() => {
			const { project, stage, app } = this.#props()
			return appQuery(appRefFrom(project, stage, app))
		})
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
		// Un deploy que termina bien adelanta `last_deployed_at` hasta su `started_at`, en la
		// misma transacción (diseño §7 paso 8): si el catálogo es de antes del deploy actual,
		// sus cambios sin desplegar están viejos y se piden de nuevo, una vez por deploy.
		$effect(() => {
			const current = this.current
			const app = this.#catalogApp.data
			if (!current?.startedAt || !app || this.#catalogRefreshedFor === current.id) return
			const deployedAt = app.lastDeployedAt?.getTime() ?? Number.NEGATIVE_INFINITY
			if (deployedAt >= current.startedAt.getTime()) return
			this.#catalogRefreshedFor = current.id
			untrack(() => void this.#catalogApp.refetch())
		})
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
		return this.#props().kind ?? this.#catalogApp.data?.kind ?? null
	}

	get pendingChanges(): boolean {
		return this.#props().pendingChanges ?? this.#catalogApp.data?.pendingChanges ?? false
	}

	get healthcheck(): string | null {
		const fromProps = this.#props().healthcheck
		if (fromProps !== undefined) return fromProps
		const app = this.#catalogApp.data
		return app ? healthcheckText(app) : null
	}

	/** Configuración, donde se ve qué cambió (o lo que pase la ruta). */
	get pendingChangesHref(): string {
		const fromProps = this.#props().pendingChangesHref
		if (fromProps !== undefined) return fromProps
		const { project: p, stage: s, app: a } = this.ref
		return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/configuracion', { p, s, a })
	}

	get stack(): string {
		return stackName(this.ref)
	}

	get network(): string {
		return stageNetwork(this.ref)
	}

	get isLoading(): boolean {
		return this.status.isPending
	}

	get loadError(): unknown {
		return this.status.error
	}

	get health(): AppHealth | null {
		return this.status.data ? appHealth(this.status.data) : null
	}

	get services(): readonly ServiceStatus[] {
		return this.status.data?.services ?? []
	}

	/** Los servicios del diálogo de deploy: los de la configuración o, sin ella, los de Swarm. */
	get deployServices(): readonly string[] {
		return deployableServices(
			this.#catalogApp.data?.services.map((service) => service.name),
			this.services.map((service) => service.name),
		)
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

	/**
	 * La retención (contrato §2.3) siempre guarda el actual y su anterior: con el historial
	 * cargado, si no hay destino de rollback, es que no hay.
	 */
	get historyComplete(): boolean {
		return this.deploys.isSuccess
	}

	/**
	 * Lo que el deploy actual deja fijo en un digest (`requested_images`): desplegar sin
	 * imagen repite esas refs (no vuelve a seguir el tag; contrato §2.2), y el diálogo de
	 * deploy lo avisa.
	 */
	get pinned(): PinnedImages | null {
		return pinnedImages(this.history)
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
		const deployed = current ? imageOf(current, service) : null
		if (deployed) return deployed
		// Sin un deploy exitoso en el historial: lo que corre según Swarm.
		const image = this.status.data?.services.find((item) => item.name === service)?.image
		return image ? { service, ...splitImageRef(image) } : null
	}

	swarmName = (service: string): string => swarmServiceName(this.ref, service)

	get deploysHref(): string {
		const { project: p, stage: s, app: a } = this.ref
		return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', { p, s, a })
	}

	/** Deploys con ese deploy elegido (`?deploy=144`). */
	deployHref = (id: DeployId): string => withQuery(this.deploysHref, { deploy: id })

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

	/**
	 * Encolado: a Deploys, que abre el más reciente. La mutation ya refrescó el historial
	 * antes de volver, así que el más reciente es este.
	 */
	onQueued = async (): Promise<void> => {
		this.deployOpen = false
		this.rollbackOpen = false
		const { project: p, stage: s, app: a } = this.ref
		await goto(resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', { p, s, a }))
	}
}
