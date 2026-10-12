import { describe, expect, it } from 'vitest'
import {
	appRefFrom,
	type CatalogTree,
	countApps,
	filterCatalog,
	type ProjectSlug,
	type StageSlug,
	switchProjectTarget,
	switchStageTarget,
} from '../..'
import {
	fakeComposeApp,
	fakeProject,
	fakeServiceApp,
	fakeStage,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const tree: CatalogTree = [
	{
		project: fakeProject('auditfy', 'Auditfy'),
		stages: [
			{
				stage: fakeStage('auditfy', 'staging'),
				apps: [
					fakeServiceApp('auditfy', 'staging', 'api', {
						spec: {
							...fakeServiceApp('a', 'b', 'c').spec,
							image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
						},
					}),
					fakeServiceApp('auditfy', 'staging', 'redis'),
				],
			},
			{
				stage: fakeStage('auditfy', 'production'),
				apps: [fakeServiceApp('auditfy', 'production', 'redis')],
			},
		],
	},
	{
		project: fakeProject('aumi-agents', 'Aumi Agents'),
		stages: [
			{
				stage: fakeStage('aumi-agents', 'staging'),
				apps: [
					fakeComposeApp(
						'aumi-agents',
						'staging',
						'agents',
						'services:\n  gateway:\n    image: g\n  scheduler:\n    image: s\n',
					),
				],
			},
			{ stage: fakeStage('aumi-agents', 'production'), apps: [] },
		],
	},
	{ project: fakeProject('vacio'), stages: [] },
]

const slugsOf = (result: CatalogTree) =>
	result.map((node) => [
		node.project.slug,
		node.stages.map((stage) => [stage.stage.slug, stage.apps.map((app) => app.slug)]),
	])

describe('catalogTree', () => {
	it('cuenta las apps de todo el árbol o de un proyecto', () => {
		expect(countApps(tree)).toBe(4)
		expect(countApps(tree[0]!)).toBe(3)
		expect(countApps([])).toBe(0)
	})

	it('sin búsqueda devuelve el árbol tal cual', () => {
		expect(filterCatalog(tree, '  ')).toBe(tree)
	})

	it('busca por app, imagen y servicio, y saca lo que queda vacío', () => {
		expect(slugsOf(filterCatalog(tree, 'REDIS'))).toEqual([
			[
				'auditfy',
				[
					['staging', ['redis']],
					['production', ['redis']],
				],
			],
		])
		expect(slugsOf(filterCatalog(tree, 'backend-app'))).toEqual([
			['auditfy', [['staging', ['api']]]],
		])
		expect(slugsOf(filterCatalog(tree, 'scheduler'))).toEqual([
			['aumi-agents', [['staging', ['agents']]]],
		])
		expect(filterCatalog(tree, 'nada-que-ver')).toEqual([])
	})

	it('un proyecto o un stage que coincide se muestra entero', () => {
		expect(slugsOf(filterCatalog(tree, 'Aumi Agents'))).toEqual([
			[
				'aumi-agents',
				[
					['staging', ['agents']],
					['production', []],
				],
			],
		])
		expect(slugsOf(filterCatalog(tree, 'producti'))).toEqual([
			['auditfy', [['production', ['redis']]]],
			['aumi-agents', [['production', []]]],
		])
	})

	it('el selector de proyecto busca el mismo stage y la misma app', () => {
		const current = appRefFrom('aumi-agents', 'staging', 'agents')
		expect(switchProjectTarget(tree, current, 'auditfy' as ProjectSlug)).toEqual(
			appRefFrom('auditfy', 'staging', 'api'),
		)
		const redis = appRefFrom('x', 'production', 'redis')
		expect(switchProjectTarget(tree, redis, 'auditfy' as ProjectSlug)).toEqual(
			appRefFrom('auditfy', 'production', 'redis'),
		)
		expect(switchProjectTarget(tree, current, 'vacio' as ProjectSlug)).toBeNull()
		expect(switchProjectTarget(tree, current, 'no-existe' as ProjectSlug)).toBeNull()
	})

	it('el selector de stage lleva a la misma app o a la primera', () => {
		const redis = appRefFrom('auditfy', 'staging', 'redis')
		expect(switchStageTarget(tree, redis, 'production' as StageSlug)).toEqual(
			appRefFrom('auditfy', 'production', 'redis'),
		)
		const api = appRefFrom('auditfy', 'staging', 'api')
		expect(switchStageTarget(tree, api, 'production' as StageSlug)).toEqual(
			appRefFrom('auditfy', 'production', 'redis'),
		)
		const agents = appRefFrom('aumi-agents', 'staging', 'agents')
		expect(switchStageTarget(tree, agents, 'production' as StageSlug)).toBeNull()
	})
})
