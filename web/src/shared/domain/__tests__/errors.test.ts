import { describe, expect, it } from 'vitest'
import { hasCode, NotFoundError, pendingPhase, RateLimitedError, UnavailableError } from '../errors'

describe('UnavailableError', () => {
	it('sin fase es un servicio caído', () => {
		const error = new UnavailableError('caído')

		expect(error.code).toBe('unavailable')
		expect(error.phase).toBeNull()
		expect(pendingPhase(error)).toBeNull()
	})

	it('con fase es un endpoint que todavía no existe, y conserva la causa', () => {
		const cause = new Error('404')
		const error = new UnavailableError('Llega con la Fase 2', { phase: 2, cause })

		expect(error.phase).toBe(2)
		expect(error.cause).toBe(cause)
		expect(pendingPhase(error)).toBe(2)
	})

	it('pendingPhase es null para cualquier otro error', () => {
		expect(pendingPhase(new NotFoundError('no'))).toBeNull()
		expect(pendingPhase('texto')).toBeNull()
	})
})

describe('hasCode', () => {
	it('compara el código de dominio', () => {
		expect(hasCode(new RateLimitedError('espera'), 'rate_limited')).toBe(true)
		expect(hasCode(new RateLimitedError('espera'), 'forbidden')).toBe(false)
		expect(hasCode(new Error('x'), 'unexpected')).toBe(false)
	})
})
