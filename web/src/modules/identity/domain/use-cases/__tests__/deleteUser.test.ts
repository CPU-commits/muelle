import { beforeEach, describe, expect, it } from 'vitest'
import { CannotDeleteSelf, makeDeleteUser, UserNotFound } from '../..'
import {
	fakeToken,
	fakeUser,
	InMemoryIdentityRepository,
} from '../../repositories/__fakes__/inMemoryIdentityRepository'

describe('deleteUser', () => {
	let repo: InMemoryIdentityRepository

	beforeEach(() => {
		repo = new InMemoryIdentityRepository({
			users: [fakeUser({ username: 'felipe' }), fakeUser({ username: 'soporte' })],
			tokens: [fakeToken({ createdBy: { kind: 'user', username: 'soporte' } })],
		})
	})

	it('borra a otro usuario y sus tokens quedan, sin creador', async () => {
		await makeDeleteUser({ users: repo.asUsers() })({
			username: 'soporte',
			currentUsername: 'felipe',
		})

		expect(repo.users.map((user) => user.username)).toEqual(['felipe'])
		expect(repo.tokens[0]?.createdBy).toEqual({ kind: 'deleted' })
	})

	it('a sí mismo no se borra, y ni lo pide', async () => {
		await expect(
			makeDeleteUser({ users: repo.asUsers() })({
				username: 'felipe',
				currentUsername: 'felipe',
			}),
		).rejects.toBeInstanceOf(CannotDeleteSelf)
		expect(repo.calls.delete).toBe(0)
	})

	it('sin saber quién es la sesión, el backend igual lo rechaza', async () => {
		await expect(
			makeDeleteUser({ users: repo.asUsers() })({
				username: 'felipe',
				currentUsername: null,
			}),
		).rejects.toBeInstanceOf(CannotDeleteSelf)
		expect(repo.calls.delete).toBe(1)
	})

	it('propaga que el usuario ya no existe', async () => {
		await expect(
			makeDeleteUser({ users: repo.asUsers() })({
				username: 'nadie',
				currentUsername: 'felipe',
			}),
		).rejects.toBeInstanceOf(UserNotFound)
	})
})
