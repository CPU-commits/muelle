import { describe, expect, it } from 'vitest'
import { hasCode, RateLimitedError, UnavailableError } from '../errors'

describe('UnavailableError', () => {
	it('es un servicio caído o sin red, con su motivo y su causa', () => {
		const cause = new Error('500')
		const error = new UnavailableError('caído', { reason: 'server', cause })

		expect(error.code).toBe('unavailable')
		expect(error.reason).toBe('server')
		expect(error.cause).toBe(cause)
		expect(new UnavailableError('caído').reason).toBeNull()
		// Ya no hay "fase pendiente": el panel va con el backend entero (contrato §5.5).
		expect(error).not.toHaveProperty('phase')
	})
})

describe('hasCode', () => {
	it('compara el código de dominio', () => {
		expect(hasCode(new RateLimitedError('espera'), 'rate_limited')).toBe(true)
		expect(hasCode(new RateLimitedError('espera'), 'forbidden')).toBe(false)
		expect(hasCode(new Error('x'), 'unexpected')).toBe(false)
	})
})
