import { describe, expect, it } from 'vitest'
import { isCurrentUser } from '../..'
import { fakeUser } from '../../repositories/__fakes__/inMemoryIdentityRepository'

describe('user', () => {
	it('reconoce la fila del usuario de la sesión', () => {
		expect(isCurrentUser(fakeUser({ username: 'felipe' }), 'felipe')).toBe(true)
		expect(isCurrentUser(fakeUser({ username: 'soporte' }), 'felipe')).toBe(false)
		expect(isCurrentUser(fakeUser(), null)).toBe(false)
	})
})
