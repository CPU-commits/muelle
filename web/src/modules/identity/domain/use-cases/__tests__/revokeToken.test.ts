import { describe, expect, it } from 'vitest'
import { makeListTokens, makeRevokeToken, type TokenId, TokenNotFound } from '../..'
import {
	fakeToken,
	InMemoryIdentityRepository,
} from '../../repositories/__fakes__/inMemoryIdentityRepository'

describe('revokeToken', () => {
	it('lo saca de la lista, y revocarlo otra vez es TokenNotFound', async () => {
		const repo = new InMemoryIdentityRepository({
			tokens: [
				fakeToken({ id: 1 as TokenId }),
				fakeToken({ id: 2 as TokenId, name: 'otro' }),
			],
		})
		const revoke = makeRevokeToken({ tokens: repo.asTokens() })

		await revoke(1 as TokenId)

		const left = await makeListTokens({ tokens: repo.asTokens() })()
		expect(left.map((token) => token.name)).toEqual(['otro'])
		await expect(revoke(1 as TokenId)).rejects.toBeInstanceOf(TokenNotFound)
	})
})
