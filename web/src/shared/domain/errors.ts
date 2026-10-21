export type DomainErrorCode =
	| 'validation'
	| 'not_found'
	| 'unauthorized'
	| 'forbidden'
	| 'conflict'
	| 'rate_limited'
	| 'unavailable'
	| 'unexpected'

/** Las fases del roadmap de muelle que el panel muestra antes de que existan en el backend. */
export type RoadmapPhase = 2 | 3

export class DomainError extends Error {
	readonly code: DomainErrorCode

	constructor(code: DomainErrorCode, message: string, options?: ErrorOptions) {
		super(message, options)
		this.name = 'DomainError'
		this.code = code
	}
}

export class ValidationError extends DomainError {
	readonly field: string | undefined

	constructor(message: string, field?: string, options?: ErrorOptions) {
		super('validation', message, options)
		this.name = 'ValidationError'
		this.field = field
	}
}

export class NotFoundError extends DomainError {
	constructor(message: string, options?: ErrorOptions) {
		super('not_found', message, options)
		this.name = 'NotFoundError'
	}
}

export class UnauthorizedError extends DomainError {
	constructor(message: string, options?: ErrorOptions) {
		super('unauthorized', message, options)
		this.name = 'UnauthorizedError'
	}
}

export class ForbiddenError extends DomainError {
	constructor(message: string, options?: ErrorOptions) {
		super('forbidden', message, options)
		this.name = 'ForbiddenError'
	}
}

export class ConflictError extends DomainError {
	constructor(message: string, options?: ErrorOptions) {
		super('conflict', message, options)
		this.name = 'ConflictError'
	}
}

/**
 * `bad_origin`: el backend rechazó una escritura porque el `Origin` no es uno de los
 * permitidos (contrato §0.1): el del panel según Ajustes (`https://<panel_domain>`, con el
 * puerto HTTPS si no es 443) o `MUELLE_PANEL_ORIGIN`, si alguien la puso a mano (detrás de
 * otro proxy, o en desarrollo). Es de dónde se entró, no del usuario.
 */
export class BadOriginError extends ForbiddenError {
	constructor(options?: ErrorOptions) {
		super('muelle no acepta escrituras desde este origen', options)
		this.name = 'BadOriginError'
	}
}

/**
 * El panel se mudó: después de cambiar el puerto HTTPS desde la dirección en que lo servía
 * muelle-caddy, esa dirección ya no pasa el chequeo de `Origin` (contrato §0.1 y §3.5), y el
 * panel no manda escrituras desde ahí (`holdWrites` del cliente HTTP). Es un `bad_origin`
 * que no llegó a salir. `origin` es la dirección nueva. Su texto (`describeError`) también
 * dice qué hacer si la nueva no responde: recargar, si muelle volvió a los puertos anteriores.
 */
export class PanelMovedError extends BadOriginError {
	readonly origin: string

	constructor(origin: string, options?: ErrorOptions) {
		super(options)
		this.message = `El panel se mudó a ${origin}`
		this.name = 'PanelMovedError'
		this.origin = origin
	}
}

/**
 * `ambiguous_session`: el navegador mandó más de una cookie de sesión (otra app de un
 * subdominio plantó la suya). Se arregla borrando las cookies del panel.
 */
export class AmbiguousSessionError extends UnauthorizedError {
	constructor(options?: ErrorOptions) {
		super('Hay más de una cookie de sesión', options)
		this.name = 'AmbiguousSessionError'
	}
}

/** Demasiados intentos (429) o un bloqueo temporal: hay que esperar, no reintentar ya. */
export class RateLimitedError extends DomainError {
	constructor(message: string, options?: ErrorOptions) {
		super('rate_limited', message, options)
		this.name = 'RateLimitedError'
	}
}

/** Por qué no se pudo: sin red, muelle tardó demasiado o muelle respondió un 5xx. */
export type UnavailableReason = 'network' | 'timeout' | 'server'

export interface UnavailableOptions extends ErrorOptions {
	/** La fase en la que llega el endpoint, si no responde porque todavía no existe. */
	phase?: RoadmapPhase
	reason?: UnavailableReason
}

/**
 * El backend no puede atender. Dos casos, que la UI distingue con `phase`:
 * - `phase === null`: caído o sin red. `reason` dice cuál: `network`, `server` (un 5xx)
 *   o `timeout`. Se reintenta, salvo un timeout.
 * - `phase === 2 | 3`: el endpoint es de una fase que el backend todavía no tiene (un 404
 *   de una ruta de deploys, dominios, registries…). No se reintenta y la pantalla
 *   muestra <ComingSoon phase={…} />.
 */
export class UnavailableError extends DomainError {
	readonly phase: RoadmapPhase | null
	readonly reason: UnavailableReason | null

	constructor(message: string, options: UnavailableOptions = {}) {
		const { phase, reason, ...errorOptions } = options
		super('unavailable', message, errorOptions)
		this.name = 'UnavailableError'
		this.phase = phase ?? null
		this.reason = reason ?? null
	}
}

export class UnexpectedError extends DomainError {
	constructor(message: string, options?: ErrorOptions) {
		super('unexpected', message, options)
		this.name = 'UnexpectedError'
	}
}

export function isDomainError(error: unknown): error is DomainError {
	return error instanceof DomainError
}

export function hasCode(error: unknown, code: DomainErrorCode): boolean {
	return isDomainError(error) && error.code === code
}

/** La fase que falta si el error es "todavía no existe"; si no, null. */
export function pendingPhase(error: unknown): RoadmapPhase | null {
	return error instanceof UnavailableError ? error.phase : null
}
