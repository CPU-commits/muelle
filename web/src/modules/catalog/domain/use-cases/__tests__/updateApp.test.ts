import { describe, expect, it } from 'vitest'
import {
	appRefFrom,
	InvalidAppSpec,
	InvalidCompose,
	InvalidDeployTimeout,
	KindMismatch,
	makeUpdateApp,
	newServiceSpec,
} from '../..'
import {
	fakeComposeApp,
	fakeServiceApp,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const api = appRefFrom('auditfy', 'staging', 'api')
const agents = appRefFrom('auditfy', 'staging', 'agents')
const deployed = new Date('2026-10-02T00:00:00.000Z')

function repo() {
	return new InMemoryCatalogRepository({
		apps: [
			fakeServiceApp('auditfy', 'staging', 'api', { lastDeployedAt: deployed }),
			fakeComposeApp('auditfy', 'staging', 'agents'),
		],
	})
}

describe('updateApp', () => {
	it('guarda el spec y deja la app con cambios pendientes', async () => {
		const catalog = repo()
		const spec = { ...newServiceSpec('nginx:1.28', 80), replicas: 2 }

		const saved = await makeUpdateApp({ catalog })({ ref: api, changes: { spec } })

		expect(saved).toMatchObject({ kind: 'service', spec, pendingChanges: true })
		expect(catalog.lastChanges).toEqual({ spec })
	})

	it('el timeout solo no es un cambio de configuración', async () => {
		const catalog = repo()

		const saved = await makeUpdateApp({ catalog })({
			ref: api,
			changes: { deployTimeoutSeconds: 120 },
		})

		expect(saved).toMatchObject({ deployTimeoutSeconds: 120, pendingChanges: false })
	})

	it('valida lo que se manda y nada más', async () => {
		const catalog = repo()
		const update = makeUpdateApp({ catalog })

		await expect(
			update({ ref: api, changes: { spec: newServiceSpec('Mala Imagen', 80) } }),
		).rejects.toBeInstanceOf(InvalidAppSpec)
		await expect(update({ ref: agents, changes: { composeYaml: '' } })).rejects.toBeInstanceOf(
			InvalidCompose,
		)
		await expect(
			update({ ref: api, changes: { deployTimeoutSeconds: 4000 } }),
		).rejects.toBeInstanceOf(InvalidDeployTimeout)
		expect(catalog.calls.updateApp).toBe(0)
	})

	it('el spec de una compose o el compose de una de formulario es KindMismatch', async () => {
		const update = makeUpdateApp({ catalog: repo() })

		await expect(
			update({ ref: agents, changes: { spec: newServiceSpec('nginx', 80) } }),
		).rejects.toBeInstanceOf(KindMismatch)
		await expect(
			update({ ref: api, changes: { composeYaml: 'services: {}' } }),
		).rejects.toBeInstanceOf(KindMismatch)
	})
})
