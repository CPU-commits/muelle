import { describe, expect, it } from 'vitest'
import {
	AmbiguousSessionError,
	BadOriginError,
	ConflictError,
	ForbiddenError,
	NotFoundError,
	RateLimitedError,
	ReadOnlyTokenError,
	UnauthorizedError,
	UnavailableError,
	UnexpectedError,
	ValidationError,
} from '@/shared/domain'
import { toDomainError, withDomainErrors } from '../domainErrors'
import { ApiError, ContractError, NetworkError, RequestTimeoutError } from '../errors'

const apiError = (status: number, code: string, message = 'mensaje') =>
	new ApiError(status, code, message)

class ProjectNotEmpty extends ConflictError {}
class ProjectNotFound extends NotFoundError {}

describe('toDomainError', () => {
	it('traduce cada status a su error de dominio', () => {
		expect(toDomainError(apiError(401, 'unauthenticated'))).toBeInstanceOf(UnauthorizedError)
		expect(toDomainError(apiError(403, 'forbidden'))).toBeInstanceOf(ForbiddenError)
		expect(toDomainError(apiError(404, 'not_found'))).toBeInstanceOf(NotFoundError)
		expect(toDomainError(apiError(409, 'conflict'))).toBeInstanceOf(ConflictError)
		expect(toDomainError(apiError(423, 'locked'))).toBeInstanceOf(RateLimitedError)
		expect(toDomainError(apiError(429, 'rate_limited'))).toBeInstanceOf(RateLimitedError)
		expect(toDomainError(apiError(503, 'unknown'))).toBeInstanceOf(UnavailableError)
		expect(toDomainError(apiError(400, 'invalid_json'))).toBeInstanceOf(UnexpectedError)
	})

	it('un error de red es un servicio no disponible', () => {
		const error = toDomainError(new NetworkError(new Error('offline')))

		expect(error).toBeInstanceOf(UnavailableError)
	})

	it('saca el campo de un 422 "campo: motivo" y lo renombra', () => {
		const error = toDomainError(
			apiError(422, 'invalid', 'deploy_timeout_s: debe ser positivo'),
			{
				fields: { deploy_timeout_s: 'deployTimeoutSeconds' },
			},
		)

		expect(error).toBeInstanceOf(ValidationError)
		expect((error as ValidationError).field).toBe('deployTimeoutSeconds')
		expect((error as ValidationError).message).toBe('debe ser positivo')
	})

	it('un 422 sin campo queda como validación general', () => {
		const error = toDomainError(apiError(422, 'invalid', 'el compose no es válido'))

		expect((error as ValidationError).field).toBeUndefined()
		expect((error as ValidationError).message).toBe('el compose no es válido')
	})

	it('los códigos propios del módulo ganan sobre el status', () => {
		const error = toDomainError(apiError(409, 'not_empty'), {
			codes: { not_empty: () => new ProjectNotEmpty('tiene stages') },
		})

		expect(error).toBeInstanceOf(ProjectNotEmpty)
	})

	it('usa el notFound del módulo para un 404', () => {
		const error = toDomainError(apiError(404, 'not_found'), {
			notFound: () => new ProjectNotFound('no existe auditfy'),
		})

		expect(error).toBeInstanceOf(ProjectNotFound)
	})

	it('una ruta que no existe (404 no_route, 405 method_not_allowed) es un error inesperado', () => {
		// El panel va embebido en el mismo binario que el backend (contrato §5.5): una ruta que
		// no existe es una ruta mal armada del panel, no un recurso que no está.
		const notFound = () => new ProjectNotFound('no existe auditfy')
		const route = toDomainError(apiError(404, 'no_route'), { notFound })
		const method = toDomainError(apiError(405, 'method_not_allowed'), { notFound })

		expect(route).toBeInstanceOf(UnexpectedError)
		expect(method).toBeInstanceOf(UnexpectedError)
		expect(route).not.toBeInstanceOf(NotFoundError)
		// Un recurso que no existe sigue siendo el notFound del módulo.
		expect(toDomainError(apiError(404, 'not_found'), { notFound })).toBeInstanceOf(
			ProjectNotFound,
		)
	})

	it('bad_origin y ambiguous_session tienen su propio error, en cualquier módulo', () => {
		expect(toDomainError(apiError(403, 'bad_origin'))).toBeInstanceOf(BadOriginError)
		expect(toDomainError(apiError(400, 'ambiguous_session'))).toBeInstanceOf(
			AmbiguousSessionError,
		)
	})

	it('read_only_token (un token de solo lectura que escribe) es un 403 con su propio error', () => {
		const error = toDomainError(apiError(403, 'read_only_token', 'el token es de solo lectura'))

		expect(error).toBeInstanceOf(ReadOnlyTokenError)
		expect(error).toBeInstanceOf(ForbiddenError)
	})

	it('una petición que tardó demasiado es un servicio no disponible', () => {
		const error = toDomainError(new RequestTimeoutError('/projects', 30_000))

		expect(error).toBeInstanceOf(UnavailableError)
	})

	it('un UnavailableError dice por qué: la red, un timeout o un 5xx de muelle', () => {
		const reason = (error: unknown) => (error as UnavailableError).reason

		expect(reason(toDomainError(new NetworkError(new Error('offline'))))).toBe('network')
		expect(reason(toDomainError(new RequestTimeoutError('/projects', 30_000)))).toBe('timeout')
		expect(reason(toDomainError(apiError(500, 'internal')))).toBe('server')
		expect(reason(toDomainError(apiError(503, 'unknown')))).toBe('server')
	})

	it('deja pasar lo que no es un ApiError', () => {
		const contract = new ContractError('auth.me', 'username: requerido')
		expect(toDomainError(contract)).toBe(contract)
	})
})

describe('withDomainErrors', () => {
	it('devuelve el resultado o tira el error traducido', async () => {
		await expect(withDomainErrors(async () => 42)).resolves.toBe(42)
		await expect(
			withDomainErrors(async () => {
				throw apiError(404, 'not_found')
			}),
		).rejects.toBeInstanceOf(NotFoundError)
	})
})
