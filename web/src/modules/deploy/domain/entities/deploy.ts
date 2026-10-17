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
	/**
	 * La ref pedida de cada servicio, la que se resolvió (contrato §2.1, `requested_images`).
	 * Tiene digest solo si alguien lo fijó: un rollback, un override con digest o una ref de
	 * la configuración con digest. Vacía hasta que se resolvieron las imágenes. null si el
	 * backend no la manda (uno de antes del cambio).
	 */
	readonly requestedImages: readonly ServiceImage[] | null
	readonly trigger: DeployTrigger
	readonly actor: DeployActor
	/** Por qué falló (`failed`), en una línea. */
	readonly error: string | null
	readonly createdAt: Date
	readonly startedAt: Date | null
	readonly finishedAt: Date | null
	/** Si quedó `superseded`: el deploy más nuevo que lo reemplazó. */
	readonly supersededBy: DeployId | null
	/**
	 * El paso 7 no pudo actualizar las rutas de Caddy (contrato §2.1, `routes_warning`): la
	 * app corre, pero Caddy puede no tener sus rutas nuevas. El motivo está en `GET /proxy`.
	 */
	readonly routesWarning: boolean
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
 * no hay (el backend responde `409 no_previous_deploy`). La retención del historial
 * (contrato §2.3) siempre guarda el actual y su anterior, así que null acá es definitivo.
 */
export function rollbackTarget(deploys: readonly Deploy[]): Deploy | null {
	const succeeded = newestFirst(deploys).filter((deploy) => deploy.status === 'succeeded')
	return succeeded[1] ?? null
}

/** Los servicios que siguen en la versión de un rollback, y cuál (`rollbackPin`). */
export interface RollbackPin {
	readonly rollback: DeployId
	readonly services: readonly string[]
}

function sameImage(a: ResolvedImage, b: ResolvedImage): boolean {
	return a.ref === b.ref && a.digest === b.digest
}

/**
 * Lo que sigue fijado por el último rollback que salió bien. Un rollback pide las refs
 * resueltas (con digest) del deploy al que vuelve, y un deploy sin cuerpo repite la ref
 * pedida del actual (contrato §2.2): el digest pasa de deploy en deploy hasta que llega un
 * override o cambia la ref de la configuración. El backend no expone esa ref pedida
 * (`requested_images`), así que se deduce de las resueltas: un servicio sigue fijado si en
 * el deploy actual tiene la misma imagen (ref y digest) que en el rollback. Null si no
 * queda ninguno, o si no hay un rollback en el historial.
 */
export function rollbackPin(deploys: readonly Deploy[]): RollbackPin | null {
	const current = currentDeploy(deploys)
	const rollback = newestFirst(deploys).find(
		(deploy) => deploy.status === 'succeeded' && deploy.trigger === 'rollback',
	)
	if (!current || !rollback) return null
	const services = current.images
		.filter((image) => {
			const pinned = imageOf(rollback, image.service)
			return pinned !== null && sameImage(pinned, image)
		})
		.map((image) => image.service)
	return services.length > 0 ? { rollback: rollback.id, services } : null
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
