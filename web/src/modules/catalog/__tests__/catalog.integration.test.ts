import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, invalid } from '@/mocks/api'
import { authDb, MOCK_PASSWORD, MOCK_USERNAME } from '@/mocks/db/auth'
import { catalogDb } from '@/mocks/db/catalog'
import { server } from '@/mocks/server'
import { ValidationError } from '@/shared/domain'
import { authUseCases } from '@/modules/auth'
import { catalogUseCases } from '../container'
import {
	AppNotFound,
	appRefFrom,
	countApps,
	KindMismatch,
	newServiceSpec,
	ProjectNotEmpty,
	type ProjectSlug,
	type ServiceApp,
	SlugTaken,
	StageNotEmpty,
	type StageSlug,
} from '../domain'

// Recorre el módulo entero —caso de uso, repositorio, datasource, cliente HTTP, schema
// Zod y mapper— contra el backend simulado por MSW (src/mocks/handlers/catalog.ts).
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(async () => {
	authDb.reset()
	catalogDb.reset()
	await authUseCases.login({ username: MOCK_USERNAME, password: MOCK_PASSWORD })
})
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const auditfy = 'auditfy' as ProjectSlug
const staging = 'staging' as StageSlug
const stagingApi = appRefFrom('auditfy', 'staging', 'api')

async function serviceApp(): Promise<ServiceApp> {
	const app = await catalogUseCases.getApp(stagingApi)
	if (app.kind !== 'service') throw new Error('la api de auditfy es de formulario')
	return app
}

