import type { DeployId } from '../value-objects/deployId'
import type { ResolvedImage } from '../value-objects/imageRef'
import {
	type ActorType,
	type DeployStatus,
	type DeployTrigger,
	isActiveStatus,
} from '../value-objects/deployStatus'

export interface ServiceImage extends ResolvedImage {
	/** El servicio del compose (en una app de formulario, el slug de la app). */
	readonly service: string
}

export interface DeployActor {
	readonly type: ActorType
	/** El usuario o el nombre del token al crear el deploy; null si el backend no lo guardó. */
	readonly name: string | null
}

// Una fila de `deploys` (spec §5.1), sin el log: ese se lee aparte (§10).
export interface Deploy {
	readonly id: DeployId
	readonly status: DeployStatus
	readonly images: readonly ServiceImage[]
	readonly trigger: DeployTrigger
	readonly actor: DeployActor
	/** Por qué falló (`failed`), en una línea. */
	readonly error: string | null
	readonly createdAt: Date
	readonly startedAt: Date | null
	readonly finishedAt: Date | null
	/** Si quedó `superseded`: el deploy más nuevo que lo reemplazó. */
	readonly supersededBy: DeployId | null
}

/** Lo que responde `POST …/deploys` y `POST …/rollback` (202). */
export interface DeployAccepted {
	readonly id: DeployId
}

// Las listas vienen del más nuevo al más viejo (como las devuelve `GET …/deploys`), pero
// las funciones no lo asumen: ordenan por id, que es autoincremental.
function newestFirst(deploys: readonly Deploy[]): Deploy[] {
	return [...deploys].sort((a, b) => b.id - a.id)
}

/** El más reciente, sea cual sea su estado ("Último deploy"). */
export function latestDeploy(deploys: readonly Deploy[]): Deploy | null {
	return newestFirst(deploys)[0] ?? null
}

/** El deploy "actual": el `succeeded` más reciente (spec §5.2 regla 6). */
export function currentDeploy(deploys: readonly Deploy[]): Deploy | null {
	return newestFirst(deploys).find((deploy) => deploy.status === 'succeeded') ?? null
}

/**
 * A dónde vuelve un rollback: el `succeeded` inmediatamente anterior al actual. Null si
 * no hay (el backend responde 409). El historial guarda los últimos 20, así que null acá
 * no garantiza el 409: el backend decide.
 */
export function rollbackTarget(deploys: readonly Deploy[]): Deploy | null {
	const succeeded = newestFirst(deploys).filter((deploy) => deploy.status === 'succeeded')
	return succeeded[1] ?? null
}

export function hasActiveDeploy(deploys: readonly Deploy[]): boolean {
	return deploys.some((deploy) => isActiveStatus(deploy.status))
}

/**
 * Cuánto duró (o lleva) en ms: desde que empezó hasta que terminó, o hasta `nowMs` si
 * sigue corriendo. Null si nunca empezó (en cola o reemplazado).
 */
export function deployDurationMs(deploy: Deploy, nowMs: number = Date.now()): number | null {
	if (!deploy.startedAt) return null
	const end = deploy.finishedAt?.getTime() ?? nowMs
	return Math.max(0, end - deploy.startedAt.getTime())
}

/** La imagen de un servicio en este deploy, si la tiene. */
export function imageOf(deploy: Deploy, service: string): ServiceImage | null {
	return deploy.images.find((image) => image.service === service) ?? null
}
