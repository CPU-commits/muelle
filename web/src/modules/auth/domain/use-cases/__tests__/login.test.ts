import { beforeEach, describe, expect, it } from 'vitest'
import { InvalidCredentials, InvalidPassword, InvalidUsername, makeLogin } from '../..'
import { InMemoryAuthRepository } from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('login', () => {
	let auth: InMemoryAuthRepository

	beforeEach(() => {
		auth = new InMemoryAuthRepository({
			users: [
				{ username: 'felipe', password: 'una-contraseña-larga', totpEnabled: false },
				{ username: 'ana', password: 'otra-contraseña-larga', totpEnabled: true },
			],
		})
	})

	it('sin 2FA abre la sesión completa', async () => {
		const login = makeLogin({ auth })

		await expect(
			login({ username: 'felipe', password: 'una-contraseña-larga' }),
		).resolves.toEqual({ mfaRequired: false })
		expect(auth.session).toMatchObject({ username: 'felipe', mfaPending: false })
	})

	it('con 2FA deja la sesión esperando el código', async () => {
		const login = makeLogin({ auth })

		await expect(
			login({ username: 'ana', password: 'otra-contraseña-larga' }),
		).resolves.toEqual({ mfaRequired: true })
		expect(auth.session?.mfaPending).toBe(true)
	})

	it('normaliza el usuario antes de mandarlo', async () => {
		const login = makeLogin({ auth })

		await login({ username: '  FELIPE ', password: 'una-contraseña-larga' })

		expect(auth.session?.username).toBe('felipe')
	})

	it('no llama al backend con campos vacíos', async () => {
		const login = makeLogin({ auth })

		await expect(login({ username: ' ', password: 'x' })).rejects.toBeInstanceOf(
			InvalidUsername,
		)
		await expect(login({ username: 'felipe', password: '' })).rejects.toBeInstanceOf(
			InvalidPassword,
		)
		expect(auth.calls.login).toBe(0)
	})

	it('propaga InvalidCredentials', async () => {
		const login = makeLogin({ auth })

		await expect(login({ username: 'felipe', password: 'mala' })).rejects.toBeInstanceOf(
			InvalidCredentials,
		)
	})
})
