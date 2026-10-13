import { createQuery, type CreateQueryResult } from '@tanstack/svelte-query'
import { formatRelative, type StatusTone } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { appStatusQuery, deploysQuery } from '../../../data'
import {
	type AppHealth,
	type AppRef,
	type AppStatus,
	appHealth,
	currentDeploy,
	type Deploy,
	imageName,
	latestDeploy,
	replicaCount,
	weakestService,
} from '../../../domain'
import { healthLabel, healthTone } from '../../components/labels/deployLabels'

// Para la fila de una app en Proyectos (F2, Main.dc.html): las columnas "estado" e
// "imagen" (de `GET …/status` y del último deploy exitoso) y "último deploy". Se crea en
// el componente de la fila, como un viewModel:
//
//   const state = new AppDeployState(() => ({ project, stage, app }))
//   <StatusDot status={state.tone} />  {state.statusText}  {state.lastDeployText}
//
// Si el backend no tiene la Fase 2 (o falla), todo dice "—" y no hay error: la lista de
// proyectos funciona igual. No se refresca solo.
//
// `enabled` (opcional) frena los pedidos mientras dé false: una fila que todavía no se ve,
// o un snippet de `<ShellHeader>` que puede evaluarse con los params vacíos
// (`new AppDeployState(() => ref, () => Boolean(p && s && a))`). Apagado, todo dice "—".
export class AppDeployState {
	readonly #ref: () => AppRef
	readonly #enabled: () => boolean
	readonly #status: CreateQueryResult<AppStatus, Error>
	// El historial entero (el mismo cache que `lastDeployQuery`): de ahí salen el último
	// deploy y el actual (el succeeded más reciente, que es el que corre).
	readonly #deploys: CreateQueryResult<readonly Deploy[], Error>

	constructor(ref: () => AppRef, enabled: () => boolean = () => true) {
		this.#ref = ref
		this.#enabled = enabled
		this.#status = createQuery(() =>
			appStatusQuery(this.#ref(), { enabled: this.#enabled(), noRetry: true }),
		)
		this.#deploys = createQuery(() =>
			deploysQuery(this.#ref(), { enabled: this.#enabled(), noRetry: true }),
		)
	}

	/** Cargando de verdad (apagada no carga: muestra "—"). */
	get isLoading(): boolean {
		return this.#enabled() && (this.#status.isPending || this.#deploys.isPending)
	}

	/** El estado no está disponible (Fase 2 pendiente, Docker caído, error): se muestra "—". */
	get statusUnavailable(): boolean {
		return this.#status.isError
	}

	get health(): AppHealth | null {
		return this.#status.data ? appHealth(this.#status.data) : null
	}

	/** El punto de la fila. `idle` también mientras carga o si no hay dato. */
	get tone(): StatusTone {
		const health = this.health
		return health ? healthTone(health) : 'idle'
	}

	/** "Corriendo · 2/2", "Degradada · 1/2", "Caída · gateway 0/1", "Sin desplegar" o "—". */
	get statusText(): string {
		const status = this.#status.data
		const health = this.health
		if (!status || !health) return m.deploy_none()
		if (health === 'not_deployed') return healthLabel(health)
		const weakest = weakestService(status)
		if (weakest && status.services.length > 1) {
			return m.deploy_row_status_service({
				health: healthLabel(health),
				service: weakest.name,
				running: weakest.runningReplicas,
				desired: weakest.desiredReplicas,
			})
		}
		const { running, desired } = replicaCount(status)
		return m.deploy_row_status({ health: healthLabel(health), running, desired })
	}

	/** El último deploy, de cualquier estado; null si no hay o no se sabe. */
	get lastDeploy(): Deploy | null {
		return this.#deploys.data ? latestDeploy(this.#deploys.data) : null
	}

	/** El que corre: el succeeded más reciente. */
	get currentDeploy(): Deploy | null {
		return this.#deploys.data ? currentDeploy(this.#deploys.data) : null
	}

	/** "#143 · hace 12 min", "Sin deploys" o "—". */
	get lastDeployText(): string {
		if (this.#deploys.isError || this.#deploys.isPending) return m.deploy_none()
		const deploy = this.lastDeploy
		if (!deploy) return m.deploy_row_no_deploys()
		return m.deploy_row_last({ id: deploy.id, when: formatRelative(deploy.createdAt) })
	}

	/** El último terminó `failed`: la fila agrega " · falló" en rojo (`lastDeployFailedText`). */
	get lastDeployFailed(): boolean {
		return this.lastDeploy?.status === 'failed'
	}

	get lastDeployFailedText(): string {
		return m.deploy_row_failed()
	}

	/**
	 * Lo que corre según el deploy actual: la ref de una app de formulario, o los servicios
	 * de una compose ("gateway · runner"). Null si no se sabe: la fila usa la imagen del
	 * spec del catálogo.
	 */
	get image(): string | null {
		const images = this.currentDeploy?.images ?? []
		if (images.length === 0) return null
		if (images.length === 1) return images[0]?.ref ?? null
		return images.map((image) => image.service).join(' · ')
	}

	/** Igual que `image`, más corto: solo el nombre (`backend-app:sha-3f9c2e1`). */
	get imageShort(): string | null {
		const image = this.image
		return image && !image.includes(' · ') ? imageName(image) : image
	}
}
