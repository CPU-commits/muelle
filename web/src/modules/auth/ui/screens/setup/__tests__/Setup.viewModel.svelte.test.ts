import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { SetupViewModel } from '..'
import { AlreadySetUp, InvalidInvite, UsernameTaken } from '../../../../domain'
import SetupHarness from './SetupHarness.svelte'

const mocks = vi.hoisted(() => ({
	setup: vi.fn(),
	goto: vi.fn(),
	replaceState: vi.fn(),
	// Como SvelteKit: el callback corre cuando terminó la navegación, después de montar.
	afterNavigate: vi.fn((callback: () => void) => queueMicrotask(callback)),
}))

vi.mock('@/modules/auth/container', () => ({
	authUseCases: {
		getMe: vi.fn(),
		login: vi.fn(),
		completeTotp: vi.fn(),
		logout: vi.fn(),
		setup: mocks.setup,
		acceptInvite: vi.fn(),
		beginTotpSetup: vi.fn(),
		enableTotp: vi.fn(),
	},
}))

vi.mock('$app/navigation', () => ({
	afterNavigate: mocks.afterNavigate,
	goto: mocks.goto,
	replaceState: mocks.replaceState,
}))

const values = { username: 'admin', password: 'una-contraseña-larga', passwordRepeat: '' }

async function mountViewModel(token: string | null): Promise<SetupViewModel> {
	const client = createQueryClient({ retry: false })
	let vm: SetupViewModel | undefined
	await render(SetupHarness, { client, token, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('SetupViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.setup.mockResolvedValue('admin')
		mocks.goto.mockResolvedValue(undefined)
	})

	it('lee el token una vez y lo saca de la URL', async () => {
		const vm = await mountViewModel('tok')

		expect(vm.hasToken).toBe(true)
		await expect.poll(() => mocks.replaceState).toHaveBeenCalledWith('/setup', {})
	})

	it('sin token no se puede enviar', async () => {
		const vm = await mountViewModel(null)

		expect(vm.hasToken).toBe(false)
		expect(vm.canSubmit).toBe(false)
		expect(mocks.replaceState).not.toHaveBeenCalled()
	})

	it('crea el primer usuario con el token y va a configurar 2FA', async () => {
		const vm = await mountViewModel('tok')

		await vm.submit(values)

		expect(mocks.setup).toHaveBeenCalledWith({
			token: 'tok',
			username: 'admin',
			password: 'una-contraseña-larga',
		})
		expect(mocks.goto).toHaveBeenCalledWith('/2fa')
	})

	it('si ya hay usuarios, apaga el formulario', async () => {
		mocks.setup.mockRejectedValue(new AlreadySetUp())
		const vm = await mountViewModel('tok')

		await vm.submit(values)

		expect(vm.alreadySetUp).toBe(true)
		expect(vm.canSubmit).toBe(false)
		expect(mocks.goto).not.toHaveBeenCalled()
	})

	it('un nombre tomado va al campo; un token que no vale, arriba', async () => {
		mocks.setup.mockRejectedValueOnce(new UsernameTaken())
		const vm = await mountViewModel('tok')

		await vm.submit(values)
		expect(vm.serverErrors.username).toBe(m.auth_error_username_taken())

		mocks.setup.mockRejectedValueOnce(new InvalidInvite())
		await vm.submit(values)
		expect(vm.serverErrors.username).toBeNull()
		expect(vm.formError).toBe(m.auth_setup_invalid_token())
	})
})
