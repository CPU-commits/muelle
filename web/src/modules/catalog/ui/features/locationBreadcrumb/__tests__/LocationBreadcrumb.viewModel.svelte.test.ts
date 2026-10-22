import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { BreadcrumbItem, BreadcrumbMenu, MenuEntry } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { CatalogTree } from '../../../../domain'
import {
	fakeComposeApp,
	fakeProject,
	fakeServiceApp,
	fakeStage,
} from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel, until } from '../../../__tests__/mountViewModel'
import {
	type Location,
	LocationBreadcrumbViewModel,
} from '../LocationBreadcrumb.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({
	listCatalog: vi.fn(),
	page: { url: new URL('http://localhost/') },
}))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: mocks.listCatalog,
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))
vi.mock('$app/state', () => ({ page: mocks.page }))

const tree: CatalogTree = [
	{
		project: fakeProject('auditfy'),
		stages: [
			{
				stage: fakeStage('auditfy', 'staging'),
				apps: [
					fakeServiceApp('auditfy', 'staging', 'api'),
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
		project: fakeProject('aumi-agents'),
		stages: [
			{
				stage: fakeStage('aumi-agents', 'staging'),
				apps: [fakeComposeApp('aumi-agents', 'staging', 'agents')],
			},
		],
	},
	{ project: fakeProject('vacio'), stages: [] },
]

async function loaded(location: Location, path: string): Promise<LocationBreadcrumbViewModel> {
	mocks.page.url = new URL(`http://localhost${path}`)
	const vm = await mountViewModel(() => new LocationBreadcrumbViewModel(() => location))
	await until(() => vm.items.some((item) => item.kind === 'menu'))
	return vm
}

function menu(items: BreadcrumbItem[], index: number): BreadcrumbMenu {
	const item = items[index]
	if (item?.kind !== 'menu') throw new Error(`el segmento ${index} no es un selector`)
	return item
}

const links = (entries: readonly MenuEntry[]) =>
	entries.flatMap((entry) =>
		entry.kind === 'link'
			? [{ label: entry.label, href: entry.href, current: entry.current === true }]
			: [],
	)

describe('LocationBreadcrumbViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listCatalog.mockResolvedValue(tree)
	})

	it('los selectores de una app llevan al mismo tab en el otro lugar', async () => {
		const vm = await loaded(
			{ project: 'auditfy', stage: 'staging', app: 'redis' },
			'/projects/auditfy/stages/staging/apps/redis/configuration',
		)
		const [project, stage, app] = [menu(vm.items, 0), menu(vm.items, 1), menu(vm.items, 2)]

		expect(project.ariaLabel).toBe(m.catalog_crumb_project({ project: 'auditfy' }))
		expect(project.initial).toBe('a')
		expect(links(project.items)).toEqual([
			{ label: m.catalog_crumb_projects(), href: '/', current: false },
			{
				label: 'auditfy',
				href: '/projects/auditfy/stages/staging/apps/redis/configuration',
				current: true,
			},
			// Sin redis allá: la primera app del mismo stage, en el mismo tab.
			{
				label: 'aumi-agents',
				href: '/projects/aumi-agents/stages/staging/apps/agents/configuration',
				current: false,
			},
			// Sin apps: su sección en Proyectos.
			{ label: 'vacio', href: '/#p-vacio', current: false },
		])
		expect(links(stage.items)).toEqual([
			{
				label: m.catalog_crumb_stage_variables({ stage: 'staging' }),
				href: '/projects/auditfy/stages/staging/variables',
				current: false,
			},
			{
				label: 'staging',
				href: '/projects/auditfy/stages/staging/apps/redis/configuration',
				current: true,
			},
			{
				label: 'production',
				href: '/projects/auditfy/stages/production/apps/redis/configuration',
				current: false,
			},
		])
		expect(links(app.items)).toEqual([
			{
				label: 'api',
				href: '/projects/auditfy/stages/staging/apps/api/configuration',
				current: false,
			},
			{
				label: 'redis',
				href: '/projects/auditfy/stages/staging/apps/redis/configuration',
				current: true,
			},
		])
	})

	it('en Resumen el destino es el Resumen de la otra app', async () => {
		const vm = await loaded(
			{ project: 'auditfy', stage: 'staging', app: 'api' },
			'/projects/auditfy/stages/staging/apps/api',
		)

		expect(links(menu(vm.items, 1).items)[2]).toEqual({
			label: 'production',
			href: '/projects/auditfy/stages/production/apps/redis',
			current: false,
		})
	})

	it('el de las variables de un stage cambia entre variables de stages', async () => {
		const vm = await loaded(
			{ project: 'auditfy', stage: 'production', app: null },
			'/projects/auditfy/stages/production/variables',
		)

		expect(vm.items).toHaveLength(2)
		expect(links(menu(vm.items, 0).items).slice(1)).toEqual([
			{
				label: 'auditfy',
				href: '/projects/auditfy/stages/production/variables',
				current: true,
			},
			// Sin production: el primer stage.
			{
				label: 'aumi-agents',
				href: '/projects/aumi-agents/stages/staging/variables',
				current: false,
			},
			{ label: 'vacio', href: '/#p-vacio', current: false },
		])
		expect(links(menu(vm.items, 1).items)).toEqual([
			{
				label: 'staging',
				href: '/projects/auditfy/stages/staging/variables',
				current: false,
			},
			{
				label: 'production',
				href: '/projects/auditfy/stages/production/variables',
				current: true,
			},
		])
	})

	it('mientras no hay árbol (o el proyecto no está), los segmentos son links', async () => {
		mocks.listCatalog.mockReturnValue(new Promise(() => {}))
		mocks.page.url = new URL('http://localhost/projects/auditfy/stages/staging/apps/api')
		const vm = await mountViewModel(
			() =>
				new LocationBreadcrumbViewModel(() => ({
					project: 'auditfy',
					stage: 'staging',
					app: 'api',
				})),
		)

		expect(vm.items).toEqual([
			{ label: 'auditfy', initial: 'a', href: '/#p-auditfy' },
			{ label: 'staging', href: '/projects/auditfy/stages/staging/variables' },
			{ label: 'api', href: '/projects/auditfy/stages/staging/apps/api' },
		])
	})

	it('con los params vacíos (al salir de la ruta) no pide el árbol', async () => {
		const vm = await mountViewModel(
			() => new LocationBreadcrumbViewModel(() => ({ project: '', stage: '', app: '' })),
		)

		expect(vm.ready).toBe(false)
		expect(mocks.listCatalog).not.toHaveBeenCalled()
	})
})
