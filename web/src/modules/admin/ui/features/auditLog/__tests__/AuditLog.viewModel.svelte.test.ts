import type { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { AuditLogViewModel } from '..'
import type { AuditEvent, AuditEventId, AuditPage } from '../../../../domain'
import AuditLogHarness from './AuditLogHarness.svelte'

const mocks = vi.hoisted(() => ({ listAudit: vi.fn(), listTokens: vi.fn() }))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: vi.fn(),
		updateSecurity: vi.fn(),
		updatePublicIp: vi.fn(),
		updateAcmeEmail: vi.fn(),
		updateProxyPorts: vi.fn(),
		replaceCloudflareToken: vi.fn(),
		listAudit: mocks.listAudit,
		listRegistries: vi.fn(),
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

// El nombre de los tokens sale de identity (por su barrel): también se mockea su container.
vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: vi.fn(),
		deleteUser: vi.fn(),
		resetUserTotp: vi.fn(),
		createInvite: vi.fn(),
		listTokens: mocks.listTokens,
		createToken: vi.fn(),
		revokeToken: vi.fn(),
		listScopeProjects: vi.fn(),
		listScopeStages: vi.fn(),
	},
}))

function event(id: number, actor: AuditEvent['actor']): AuditEvent {
	return {
		id: id as AuditEventId,
		actor,
		action: 'env.patch',
		target: 'app:auditfy/staging/api',
		at: new Date('2026-10-10T14:27:00Z'),
	}
}

const FIRST: AuditPage = {
	items: [
		event(4, { kind: 'user', id: 1 }),
		event(3, { kind: 'token', id: 7 }),
		event(2, { kind: 'token', id: 99 }),
	],
	nextCursor: '2',
}

const LAST: AuditPage = { items: [event(1, { kind: 'system' })], nextCursor: null }

let client: QueryClient

async function mountViewModel(): Promise<AuditLogViewModel> {
	client = createQueryClient({ retry: false })
	let vm: AuditLogViewModel | undefined
	await render(AuditLogHarness, { client, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	await vi.waitUntil(() => !vm?.isLoading)
	return vm
}

describe('AuditLogViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listAudit.mockImplementation((input: { before: string | null }) =>
			Promise.resolve(input.before === null ? FIRST : LAST),
		)
		mocks.listTokens.mockResolvedValue([{ id: 7, name: 'github-actions' }])
	})

	it('muestra quién con el nombre del token si existe, o su id', async () => {
		const vm = await mountViewModel()
		await vi.waitUntil(() => vm.rows[1]?.actor.kind === 'token')

		expect(vm.rows.map((row) => row.actor)).toEqual([
			{ kind: 'text', text: m.admin_audit_actor_user({ id: 1 }) },
			{ kind: 'token', name: 'github-actions' },
			{ kind: 'text', text: m.admin_audit_actor_token_id({ id: 99 }) },
		])
		expect(vm.rows[0]).toMatchObject({
			action: 'env.patch',
			target: 'app:auditfy/staging/api',
			iso: '2026-10-10T14:27:00.000Z',
		})
	})

	it('"Cargar más" pide la página siguiente con before y la agrega abajo', async () => {
		const vm = await mountViewModel()
		expect(vm.hasMore).toBe(true)

		vm.loadMore()
		await vi.waitUntil(() => vm.rows.length === 4)

		expect(mocks.listAudit).toHaveBeenLastCalledWith({ before: '2' }, expect.anything())
		expect(vm.rows.at(-1)?.actor).toEqual({ kind: 'text', text: m.admin_audit_actor_system() })
		expect(vm.hasMore).toBe(false)
	})

	it('una escritura de otro módulo (borrar un usuario, crear un token) refresca la auditoría', async () => {
		await mountViewModel()
		expect(mocks.listAudit).toHaveBeenCalledTimes(1)

		// Una mutación cualquiera del cache, como las de identity: no conoce las keys de admin.
		await client
			.getMutationCache()
			.build(client, { mutationFn: () => Promise.resolve('ok') })
			.execute(undefined)

		await vi.waitUntil(() => mocks.listAudit.mock.calls.length > 1)
		expect(mocks.listAudit).toHaveBeenLastCalledWith({ before: null }, expect.anything())
	})

	it('una escritura que falla no refresca nada', async () => {
		await mountViewModel()

		await client
			.getMutationCache()
			.build(client, { mutationFn: () => Promise.reject(new Error('boom')) })
			.execute(undefined)
			.catch(() => undefined)

		await new Promise((resolve) => setTimeout(resolve, 50))
		expect(mocks.listAudit).toHaveBeenCalledTimes(1)
	})

	it('sin eventos, el estado vacío; un error al cargar, el mensaje', async () => {
		mocks.listAudit.mockResolvedValueOnce({ items: [], nextCursor: null })
		const empty = await mountViewModel()
		expect(empty.isEmpty).toBe(true)

		mocks.listAudit.mockRejectedValueOnce(new Error('boom'))
		const failed = await mountViewModel()
		expect(failed.loadError).toBe(m.admin_audit_load_error())
	})
})
