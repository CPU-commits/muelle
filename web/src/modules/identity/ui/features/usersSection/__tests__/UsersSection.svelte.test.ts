import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { authKeys } from '@/modules/auth'
import UsersSectionView from './UsersSectionView.svelte'

const mocks = vi.hoisted(() => ({ listUsers: vi.fn(), createInvite: vi.fn() }))

vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: mocks.listUsers,
		deleteUser: vi.fn(),
		resetUserTotp: vi.fn(),
		createInvite: mocks.createInvite,
		listTokens: vi.fn(),
		createToken: vi.fn(),
		revokeToken: vi.fn(),
		listScopeProjects: vi.fn(),
		listScopeStages: vi.fn(),
	},
}))

function client(): QueryClient {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	queryClient.setQueryData(authKeys.me(), {
		username: 'felipe',
		totpEnabled: true,
		needsTotpSetup: false,
	})
	return queryClient
}

describe('UsersSection', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listUsers.mockResolvedValue([
			{ username: 'felipe', totpEnabled: true, createdAt: new Date('2026-10-09T12:00:00Z') },
		])
		mocks.createInvite.mockResolvedValue({
			url: 'http://panel/invite/abc',
			expiresAt: new Date(Date.now() + 24 * 60 * 60 * 1000),
		})
	})

	it('la URL de la invitación solo se cierra con "Listo": Escape no la pierde', async () => {
		const screen = await render(UsersSectionView, { client: client() })

		await screen.getByRole('button', { name: 'Invitar' }).click()
		const dialog = screen.getByRole('dialog', { name: 'Invitación creada' })
		await expect.element(dialog).toBeVisible()
		await expect
			.element(screen.getByLabelText('Link de invitación'))
			.toHaveValue('http://panel/invite/abc')

		await userEvent.keyboard('{Escape}')
		await expect.element(dialog).toBeVisible()

		await screen.getByRole('button', { name: 'Listo' }).click()
		await expect.element(dialog).not.toBeInTheDocument()
	})
})
