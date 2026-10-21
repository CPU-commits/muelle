import { describe, expect, it } from 'vitest'
import { makeResetUserTotp, UserNotFound } from '../..'
import {
	fakeUser,
	InMemoryIdentityRepository,
} from '../../repositories/__fakes__/inMemoryIdentityRepository'

describe('resetUserTotp', () => {
	it('quita el 2FA y cierra las sesiones de ese usuario', async () => {
		const repo = new InMemoryIdentityRepository({
			users: [fakeUser({ username: 'felipe' }), fakeUser({ username: 'soporte' })],
		})

		await makeResetUserTotp({ users: repo.asUsers() })('soporte')

		expect(repo.users.find((user) => user.username === 'soporte')?.totpEnabled).toBe(false)
		expect(repo.users.find((user) => user.username === 'felipe')?.totpEnabled).toBe(true)
		expect(repo.closedSessions).toEqual(['soporte'])
	})

	it('propaga que el usuario ya no existe', async () => {
		const repo = new InMemoryIdentityRepository()

		await expect(makeResetUserTotp({ users: repo.asUsers() })('nadie')).rejects.toBeInstanceOf(
			UserNotFound,
		)
	})
})
