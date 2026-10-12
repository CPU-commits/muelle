import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { UnavailableError } from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import type { SessionShellViewModel } from '..'
import { fakeMe } from '../../../../domain/repositories/__fakes__/inMemoryAuthRepository'
import SessionShellHarness from './SessionShellHarness.svelte'

const mocks = vi.hoisted(() => ({
	getMe: vi.fn(),
	logout: vi.fn(),
	goto: vi.fn(),
}))

vi.mock('@/modules/auth/container', () => ({
	authUseCases: {
		getMe: mocks.getMe,
		login: vi.fn(),
		completeTotp: vi.fn(),
		logout: mocks.logout,
		setup: vi.fn(),
		acceptInvite: vi.fn(),
		beginTotpSetup: vi.fn(),
		enableTotp: vi.fn(),
	},
}))

vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

async function mountViewModel(): Promise<{ vm: SessionShellViewModel; client: QueryClient }> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vm: SessionShellViewModel | undefined
	await render(SessionShellHarness, {
		client,
		me: fakeMe({ totpEnabled: true }),
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return { vm, client }
}

describe('SessionShellViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		toasts.items = []
		mocks.getMe.mockResolvedValue(fakeMe({ totpEnabled: true }))
		mocks.logout.mockResolvedValue(undefined)
		mocks.goto.mockResolvedValue(undefined)
	})

	it('arma el menú de la cuenta: 2FA, idioma y cerrar sesión', async () => {
		const { vm } = await mountViewModel()

		expect(vm.username).toBe('felipe')
		expect(vm.totpEnabled).toBe(true)
		expect(vm.accountItems.map((item) => item.id)).toEqual([
			'totp',
			'locale',
			'separator',
			'logout',
		])
	})

	it('cerrar sesión vacía el cache y va al login', async () => {
		const { vm, client } = await mountViewModel()
		client.setQueryData(['catalog', 'projects'], ['auditfy'])

		await vm.logout()

		expect(mocks.goto).toHaveBeenCalledWith('/login')
		expect(client.getQueryData(['catalog', 'projects'])).toBeUndefined()
	})

	it('si cerrar sesión falla, se queda, lo dice y no borra nada', async () => {
		mocks.logout.mockRejectedValue(new UnavailableError('caído'))
		const { vm, client } = await mountViewModel()
		client.setQueryData(['catalog', 'projects'], ['auditfy'])

		await vm.logout()

		expect(mocks.goto).not.toHaveBeenCalled()
		expect(toasts.items.map((item) => item.title)).toContain(m.auth_error_logout())
		expect(client.getQueryData(['catalog', 'projects'])).toEqual(['auditfy'])
	})
})
