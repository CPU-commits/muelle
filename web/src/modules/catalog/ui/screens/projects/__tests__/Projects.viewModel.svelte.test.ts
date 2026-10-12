import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { type CatalogTree, ProjectNotEmpty, StageNotEmpty } from '../../../../domain'
import {
	fakeComposeApp,
	fakeProject,
	fakeServiceApp,
	fakeStage,
} from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel, until } from '../../../__tests__/mountViewModel'
import { ProjectsViewModel } from '..'

const mocks = vi.hoisted(() => ({
	listCatalog: vi.fn(),
	deleteProject: vi.fn(),
	deleteStage: vi.fn(),
}))

// Se mockea el CONTAINER, no la red: el test verifica lo que hace el viewModel con lo
// que le devuelven los casos de uso.
vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: mocks.listCatalog,
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: mocks.deleteProject,
		createStage: vi.fn(),
		deleteStage: mocks.deleteStage,
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))

const tree: CatalogTree = [
	{
		project: fakeProject('auditfy', 'Auditfy'),
		stages: [
			{
				stage: fakeStage('auditfy', 'staging'),
				apps: [
					fakeServiceApp('auditfy', 'staging', 'api'),
					fakeServiceApp('auditfy', 'staging', 'redis'),
				],
			},
		],
	},
	{
		project: fakeProject('aumi-agents', 'Aumi Agents'),
		stages: [
			{
				stage: fakeStage('aumi-agents', 'staging'),
				apps: [fakeComposeApp('aumi-agents', 'staging', 'agents')],
			},
		],
	},
	{ project: fakeProject('vacio'), stages: [] },
]

async function loaded(): Promise<ProjectsViewModel> {
	const vm = await mountViewModel(() => new ProjectsViewModel())
	await until(() => !vm.isLoading)
	return vm
}

describe('ProjectsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listCatalog.mockResolvedValue(tree)
		mocks.deleteProject.mockResolvedValue(undefined)
		mocks.deleteStage.mockResolvedValue(undefined)
	})

	it('carga el árbol y cuenta las apps', async () => {
		const vm = await loaded()

		expect(vm.projects).toHaveLength(3)
		expect(vm.appsMeta).toBe(m.catalog_apps_other({ count: 3 }))
		expect(vm.projectMeta(tree[0]!)).toBe(
			m.catalog_meta_join({
				first: m.catalog_stages_one(),
				second: m.catalog_apps_other({ count: 2 }),
			}),
		)
		expect(vm.appHref(fakeServiceApp('auditfy', 'staging', 'api'))).toBe(
			'/projects/auditfy/stages/staging/apps/api',
		)
	})

	it('el buscador filtra y avisa cuando no queda nada', async () => {
		const vm = await loaded()

		vm.setSearch('redis')
		expect(vm.projects.map((node) => node.project.slug)).toEqual(['auditfy'])
		expect(vm.projects[0]?.stages[0]?.apps.map((app) => app.slug)).toEqual(['redis'])

		vm.setSearch('nada')
		expect(vm.noMatches).toBe(true)

		vm.clearSearch()
		expect(vm.projects).toHaveLength(3)
	})

	it('un error de carga se muestra con su mensaje', async () => {
		mocks.listCatalog.mockRejectedValue(new Error('boom'))
		const vm = await mountViewModel(() => new ProjectsViewModel())
		await until(() => !vm.isLoading)

		expect(vm.loadError).toBe(m.common_error_generic())
	})

	it('el menú del proyecto abre los diálogos', async () => {
		const vm = await loaded()
		const menu = vm.projectMenu(tree[0]!)

		const select = (id: string) => {
			const entry = menu.find((item) => item.id === id)
			if (entry?.kind === 'action') entry.onSelect()
		}
		select('new-stage')
		expect(vm.dialog).toEqual({ kind: 'newStage', project: 'auditfy' })
		select('rename')
		expect(vm.dialog).toMatchObject({ kind: 'renameProject' })
		select('delete')
		expect(vm.dialog).toMatchObject({ kind: 'deleteProject' })
		expect(vm.confirmation?.body).toBe(m.catalog_delete_project_body_stages())
	})

	it('borrar un proyecto vacío avisa y cierra', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await loaded()

		vm.open({ kind: 'deleteProject', node: tree[2]! })
		expect(vm.confirmation?.body).toBe(m.catalog_delete_project_body())
		await vm.confirmDelete()

		expect(mocks.deleteProject).toHaveBeenCalledWith('vacio')
		expect(success).toHaveBeenCalledWith(m.catalog_project_deleted({ project: 'vacio' }))
		expect(vm.dialog).toBeNull()
	})

	it('not_empty dice claramente qué falta', async () => {
		const error = vi.spyOn(toast, 'error')
		mocks.deleteProject.mockRejectedValue(new ProjectNotEmpty())
		mocks.deleteStage.mockRejectedValue(new StageNotEmpty())
		const vm = await loaded()

		vm.open({ kind: 'deleteProject', node: tree[0]! })
		await vm.confirmDelete()
		expect(error).toHaveBeenCalledWith(
			m.catalog_project_not_empty_title({ project: 'auditfy' }),
			m.catalog_project_not_empty_body(),
		)

		vm.open({ kind: 'deleteStage', project: tree[0]!.project.slug, node: tree[0]!.stages[0]! })
		expect(vm.confirmation?.body).toBe(m.catalog_delete_stage_body_apps())
		await vm.confirmDelete()
		expect(mocks.deleteStage).toHaveBeenCalledWith({ project: 'auditfy', stage: 'staging' })
		expect(error).toHaveBeenCalledWith(
			m.catalog_stage_not_empty_title({ stage: 'staging' }),
			m.catalog_stage_not_empty_body(),
		)
		expect(vm.dialog).toBeNull()
	})
})
