import { describe, expect, it } from 'vitest'
import { AlreadySetUp, InvalidInvite, InvalidUsername, makeSetup, MissingInviteToken } from '../..'
import { InMemoryAuthRepository } from '../../repositories/__fakes__/inMemoryAuthRepository'

const account = { username: 'Felipe', password: 'una-contraseña-larga' }

describe('setup', () => {
	it('crea el primer usuario y abre su sesión', async () => {
		const auth = new InMemoryAuthRepository({ setupToken: 'tok' })

		await expect(makeSetup({ auth })({ token: 'tok', ...account })).resolves.toBe('felipe')
		expect(auth.session?.username).toBe('felipe')
	})

	it('sin token del link no llama al backend', async () => {
		const auth = new InMemoryAuthRepository()

		await expect(makeSetup({ auth })({ token: null, ...account })).rejects.toBeInstanceOf(
			MissingInviteToken,
		)
		expect(auth.calls.setup).toBe(0)
	})

	it('valida la cuenta antes de mandarla', async () => {
		const auth = new InMemoryAuthRepository({ setupToken: 'tok' })

		await expect(
			makeSetup({ auth })({ token: 'tok', username: 'x', password: account.password }),
		).rejects.toBeInstanceOf(InvalidUsername)
		expect(auth.calls.setup).toBe(0)
	})

	it('propaga un token que no vale y un setup ya hecho', async () => {
		const empty = new InMemoryAuthRepository({ setupToken: 'tok' })
		await expect(
			makeSetup({ auth: empty })({ token: 'otro', ...account }),
		).rejects.toBeInstanceOf(InvalidInvite)

		const done = new InMemoryAuthRepository({
			users: [{ username: 'ana', password: 'otra-contraseña-larga', totpEnabled: false }],
		})
		await expect(
			makeSetup({ auth: done })({ token: 'setup-token', ...account }),
		).rejects.toBeInstanceOf(AlreadySetUp)
	})
})
