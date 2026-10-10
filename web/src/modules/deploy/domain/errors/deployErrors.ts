import { ConflictError, NotFoundError, UnavailableError } from '@/shared/domain'

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
