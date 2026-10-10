import { describe, expect, it } from 'vitest'
import {
	ConflictError,
	ForbiddenError,
	NotFoundError,
	pendingPhase,
	RateLimitedError,
	UnauthorizedError,
	UnavailableError,
	UnexpectedError,
	ValidationError,
} from '@/shared/domain'
import { toDomainError, withDomainErrors } from '../domainErrors'
import { ApiError, ContractError, NetworkError } from '../errors'

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

	it('un error de red es un servicio no disponible, sin fase', () => {
		const error = toDomainError(new NetworkError(new Error('offline')))

		expect(error).toBeInstanceOf(UnavailableError)
		expect(pendingPhase(error)).toBeNull()
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

	it('en un endpoint de Fase 2/3, un 404 o 405 es "todavía no existe"', () => {
		const notYet = toDomainError(apiError(404, 'not_found'), { phase: 2 })
		const method = toDomainError(apiError(405, 'method_not_allowed'), { phase: 3 })

		expect(notYet).toBeInstanceOf(UnavailableError)
		expect(pendingPhase(notYet)).toBe(2)
		expect(pendingPhase(method)).toBe(3)
		expect(pendingPhase(toDomainError(apiError(503, 'unknown'), { phase: 2 }))).toBeNull()
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
