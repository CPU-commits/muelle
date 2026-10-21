import { describe, expect, it } from 'vitest'
import { makeListCatalog } from '../..'
import {
	fakeComposeApp,
	fakeProject,
	fakeServiceApp,
	fakeStage,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

describe('listCatalog', () => {
	it('arma proyectos → stages → apps con un pedido por proyecto y por stage', async () => {
		const catalog = new InMemoryCatalogRepository({
			projects: [fakeProject('auditfy'), fakeProject('vacio')],
			stages: [fakeStage('auditfy', 'staging'), fakeStage('auditfy', 'production')],
			apps: [
				fakeServiceApp('auditfy', 'staging', 'api'),
				fakeComposeApp('auditfy', 'production', 'agents'),
			],
		})

		const tree = await makeListCatalog({ catalog })()

		expect(
			tree.map((node) => ({
				project: node.project.slug,
				stages: node.stages.map((stage) => ({
					stage: stage.stage.slug,
					apps: stage.apps.map((app) => app.slug),
				})),
			})),
		).toEqual([
			{
				project: 'auditfy',
				stages: [
					{ stage: 'staging', apps: ['api'] },
					{ stage: 'production', apps: ['agents'] },
				],
			},
			{ project: 'vacio', stages: [] },
		])
		expect(catalog.calls).toMatchObject({ listProjects: 1, listStages: 2, listApps: 2 })
	})
})
