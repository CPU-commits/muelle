import { afterAll, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { authDb, MOCK_PASSWORD, MOCK_USERNAME } from '@/mocks/db/auth'
import { catalogDb } from '@/mocks/db/catalog'
import { deployDb } from '@/mocks/db/deploy'
import { secretsDb } from '@/mocks/db/secrets'
import { server } from '@/mocks/server'
import { authUseCases } from '@/modules/auth'
import { appRefFrom, catalogUseCases } from '@/modules/catalog'
import { appOwner, secretsUseCases, stageOwner } from '@/modules/secrets'
import { NotFoundError } from '@/shared/domain'

// El mock de secrets con el de catalog, como el backend: cambiar variables de una app (o de
// su stage) actualiza su `config_changed_at`, y la app queda con cambios pendientes (§5.2
// regla 4 del diseño). Un dueño que no está en el catálogo es 404.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(async () => {
	authDb.reset()
	catalogDb.reset()
	deployDb.reset()
	secretsDb.reset()
	await authUseCases.login({ username: MOCK_USERNAME, password: MOCK_PASSWORD })
})
afterAll(() => server.close())

const pending = async (stage: string, app: string) =>
	(await catalogUseCases.getApp(appRefFrom('auditfy', stage, app))).pendingChanges

describe('variables y cambios pendientes en el mock', () => {
	it('cambiar una variable de la app la deja con cambios pendientes, y a las otras no', async () => {
		expect(await pending('staging', 'nats')).toBe(false)

		await secretsUseCases.patchEnv({
			owner: appOwner('auditfy', 'staging', 'nats'),
			set: [['NATS_DEBUG', 'true']],
			unset: [],
			secret: false,
		})

		expect(await pending('staging', 'nats')).toBe(true)
		expect(await pending('staging', 'redis')).toBe(false)
	})

	it('cambiar una del stage deja con cambios pendientes a todas sus apps', async () => {
		await secretsUseCases.patchEnv({
			owner: stageOwner('auditfy', 'production'),
			set: [['FEATURE_X', 'on']],
			unset: [],
			secret: false,
		})

		for (const app of ['api', 'nats', 'redis'])
			expect(await pending('production', app)).toBe(true)
		expect(await pending('staging', 'nats')).toBe(false)
	})

	it('un PATCH que no cambia nada no marca nada', async () => {
		await secretsUseCases.patchEnv({
			owner: stageOwner('auditfy', 'production'),
			set: [['NODE_ENV', 'production']],
			unset: [],
			secret: false,
		})

		expect(await pending('production', 'nats')).toBe(false)
	})

	it('las variables de una app o un stage que no existen son 404', async () => {
		await expect(
			secretsUseCases.getEnv(appOwner('auditfy', 'staging', 'nada')),
		).rejects.toBeInstanceOf(NotFoundError)
		await expect(secretsUseCases.getEnv(stageOwner('nada', 'staging'))).rejects.toBeInstanceOf(
			NotFoundError,
		)
	})
})
