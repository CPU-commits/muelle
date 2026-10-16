import { ConflictError, NotFoundError, UnavailableError, ValidationError } from '@/shared/domain'

/** El deploy pedido no existe (o salió de los últimos 20 que se guardan). */
export class DeployNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('Ese deploy no existe', options)
		this.name = 'DeployNotFound'
	}
}

/** Rollback sin un `succeeded` anterior al actual (spec §5.2 regla 6: `409`). */
export class NoPreviousDeploy extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('No hay un deploy exitoso anterior al actual', options)
		this.name = 'NoPreviousDeploy'
	}
}

/**
 * `409 conflict` al desplegar o hacer rollback: la app se está borrando (un `DELETE` de la
 * app en curso, que dura a lo más 2 min; contrato §1.1 y §2.2). No es "ya existe".
 */
export class AppRemoving extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('La app se está borrando', options)
		this.name = 'AppRemoving'
	}
}

/** `503 docker_unavailable`: Docker no responde, o el nodo no es manager de un Swarm. */
export class DockerUnavailable extends UnavailableError {
	constructor(options?: ErrorOptions) {
		super('Docker no responde', { ...options, reason: 'server' })
		this.name = 'DockerUnavailable'
	}
}

/**
 * El stream SSE (log de un deploy o logs en vivo) no abrió o se cortó. El log de un deploy
 * sigue por polling; los logs en vivo ofrecen reconectar.
 */
export class LogStreamFailed extends UnavailableError {
	/** Si llegó a abrir antes de cortarse. */
	readonly opened: boolean

	constructor(opened: boolean, options?: ErrorOptions) {
		super(opened ? 'Se cortó el stream de logs' : 'No se pudo abrir el stream de logs', options)
		this.name = 'LogStreamFailed'
		this.opened = opened
	}
}

/**
 * Por qué el backend rechazó una imagen del deploy (el `422` de `image` o
 * `images.<servicio>`, contrato §2.2):
 * - `both`: vinieron `image` e `images` juntos;
 * - `compose`: una app compose recibe `images`, por servicio;
 * - `unknown_service`: el servicio no existe en la app (según la configuración actual);
 * - `invalid`: la ref no pasa `validate.ImageRef`, o un motivo que el panel no conoce.
 */
export type ImageRejectionReason = 'both' | 'compose' | 'unknown_service' | 'invalid'

export class ImageRejected extends ValidationError {
	/** El servicio del campo (`images.<servicio>`); null si fue `image`. */
	readonly service: string | null
	readonly reason: ImageRejectionReason

	constructor(
		message: string,
		service: string | null,
		reason: ImageRejectionReason,
		options?: ErrorOptions,
	) {
		super(message, service === null ? 'image' : `images.${service}`, options)
		this.name = 'ImageRejected'
		this.service = service
		this.reason = reason
	}
}
