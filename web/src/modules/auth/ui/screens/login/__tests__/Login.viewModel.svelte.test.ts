import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { LoginStep, LoginViewModel } from '..'
import { AccountLocked, InvalidCode, InvalidCredentials, SessionExpired } from '../../../../domain'
import LoginHarness from './LoginHarness.svelte'

const mocks = vi.hoisted(() => ({
	login: vi.fn(),
	completeTotp: vi.fn(),
	goto: vi.fn(),
}))

// Se mockea el CONTAINER, no la red: el test verifica lo que hace el viewModel con lo
// que le devuelven los casos de uso. `goto` no existe fuera del runtime de SvelteKit.
vi.mock('@/modules/auth/container', () => ({
	authUseCases: {
		getMe: vi.fn(),
		login: mocks.login,
		completeTotp: mocks.completeTotp,
		logout: vi.fn(),
		setup: vi.fn(),
		acceptInvite: vi.fn(),
		beginTotpSetup: vi.fn(),
		enableTotp: vi.fn(),
	},
}))

vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

async function mountViewModel(initialStep?: LoginStep): Promise<LoginViewModel> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vm: LoginViewModel | undefined
	await render(LoginHarness, {
		client,
		...(initialStep ? { initialStep } : {}),
		onReady: (instance) => {
			vm = instance
		},
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

async function submitPassword(vm: LoginViewModel, username: string, password: string) {
	vm.form.setFieldValue('username', username)
	vm.form.setFieldValue('password', password)
	await vm.form.handleSubmit()
}

describe('LoginViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.login.mockResolvedValue({ mfaRequired: false })
		mocks.completeTotp.mockResolvedValue(undefined)
		mocks.goto.mockResolvedValue(undefined)
	})

	it('sin 2FA entra directo al inicio', async () => {
		const vm = await mountViewModel()

		await submitPassword(vm, 'felipe', 'una-contraseña-larga')

		expect(mocks.login).toHaveBeenCalledWith({
			username: 'felipe',
			password: 'una-contraseña-larga',
		})
		expect(mocks.goto).toHaveBeenCalledWith('/')
		expect(vm.formError).toBeNull()
	})

	it('con 2FA pasa al paso del código sin navegar', async () => {
		mocks.login.mockResolvedValue({ mfaRequired: true })
		const vm = await mountViewModel()

		await submitPassword(vm, 'felipe', 'una-contraseña-larga')

		expect(vm.step).toBe('code')
		expect(mocks.goto).not.toHaveBeenCalled()
	})

	it('elige el mensaje por la clase del error de dominio', async () => {
		mocks.login.mockRejectedValueOnce(new InvalidCredentials())
		const vm = await mountViewModel()

		await submitPassword(vm, 'felipe', 'mala')
		expect(vm.formError).toBe(m.auth_error_invalid_credentials())
		expect(vm.step).toBe('password')

		mocks.login.mockRejectedValueOnce(new AccountLocked())
		await submitPassword(vm, 'felipe', 'mala')
		expect(vm.formError).toBe(m.auth_error_locked())
	})

	it('el código se manda recién completo y después entra', async () => {
		const vm = await mountViewModel('code')

		vm.setCode('123')
		expect(vm.isCodeComplete).toBe(false)
		await vm.submitCode()
		expect(mocks.completeTotp).not.toHaveBeenCalled()

		vm.setCode('123456')
		await vm.submitCode()

		expect(mocks.completeTotp).toHaveBeenCalledWith('123456')
		expect(mocks.goto).toHaveBeenCalledWith('/')
	})

	it('un código malo limpia el campo y lo dice', async () => {
		mocks.completeTotp.mockRejectedValue(new InvalidCode())
		const vm = await mountViewModel('code')

		vm.setCode('000000')
		await vm.submitCode()

		expect(vm.code).toBe('')
		expect(vm.formError).toBe(m.auth_error_invalid_code())
		expect(vm.step).toBe('code')
	})

	it('si la sesión pendiente se agotó, vuelve a la contraseña con el aviso', async () => {
		mocks.completeTotp.mockRejectedValue(new SessionExpired())
		const vm = await mountViewModel('code')

		vm.setCode('000000')
		await vm.submitCode()

		expect(vm.step).toBe('password')
		expect(vm.formError).toBe(m.auth_error_session_expired())
	})

	it('Volver deja la contraseña limpia de errores', async () => {
		mocks.completeTotp.mockRejectedValue(new InvalidCode())
		const vm = await mountViewModel('code')
		vm.setCode('000000')
		await vm.submitCode()

		vm.backToPassword()

		expect(vm.step).toBe('password')
		expect(vm.formError).toBeNull()
		expect(vm.code).toBe('')
	})
})
