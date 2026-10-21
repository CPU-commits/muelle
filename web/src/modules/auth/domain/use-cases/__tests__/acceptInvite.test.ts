import { beforeEach, describe, expect, it } from 'vitest'
import { InvalidInvite, InvalidPassword, makeAcceptInvite, UsernameTaken } from '../..'
import { InMemoryAuthRepository } from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('acceptInvite', () => {
	let auth: InMemoryAuthRepository

	beforeEach(() => {
		auth = new InMemoryAuthRepository({
			users: [{ username: 'felipe', password: 'una-contraseña-larga', totpEnabled: false }],
			invites: ['inv-1'],
		})
	})

	it('crea la cuenta, abre la sesión y gasta la invitación', async () => {
		const accept = makeAcceptInvite({ auth })

		await expect(
			accept({ token: 'inv-1', username: 'soporte', password: 'clave-de-soporte' }),
		).resolves.toBe('soporte')
		expect(auth.session?.username).toBe('soporte')
		expect(auth.invites.has('inv-1')).toBe(false)
	})

	it('una invitación usada ya no sirve', async () => {
		const accept = makeAcceptInvite({ auth })
		await accept({ token: 'inv-1', username: 'soporte', password: 'clave-de-soporte' })

		await expect(
			accept({ token: 'inv-1', username: 'otro', password: 'clave-de-soporte' }),
		).rejects.toBeInstanceOf(InvalidInvite)
	})

	it('propaga un nombre tomado', async () => {
		await expect(
			makeAcceptInvite({ auth })({
				token: 'inv-1',
				username: 'felipe',
				password: 'clave-de-soporte',
			}),
		).rejects.toBeInstanceOf(UsernameTaken)
	})

	it('valida la contraseña antes de mandarla', async () => {
		await expect(
			makeAcceptInvite({ auth })({ token: 'inv-1', username: 'soporte', password: 'corta' }),
		).rejects.toBeInstanceOf(InvalidPassword)
		expect(auth.calls.acceptInvite).toBe(0)
	})
})
