import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { InviteViewModel } from '..'
import { InvalidInvite } from '../../../../domain'
import InviteHarness from './InviteHarness.svelte'

const mocks = vi.hoisted(() => ({
	acceptInvite: vi.fn(),
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
		setup: vi.fn(),
		acceptInvite: mocks.acceptInvite,
		beginTotpSetup: vi.fn(),
		enableTotp: vi.fn(),
	},
}))

vi.mock('$app/navigation', () => ({
	afterNavigate: mocks.afterNavigate,
	goto: mocks.goto,
	replaceState: mocks.replaceState,
}))

const values = { username: 'soporte', password: 'clave-de-soporte', passwordRepeat: '' }

async function mountViewModel(token: string | null): Promise<InviteViewModel> {
	const client = createQueryClient({ retry: false })
	let vm: InviteViewModel | undefined
	await render(InviteHarness, { client, token, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('InviteViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.acceptInvite.mockResolvedValue('soporte')
		mocks.goto.mockResolvedValue(undefined)
	})

	it('saca el token de la URL y crea la cuenta con él', async () => {
		const vm = await mountViewModel('inv-1')
		await expect.poll(() => mocks.replaceState).toHaveBeenCalledWith('/invite', {})

		await vm.submit(values)

		expect(mocks.acceptInvite).toHaveBeenCalledWith({
			token: 'inv-1',
			username: 'soporte',
			password: 'clave-de-soporte',
		})
		expect(mocks.goto).toHaveBeenCalledWith('/2fa')
	})

	it('una invitación que no vale apaga el formulario', async () => {
		mocks.acceptInvite.mockRejectedValue(new InvalidInvite())
		const vm = await mountViewModel('inv-1')

		await vm.submit(values)

		expect(vm.invalid).toBe(true)
		expect(vm.formError).toBe(m.auth_invite_invalid())
		expect(mocks.goto).not.toHaveBeenCalled()
	})

	it('sin token (se recargó /invite) lo dice y no deja enviar', async () => {
		const vm = await mountViewModel(null)

		expect(vm.hasToken).toBe(false)
		expect(vm.canSubmit).toBe(false)
	})
})
