import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { authKeys } from '@/modules/auth'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import type { UsersSectionViewModel } from '..'
import { CannotDeleteSelf, type User } from '../../../../domain'
import UsersSectionHarness from './UsersSectionHarness.svelte'
import { cacheHolds } from '../../../__tests__/cacheHolds'

const mocks = vi.hoisted(() => ({
	listUsers: vi.fn(),
	deleteUser: vi.fn(),
	resetUserTotp: vi.fn(),
	createInvite: vi.fn(),
}))

// Se mockea el CONTAINER, no la red: el test mira qué hace el viewModel con lo que le
// devuelven los casos de uso.
vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: mocks.listUsers,
		deleteUser: mocks.deleteUser,
		resetUserTotp: mocks.resetUserTotp,
		createInvite: mocks.createInvite,
		listTokens: vi.fn(),
		createToken: vi.fn(),
		revokeToken: vi.fn(),
		listScopeProjects: vi.fn(),
		listScopeStages: vi.fn(),
	},
}))

const USERS: User[] = [
	{ username: 'felipe', totpEnabled: true, createdAt: new Date('2026-10-09T12:00:00Z') },
	{ username: 'soporte', totpEnabled: true, createdAt: new Date('2026-10-10T12:00:00Z') },
	{ username: 'ana', totpEnabled: false, createdAt: new Date('2026-10-10T13:00:00Z') },
]

let client: QueryClient

async function mountViewModel(): Promise<UsersSectionViewModel> {
	client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	// El guard de `(app)` ya dejó al usuario de la sesión en el cache.
	client.setQueryData(authKeys.me(), {
		username: 'felipe',
		totpEnabled: true,
		needsTotpSetup: false,
	})
	let vm: UsersSectionViewModel | undefined
	await render(UsersSectionHarness, { client, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	await vi.waitUntil(() => !vm?.isLoading)
	return vm
}

function menuIds(vm: UsersSectionViewModel, username: string): string[] {
	const row = vm.rows.find((candidate) => candidate.user.username === username)
	return (row?.menu ?? []).map((entry) => entry.id)
}

describe('UsersSectionViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		vi.restoreAllMocks()
		mocks.listUsers.mockResolvedValue(USERS)
		mocks.deleteUser.mockResolvedValue(undefined)
		mocks.resetUserTotp.mockResolvedValue(undefined)
	})

	it('marca la fila propia y solo los demás tienen opciones', async () => {
		const vm = await mountViewModel()

		expect(vm.rows.map((row) => [row.user.username, row.isYou])).toEqual([
			['felipe', true],
			['soporte', false],
			['ana', false],
		])
		expect(menuIds(vm, 'felipe')).toEqual([])
		expect(menuIds(vm, 'soporte')).toEqual(['reset-totp', 'separator', 'delete'])
		// Sin 2FA no hay nada que quitar.
		expect(menuIds(vm, 'ana')).toEqual(['delete'])
	})

	it('eliminar pide confirmación y manda quién es la sesión', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel()

		vm.askDelete('soporte')
		expect(vm.confirmTitle).toBe(m.identity_users_delete_title({ username: 'soporte' }))
		expect(mocks.deleteUser).not.toHaveBeenCalled()

		await vm.confirm()

		expect(mocks.deleteUser).toHaveBeenCalledWith({
			username: 'soporte',
			currentUsername: 'felipe',
		})
		expect(vm.pending).toBeNull()
		expect(success).toHaveBeenCalledWith(m.identity_users_deleted({ username: 'soporte' }))
	})

	it('cannot_delete_self se muestra con un mensaje claro', async () => {
		const error = vi.spyOn(toast, 'error')
		mocks.deleteUser.mockRejectedValue(new CannotDeleteSelf())
		const vm = await mountViewModel()

		vm.askDelete('felipe')
		await vm.confirm()

		expect(error).toHaveBeenCalledWith(m.identity_error_cannot_delete_self())
		expect(vm.pending).toBeNull()
	})

	it('quitar el 2FA avisa que se cerraron sus sesiones', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel()

		vm.askResetTotp('soporte')
		expect(vm.confirmBody).toBe(m.identity_users_reset_body())
		await vm.confirm()

		expect(mocks.resetUserTotp).toHaveBeenCalledWith('soporte')
		expect(success).toHaveBeenCalledWith(
			m.identity_users_reset_done({ username: 'soporte' }),
			m.identity_users_reset_done_body(),
		)
	})

	it('cancelar no hace nada', async () => {
		const vm = await mountViewModel()

		vm.askResetTotp('soporte')
		vm.cancel()

		expect(vm.pending).toBeNull()
		expect(mocks.resetUserTotp).not.toHaveBeenCalled()
	})

	it('la invitación se muestra una vez, con su vencimiento, y se olvida al cerrar', async () => {
		const expiresAt = new Date(Date.now() + 24 * 60 * 60 * 1000)
		mocks.createInvite.mockResolvedValue({ url: 'http://panel/invite/abc', expiresAt })
		const vm = await mountViewModel()

		await vm.createInvite()

		expect(vm.invite?.url).toBe('http://panel/invite/abc')
		expect(vm.inviteExpiry).toContain('24')
		expect(await cacheHolds(client, 'invite/abc')).toBe(false)
		vm.closeInvite()
		expect(vm.invite).toBeNull()
	})

	it('un error al listar se muestra con reintento', async () => {
		mocks.listUsers.mockRejectedValue(new Error('boom'))
		const vm = await mountViewModel()

		expect(vm.loadError).toBe(m.identity_users_load_error())
	})
})
