import { describe, expect, it } from 'vitest'
import { AmbiguousSessionError, UnavailableError } from '@/shared/domain'
import { InvalidCredentials, MfaRequired, SessionExpired, sessionStateFromError } from '../..'

describe('sessionStateFromError', () => {
	it('distingue la sesión que espera el código', () => {
		expect(sessionStateFromError(new MfaRequired())).toEqual({ status: 'mfa_pending' })
	})

	it('cualquier otro 401 es "sin sesión"', () => {
		expect(sessionStateFromError(new SessionExpired())).toEqual({ status: 'signed_out' })
		expect(sessionStateFromError(new InvalidCredentials())).toEqual({ status: 'signed_out' })
	})

	it('un backend caído no es "sin sesión": devuelve null', () => {
		expect(sessionStateFromError(new UnavailableError('caído'))).toBeNull()
		expect(sessionStateFromError(new Error('otro'))).toBeNull()
	})

	it('dos cookies de sesión no son "sin sesión": mandar al login no lo arregla', () => {
		expect(sessionStateFromError(new AmbiguousSessionError())).toBeNull()
	})
})
