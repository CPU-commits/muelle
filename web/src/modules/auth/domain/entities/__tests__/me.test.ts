import { describe, expect, it } from 'vitest'
import { landingFor } from '../..'
import { fakeMe } from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('landingFor', () => {
	it('manda a configurar 2FA si el panel la exige y falta', () => {
		expect(landingFor(fakeMe({ needsTotpSetup: true }))).toBe('totp_setup')
	})

	it('si no, al inicio', () => {
		expect(landingFor(fakeMe())).toBe('home')
		expect(landingFor(fakeMe({ totpEnabled: true }))).toBe('home')
	})
})
