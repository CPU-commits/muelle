import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { UnavailableError } from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import type { TotpSetupViewModel } from '..'
import { InvalidCode, type Me, NoPendingTotp } from '../../../../domain'
import {
	FAKE_TOTP_SETUP,
	fakeMe,
} from '../../../../domain/repositories/__fakes__/inMemoryAuthRepository'
import TotpSetupHarness from './TotpSetupHarness.svelte'

const mocks = vi.hoisted(() => ({
	beginTotpSetup: vi.fn(),
	enableTotp: vi.fn(),
	logout: vi.fn(),
	goto: vi.fn(),
}))

vi.mock('@/modules/auth/container', () => ({
	authUseCases: {
		getMe: vi.fn(),
		login: vi.fn(),
		completeTotp: vi.fn(),
		logout: mocks.logout,
		setup: vi.fn(),
		acceptInvite: vi.fn(),
		beginTotpSetup: mocks.beginTotpSetup,
		enableTotp: mocks.enableTotp,
	},
}))

vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

async function mountViewModel(me: Me): Promise<TotpSetupViewModel> {
	const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	let vm: TotpSetupViewModel | undefined
	await render(TotpSetupHarness, { client, me, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('TotpSetupViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		toasts.items = []
		mocks.beginTotpSetup.mockResolvedValue(FAKE_TOTP_SETUP)
		mocks.enableTotp.mockResolvedValue(undefined)
		mocks.logout.mockResolvedValue(undefined)
		mocks.goto.mockResolvedValue(undefined)
	})

	it('sin 2FA genera el secreto al entrar y lo agrupa para copiarlo', async () => {
		const vm = await mountViewModel(fakeMe())

		await expect.poll(() => vm.setup).toEqual(FAKE_TOTP_SETUP)
		expect(vm.secretGroups).toBe('JBSW Y3DP EHPK 3PXP')
	})

	it('con 2FA activa no genera nada hasta que se pide', async () => {
		const vm = await mountViewModel(fakeMe({ totpEnabled: true }))

		expect(vm.alreadyEnabled).toBe(true)
		expect(mocks.beginTotpSetup).not.toHaveBeenCalled()
		await vm.start()
		expect(vm.setup).toEqual(FAKE_TOTP_SETUP)
	})

	it('con el código completo activa 2FA, avisa y va al inicio', async () => {
		const vm = await mountViewModel(fakeMe())
		await expect.poll(() => vm.setup).not.toBeNull()

		vm.setCode('123456')
		await vm.submit()

		expect(mocks.enableTotp).toHaveBeenCalledWith('123456')
		expect(toasts.items.map((item) => item.title)).toContain(m.auth_2fa_enabled_ok())
		expect(mocks.goto).toHaveBeenCalledWith('/')
	})

	it('un código malo se borra y lo dice; un secreto vencido obliga a generar otro', async () => {
		const vm = await mountViewModel(fakeMe())
		await expect.poll(() => vm.setup).not.toBeNull()

		mocks.enableTotp.mockRejectedValueOnce(new InvalidCode())
		vm.setCode('000000')
		await vm.submit()
		expect(vm.code).toBe('')
		expect(vm.formError).toBe(m.auth_error_invalid_code())

		mocks.enableTotp.mockRejectedValueOnce(new NoPendingTotp())
		vm.setCode('123456')
		await vm.submit()
		expect(vm.setup).toBeNull()
		expect(vm.formError).toBe(m.auth_error_no_pending_totp())
	})

	it('si es obligatoria se puede cerrar sesión; si eso falla, se queda y lo dice', async () => {
		const vm = await mountViewModel(fakeMe({ needsTotpSetup: true }))
		expect(vm.isRequired).toBe(true)

		mocks.logout.mockRejectedValueOnce(new UnavailableError('caído'))
		await vm.logout()
		expect(vm.formError).toBe(m.auth_error_logout())
		expect(mocks.goto).not.toHaveBeenCalled()

		await vm.logout()
		expect(mocks.goto).toHaveBeenCalledWith('/login')
	})
})
