import { beforeEach, describe, expect, it } from 'vitest'
import {
	appOwner,
	EmptyEnvPatch,
	EnvLimitExceeded,
	EnvOwnerNotFound,
	InvalidEnvKey,
	MAX_ENV_VARS,
} from '../..'
import { InMemoryEnvRepository } from '../../repositories/__fakes__/inMemoryEnvRepository'
import { makeGetEnv } from '../getEnv'
import { makePatchEnv } from '../patchEnv'

const api = appOwner('auditfy', 'staging', 'api')
const later = new Date('2026-10-05T12:00:00.000Z')

describe('patchEnv', () => {
	let env: InMemoryEnvRepository
	let patchEnv: ReturnType<typeof makePatchEnv>
	let getEnv: ReturnType<typeof makeGetEnv>

	beforeEach(() => {
		env = new InMemoryEnvRepository({
			owners: {
				'app:auditfy/staging/api': [
					{ key: 'PORT', value: '3000', secret: false },
					{ key: 'JWT_SECRET', value: 'viejo', secret: true },
				],
			},
			now: () => later,
		})
		patchEnv = makePatchEnv({ env })
		getEnv = makeGetEnv({ env })
	})

	it('agrega como secreta y borra en el mismo PATCH', async () => {
		await patchEnv({ owner: api, set: [['API_KEY', 'k']], unset: ['PORT'], secret: true })

		await expect(getEnv(api)).resolves.toEqual([
			{ key: 'API_KEY', secret: true, value: null, updatedAt: later },
			{ key: 'JWT_SECRET', secret: true, value: null, updatedAt: expect.any(Date) },
		])
	})

	it('pasar una variable a secreta reescribe su valor con la marca', async () => {
		await patchEnv({ owner: api, set: [['PORT', '3000']], secret: true })

		const [, port] = await getEnv(api)
		expect(port).toEqual({ key: 'PORT', secret: true, value: null, updatedAt: later })
	})

	it('valida antes de llamar al repositorio', async () => {
		await expect(patchEnv({ owner: api, secret: true })).rejects.toBeInstanceOf(EmptyEnvPatch)
		await expect(
			patchEnv({ owner: api, set: [['COMPOSE_FILE', 'x']], secret: true }),
		).rejects.toBeInstanceOf(InvalidEnvKey)
		expect(env.calls.patch).toHaveLength(0)
	})

	it('con current revisa los límites del dueño antes de mandar', async () => {
		const current = await getEnv(api)
		const full = Array.from({ length: MAX_ENV_VARS - 1 }, (_, index): [string, string] => [
			`K${index}`,
			'v',
		])

		await expect(
			patchEnv({ owner: api, set: full, secret: false, current }),
		).rejects.toBeInstanceOf(EnvLimitExceeded)
		expect(env.calls.patch).toHaveLength(0)
	})

	it('sin current, los límites los revisa el repositorio (el backend)', async () => {
		const full = Array.from({ length: MAX_ENV_VARS - 1 }, (_, index): [string, string] => [
			`K${index}`,
			'v',
		])

		await expect(patchEnv({ owner: api, set: full, secret: false })).rejects.toMatchObject({
			limit: 'count',
		})
		expect(env.calls.patch).toHaveLength(1)
		await expect(getEnv(api)).resolves.toHaveLength(2)
	})

	it('un dueño que no existe es EnvOwnerNotFound', async () => {
		await expect(
			patchEnv({ owner: appOwner('auditfy', 'staging', 'nada'), unset: ['X'], secret: true }),
		).rejects.toBeInstanceOf(EnvOwnerNotFound)
	})
})
