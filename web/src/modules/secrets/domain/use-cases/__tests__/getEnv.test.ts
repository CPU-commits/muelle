import { describe, expect, it } from 'vitest'
import { appOwner, EnvOwnerNotFound, stageOwner } from '../..'
import {
	FAKE_EPOCH,
	InMemoryEnvRepository,
} from '../../repositories/__fakes__/inMemoryEnvRepository'
import { makeGetEnv } from '../getEnv'

describe('getEnv', () => {
	const env = new InMemoryEnvRepository({
		owners: {
			'stage:auditfy/staging': [{ key: 'NODE_ENV', value: 'staging', secret: false }],
			'app:auditfy/staging/api': [
				{ key: 'PORT', value: '3000', secret: false },
				{ key: 'JWT_SECRET', value: 'no-se-ve', secret: true },
			],
		},
	})
	const getEnv = makeGetEnv({ env })

	it('devuelve las variables ordenadas, con las secretas enmascaradas', async () => {
		await expect(getEnv(appOwner('auditfy', 'staging', 'api'))).resolves.toEqual([
			{ key: 'JWT_SECRET', secret: true, value: null, updatedAt: FAKE_EPOCH },
			{ key: 'PORT', secret: false, value: '3000', updatedAt: FAKE_EPOCH },
		])
		await expect(getEnv(stageOwner('auditfy', 'staging'))).resolves.toHaveLength(1)
	})

	it('un dueño que no existe es EnvOwnerNotFound', async () => {
		await expect(getEnv(appOwner('auditfy', 'staging', 'nada'))).rejects.toBeInstanceOf(
			EnvOwnerNotFound,
		)
	})
})
