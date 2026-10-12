import { describe, expect, it } from 'vitest'
import {
	ForbiddenError,
	UnexpectedError,
	NotFoundError,
	RateLimitedError,
	UnavailableError,
	ValidationError,
} from '@/shared/domain'
import { ApiError, ContractError, NetworkError } from '../http/errors'
import { shouldRetry } from '../query'

describe('shouldRetry', () => {
	it('no reintenta un error de dominio not_found (lo que llega de un 404)', () => {
		const error = new NotFoundError('no existe', {
			cause: new ApiError(404, 'not_found', 'no existe'),
		})
		expect(shouldRetry(0, error)).toBe(false)
	})

	it('no reintenta validación, permisos ni un límite de intentos', () => {
		expect(shouldRetry(0, new ValidationError('mal', 'name'))).toBe(false)
		expect(shouldRetry(0, new ForbiddenError('no'))).toBe(false)
		expect(shouldRetry(0, new RateLimitedError('espera'))).toBe(false)
	})

	it('reintenta un servicio caído hasta dos veces', () => {
		const error = new UnavailableError('caído')
		expect(shouldRetry(0, error)).toBe(true)
		expect(shouldRetry(1, error)).toBe(true)
		expect(shouldRetry(2, error)).toBe(false)
	})

	it('no reintenta un endpoint de una fase que todavía no existe', () => {
		expect(shouldRetry(0, new UnavailableError('Llega con la Fase 2', { phase: 2 }))).toBe(
			false,
		)
	})

	it('sigue cortando un ApiError 4xx crudo y reintenta un error de red', () => {
		expect(shouldRetry(0, new ApiError(422, 'invalid', 'slug: no vale'))).toBe(false)
		expect(shouldRetry(0, new NetworkError(new Error('offline')))).toBe(true)
	})

	it('no reintenta ningún 4xx ni una respuesta con otra forma', () => {
		expect(shouldRetry(0, new ApiError(400, 'invalid_json', 'x'))).toBe(false)
		expect(shouldRetry(0, new ApiError(410, 'unknown', 'x'))).toBe(false)
		expect(shouldRetry(0, new ContractError('auth.me', 'username: requerido'))).toBe(false)
		expect(shouldRetry(0, new UnexpectedError('Error inesperado (400 invalid_json)'))).toBe(
			false,
		)
		expect(shouldRetry(0, new ApiError(502, 'unknown', 'x'))).toBe(true)
	})
})
