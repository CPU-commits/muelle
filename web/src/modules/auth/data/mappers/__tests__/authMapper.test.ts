import { describe, expect, it } from 'vitest'
import type { InviteToken, NewAccount, TotpCode } from '../../../domain'
import { parseLoginCredentials, parseNewAccount } from '../../../domain'
import {
	codePayload,
	credentialsPayload,
	loginOutcomeFromDto,
	meFromDto,
	setupPayload,
	totpSetupFromDto,
	usernameFromDto,
} from '../authMapper'

describe('authMapper', () => {
	it('traduce me de snake_case al dominio', () => {
		expect(
			meFromDto({ username: 'felipe', totp_enabled: true, needs_totp_setup: false }),
		).toEqual({ username: 'felipe', totpEnabled: true, needsTotpSetup: false })
	})

	it('traduce la respuesta del login', () => {
		expect(loginOutcomeFromDto({ mfa_required: true })).toEqual({ mfaRequired: true })
	})

	it('la URL otpauth pasa a otpauthUrl', () => {
		expect(totpSetupFromDto({ secret: 'ABC', url: 'otpauth://totp/x' })).toEqual({
			secret: 'ABC',
			otpauthUrl: 'otpauth://totp/x',
		})
	})

	it('lee el usuario creado', () => {
		expect(usernameFromDto({ username: 'soporte' })).toBe('soporte')
	})

	it('arma los cuerpos con los nombres exactos que lee el backend (json/v2 estricto)', () => {
		const credentials = parseLoginCredentials({ username: 'Felipe', password: 'x' })
		expect(credentialsPayload(credentials)).toEqual({ username: 'felipe', password: 'x' })

		const account: NewAccount = parseNewAccount({
			username: 'ana',
			password: 'una-contraseña-larga',
		})
		expect(setupPayload('tok' as InviteToken, account)).toEqual({
			token: 'tok',
			username: 'ana',
			password: 'una-contraseña-larga',
		})
		expect(Object.keys(setupPayload('tok' as InviteToken, account)).sort()).toEqual([
			'password',
			'token',
			'username',
		])
		expect(codePayload('123456' as TotpCode)).toEqual({ code: '123456' })
	})
})