describe('catálogo contra el backend simulado', () => {
	it('arma el árbol del diseño: 3 proyectos, 6 stages y 10 apps', async () => {
		const tree = await catalogUseCases.listCatalog()

		expect(tree.map((node) => node.project.slug)).toEqual([
			'auditfy',
			'extractor',
			'aumi-agents',
		])
		expect(tree.flatMap((node) => node.stages)).toHaveLength(6)
		expect(countApps(tree)).toBe(10)
		const api = tree[0]?.stages[0]?.apps[0]
		expect(api).toMatchObject({ slug: 'api', kind: 'service', pendingChanges: true })
		const agents = tree[2]?.stages[0]?.apps[0]
		expect(agents).toMatchObject({
			kind: 'compose',
			lastDeployedAt: null,
			pendingChanges: false,
		})
	})

	it('crea, renombra y borra un proyecto; el slug repetido es SlugTaken', async () => {
		await catalogUseCases.createProject({ slug: 'nuevo', name: 'Nuevo' })
		await expect(
			catalogUseCases.createProject({ slug: 'nuevo', name: 'Otro' }),
		).rejects.toBeInstanceOf(SlugTaken)

		await expect(
			catalogUseCases.renameProject({ project: 'nuevo' as ProjectSlug, name: 'Renombrado' }),
		).resolves.toMatchObject({ slug: 'nuevo', name: 'Renombrado' })

		await catalogUseCases.deleteProject('nuevo' as ProjectSlug)
		const tree = await catalogUseCases.listCatalog()
		expect(tree.map((node) => node.project.slug)).not.toContain('nuevo')
	})

	it('no borra un proyecto con stages ni un stage con apps', async () => {
		await expect(catalogUseCases.deleteProject(auditfy)).rejects.toBeInstanceOf(ProjectNotEmpty)
		await expect(
			catalogUseCases.deleteStage({ project: auditfy, stage: staging }),
		).rejects.toBeInstanceOf(StageNotEmpty)

		await catalogUseCases.createStage({ project: auditfy, slug: 'qa' })
		await catalogUseCases.deleteStage({ project: auditfy, stage: 'qa' as StageSlug })
	})

	it('crea apps de los dos tipos y la repetida es SlugTaken', async () => {
		await expect(
			catalogUseCases.createApp({
				project: auditfy,
				stage: staging,
				slug: 'web',
				kind: 'service',
				spec: newServiceSpec('nginx:1.27-alpine', 80),
			}),
		).resolves.toMatchObject({ slug: 'web', kind: 'service', deployTimeoutSeconds: 300 })
		await expect(
			catalogUseCases.createApp({
				project: auditfy,
				stage: staging,
				slug: 'jobs',
				kind: 'compose',
				composeYaml: 'services:\n  worker:\n    image: busybox\n',
				deployTimeoutSeconds: 120,
			}),
		).resolves.toMatchObject({ kind: 'compose', deployTimeoutSeconds: 120 })
		await expect(
			catalogUseCases.createApp({
				project: auditfy,
				stage: staging,
				slug: 'api',
				kind: 'service',
				spec: newServiceSpec('nginx', 80),
			}),
		).rejects.toMatchObject({ name: 'SlugTaken', entity: 'app' })
	})

	it('guarda el spec y el backend lo devuelve normalizado', async () => {
		const app = await serviceApp()
		const spec = { ...app.spec, replicas: 3, command: ['node', 'dist/main.js'] }

		const saved = await catalogUseCases.updateApp({ ref: stagingApi, changes: { spec } })

		expect(saved).toMatchObject({ kind: 'service', spec, pendingChanges: true })
		expect(await catalogUseCases.getApp(stagingApi)).toEqual(saved)
	})

	it('un 422 llega como ValidationError con el campo en nombres del dominio', async () => {
		server.use(
			http.patch(api('/projects/:p/stages/:s/apps/:a'), () =>
				invalid('healthcheck.interval_s', 'debe estar entre 1 y 3600'),
			),
		)
		const app = await serviceApp()

		const error = await catalogUseCases
			.updateApp({ ref: stagingApi, changes: { spec: app.spec } })
			.catch((reason: unknown) => reason)

		expect(error).toBeInstanceOf(ValidationError)
		expect(error).toMatchObject({
			field: 'healthcheck.intervalSeconds',
			message: 'debe estar entre 1 y 3600',
		})
	})

	it('los volúmenes y el timeout también llegan a su campo', async () => {
		const app = await serviceApp()
		// El dominio dejaría pasar este mount: lo rechaza el mock, como el backend real.
		server.use(
			http.patch(api('/projects/:p/stages/:s/apps/:a'), () =>
				invalid('volumes[0].mount', 'debe ser una ruta absoluta'),
			),
		)
		await expect(
			catalogUseCases.updateApp({ ref: stagingApi, changes: { spec: app.spec } }),
		).rejects.toMatchObject({ field: 'volumes[0].mount' })

		server.resetHandlers()
		server.use(
			http.patch(api('/projects/:p/stages/:s/apps/:a'), () =>
				invalid('deploy_timeout_s', 'debe estar entre 30 y 3600'),
			),
		)
		await expect(
			catalogUseCases.updateApp({ ref: stagingApi, changes: { deployTimeoutSeconds: 60 } }),
		).rejects.toMatchObject({ field: 'deployTimeoutSeconds' })
	})

	it('el compose se guarda con PATCH compose_yaml; en una de formulario es KindMismatch', async () => {
		const agents = appRefFrom('aumi-agents', 'staging', 'agents')
		const yaml = 'services:\n  gateway:\n    image: ghcr.io/aumi-app/agents-gateway:v2\n'

		await expect(
			catalogUseCases.updateApp({ ref: agents, changes: { composeYaml: yaml } }),
		).resolves.toMatchObject({ kind: 'compose', composeYaml: yaml })
		await expect(
			catalogUseCases.updateApp({ ref: stagingApi, changes: { composeYaml: yaml } }),
		).rejects.toBeInstanceOf(KindMismatch)
	})

	it('borra una app y después es AppNotFound', async () => {
		await catalogUseCases.deleteApp(stagingApi)

		await expect(catalogUseCases.getApp(stagingApi)).rejects.toBeInstanceOf(AppNotFound)
	})

	it('una respuesta con otra forma es un error de contrato, no un undefined en la UI', async () => {
		server.use(http.get(api('/projects'), () => HttpResponse.json([{ slug: 1 }])))

		await expect(catalogUseCases.listCatalog()).rejects.toMatchObject({ name: 'ContractError' })
	})
})
