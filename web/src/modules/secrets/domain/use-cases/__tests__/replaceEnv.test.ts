import { describe, expect, it } from 'vitest'
import { EnvLimitExceeded, InvalidEnvValue, stageOwner } from '../..'
import { InMemoryEnvRepository } from '../../repositories/__fakes__/inMemoryEnvRepository'
import { makeGetEnv } from '../getEnv'
import { makeReplaceEnv } from '../replaceEnv'

const staging = stageOwner('auditfy', 'staging')

function setup() {
	const env = new InMemoryEnvRepository({
		owners: { 'stage:auditfy/staging': [{ key: 'OLD', value: 'x', secret: false }] },
	})
	return { env, replaceEnv: makeReplaceEnv({ env }), getEnv: makeGetEnv({ env }) }
}

describe('replaceEnv', () => {
	it('reemplaza todo el set con un solo secret', async () => {
		const { replaceEnv, getEnv } = setup()

		await replaceEnv({
			owner: staging,
			vars: [
				['B', '2'],
				['A', '1'],
			],
			secret: false,
		})

		await expect(getEnv(staging)).resolves.toMatchObject([
			{ key: 'A', value: '1', secret: false },
			{ key: 'B', value: '2', secret: false },
		])
	})

	it('un set vacío deja al dueño sin variables', async () => {
		const { replaceEnv, getEnv } = setup()

		await replaceEnv({ owner: staging, vars: [], secret: true })

		await expect(getEnv(staging)).resolves.toEqual([])
	})

	it('valida antes de llamar al repositorio', async () => {
		const { env, replaceEnv } = setup()
		const many = Array.from({ length: 501 }, (_, index): [string, string] => [`K${index}`, ''])

		await expect(
			replaceEnv({ owner: staging, vars: many, secret: true }),
		).rejects.toBeInstanceOf(EnvLimitExceeded)
		await expect(
			replaceEnv({ owner: staging, vars: [['A', '\0']], secret: true }),
		).rejects.toBeInstanceOf(InvalidEnvValue)
		expect(env.calls.replace).toHaveLength(0)
	})
})
