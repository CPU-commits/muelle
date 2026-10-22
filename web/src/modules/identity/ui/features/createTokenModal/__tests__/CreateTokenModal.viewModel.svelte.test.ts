import type { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import type { CreateTokenModalViewModel } from '..'
import {
	type CreatedApiToken,
	InvalidTokenName,
	type TokenId,
	TokenScopeNotFound,
} from '../../../../domain'
import CreateTokenModalHarness from './CreateTokenModalHarness.svelte'
import { cacheHolds } from '../../../__tests__/cacheHolds'

const mocks = vi.hoisted(() => ({
	listScopeProjects: vi.fn(),
	listScopeStages: vi.fn(),
	createToken: vi.fn(),
	onCreated: vi.fn(),
}))

vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: vi.fn(),
		deleteUser: vi.fn(),
		resetUserTotp: vi.fn(),
		createInvite: vi.fn(),
		listTokens: vi.fn(),
		createToken: mocks.createToken,
		revokeToken: vi.fn(),
		listScopeProjects: mocks.listScopeProjects,
		listScopeStages: mocks.listScopeStages,
	},
}))

const CREATED: CreatedApiToken = {
	token: {
		id: 9 as TokenId,
		name: 'ci',
		scope: { kind: 'project', project: 'auditfy', stage: 'staging' },
		readOnly: false,
		createdBy: { kind: 'user', username: 'felipe' },
		createdAt: new Date(),
		lastUsedAt: null,
		expiresAt: null,
	},
	secret: 'mk_secreto',
}

let client: QueryClient

async function mountViewModel(): Promise<CreateTokenModalViewModel> {
	client = createQueryClient({ retry: false })
	let vm: CreateTokenModalViewModel | undefined
	await render(CreateTokenModalHarness, {
		client,
		onCreated: mocks.onCreated,
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	await vi.waitUntil(() => !vm?.isLoadingProjects)
	return vm
}

describe('CreateTokenModalViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listScopeProjects.mockResolvedValue([
			{ slug: 'auditfy', name: 'Auditfy' },
			{ slug: 'extractor', name: 'Extractor' },
		])
		mocks.listScopeStages.mockImplementation((project: string) =>
			Promise.resolve(
				project === 'auditfy' ? [{ slug: 'staging' }, { slug: 'production' }] : [],
			),
		)
		mocks.createToken.mockResolvedValue(CREATED)
	})

	it('los stages salen del proyecto elegido; cambiar de proyecto vuelve a "todo"', async () => {
		const vm = await mountViewModel()

		expect(vm.projects.map((project) => project.slug)).toEqual(['auditfy', 'extractor'])
		expect(vm.stages).toEqual([])
		expect(mocks.listScopeStages).not.toHaveBeenCalled()

		vm.setProject('auditfy')
		await vi.waitUntil(() => vm.stages.length > 0)
		expect(vm.stages.map((stage) => stage.slug)).toEqual(['staging', 'production'])

		vm.setStage('staging')
		vm.setProject('extractor')
		expect(vm.stage).toBe('')
	})

	it('crea con lo elegido y entrega el token a la pantalla', async () => {
		const vm = await mountViewModel()
		vm.setName('ci')
		vm.setProject('auditfy')
		vm.setStage('staging')
		vm.setExpiresOn('2026-12-31')

		await vm.submit()

		expect(mocks.createToken).toHaveBeenCalledWith({
			name: 'ci',
			project: 'auditfy',
			stage: 'staging',
			expiresOn: '2026-12-31',
		})
		expect(mocks.onCreated).toHaveBeenCalledWith(CREATED)
		// El valor en claro solo llega a la pantalla: no queda en el cache.
		expect(await cacheHolds(client, 'mk_secreto')).toBe(false)
	})

	it('un error de validación va a su campo y se limpia al escribir', async () => {
		const vm = await mountViewModel()
		vm.setProject('auditfy')

		await vm.submit()
		expect(vm.errors).toEqual({ name: m.common_required() })
		expect(mocks.createToken).not.toHaveBeenCalled()

		vm.setName('ci')
		expect(vm.errors).toEqual({})
	})

	it('un error del caso de uso también va a su campo', async () => {
		mocks.createToken.mockRejectedValue(new InvalidTokenName('invisible'))
		const vm = await mountViewModel()
		vm.setName('ci')
		vm.setProject('auditfy')

		await vm.submit()

		expect(vm.errors).toEqual({ name: m.identity_error_token_name_invisible() })
		expect(mocks.onCreated).not.toHaveBeenCalled()
	})

	it('marca todos los campos mal de una vez, sin mandar nada', async () => {
		const vm = await mountViewModel()
		vm.setExpiresOn('2020-01-01')

		await vm.submit()

		expect(vm.errors).toEqual({
			name: m.common_required(),
			project: m.identity_error_token_project(),
			expiresOn: m.identity_error_token_expiry_past(),
		})
		expect(mocks.createToken).not.toHaveBeenCalled()
	})

	it('un 422 del backend va al campo con su motivo', async () => {
		mocks.createToken.mockRejectedValue(
			new ValidationError('debe estar entre 0 y 3650', 'expiresOn'),
		)
		const vm = await mountViewModel()
		vm.setName('ci')
		vm.setProject('auditfy')

		await vm.submit()

		expect(vm.errors).toEqual({ expiresOn: 'debe estar entre 0 y 3650' })
	})

	it('si el proyecto se borró, lo dice en el campo y recarga la lista', async () => {
		mocks.createToken.mockRejectedValue(new TokenScopeNotFound())
		const vm = await mountViewModel()
		vm.setName('ci')
		vm.setProject('auditfy')

		await vm.submit()

		expect(vm.errors.project).toBe(m.identity_error_scope_not_found())
		expect(mocks.listScopeProjects).toHaveBeenCalledTimes(2)
	})

	it('sin proyectos no se puede crear', async () => {
		mocks.listScopeProjects.mockResolvedValue([])
		const vm = await mountViewModel()

		expect(vm.hasNoProjects).toBe(true)
		expect(vm.canSubmit).toBe(false)
		await vm.submit()
		expect(mocks.createToken).not.toHaveBeenCalled()
	})

	it('si los proyectos no cargan, lo dice y no deja crear', async () => {
		mocks.listScopeProjects.mockRejectedValue(new Error('boom'))
		const vm = await mountViewModel()

		expect(vm.projectsError).toBe(m.identity_token_form_projects_error())
		expect(vm.canSubmit).toBe(false)
	})
})
