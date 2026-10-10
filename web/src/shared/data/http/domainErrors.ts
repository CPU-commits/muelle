import {
	ConflictError,
	ForbiddenError,
	NotFoundError,
	RateLimitedError,
	type RoadmapPhase,
	UnauthorizedError,
	UnavailableError,
	UnexpectedError,
	ValidationError,
} from '@/shared/domain'
import { ApiError, NetworkError } from './errors'

// La frontera donde el status HTTP muere (regla 4 del CLAUDE.md). Cada repositorio pasa
// sus errores por acá: arriba nadie ve un 404, ven un error de dominio con su `code`.
export interface DomainErrorMap {
	/** Errores propios por `error.code` del backend (`not_empty` → `ProjectNotEmpty`). Ganan siempre. */
	readonly codes?: Readonly<Record<string, (error: ApiError) => Error>>
	/** Qué tirar ante un 404 del recurso (`ProjectNotFound`). Por defecto, `NotFoundError`. */
	readonly notFound?: (error: ApiError) => Error
	/**
	 * Nombre de dominio de los campos de un 422 (`{ deploy_timeout_s: 'deployTimeoutSeconds' }`).
	 * El backend responde `invalid` con el mensaje `"<campo>: <motivo>"`; el campo sale en
	 * `ValidationError.field`.
	 */
	readonly fields?: Readonly<Record<string, string>>
	/**
	 * Endpoints de una fase que el backend todavía no tiene (deploys, dominios, registries…).
	 * Un 404 o 405 ahí quiere decir que la RUTA no existe: se traduce a
	 * `UnavailableError` con `phase`, y la pantalla muestra <ComingSoon>.
	 */
	readonly phase?: RoadmapPhase
}

const FIELD_MESSAGE = /^([A-Za-z0-9_.[\]-]+): (.+)$/s

function validationError(error: ApiError, fields: DomainErrorMap['fields']): ValidationError {
	const match = FIELD_MESSAGE.exec(error.message)
	if (!match) return new ValidationError(error.message, undefined, { cause: error })
	const [, field = '', reason = error.message] = match
	return new ValidationError(reason, fields?.[field] ?? field, { cause: error })
}

export function toDomainError(error: unknown, map: DomainErrorMap = {}): unknown {
	if (error instanceof NetworkError) {
		return new UnavailableError('No se pudo contactar al servidor', { cause: error })
	}
	// Un ContractError, un AbortError o un error de dominio ya traducido pasan tal cual.
	if (!(error instanceof ApiError)) return error

	const byCode = map.codes?.[error.code]
	if (byCode) return byCode(error)

	if (map.phase !== undefined && (error.status === 404 || error.status === 405)) {
		return new UnavailableError(`Llega con la Fase ${map.phase}`, {
			phase: map.phase,
			cause: error,
		})
	}

	switch (error.status) {
		case 401:
			return new UnauthorizedError(error.message, { cause: error })
		case 403:
			return new ForbiddenError(error.message, { cause: error })
		case 404:
			return map.notFound?.(error) ?? new NotFoundError(error.message, { cause: error })
		case 409:
			return new ConflictError(error.message, { cause: error })
		case 422:
			return validationError(error, map.fields)
		case 423:
		case 429:
			return new RateLimitedError(error.message, { cause: error })
		case 500:
		case 502:
		case 503:
		case 504:
			return new UnavailableError('El servicio no está disponible', { cause: error })
		default:
			return new UnexpectedError(`Error inesperado (${error.status} ${error.code})`, {
				cause: error,
			})
	}
}

/** `toDomainError` alrededor de una operación: `withDomainErrors(() => ds.get(id), map)`. */
export async function withDomainErrors<T>(
	operation: () => Promise<T>,
	map?: DomainErrorMap,
): Promise<T> {
	try {
		return await operation()
	} catch (error) {
		throw toDomainError(error, map)
	}
}
