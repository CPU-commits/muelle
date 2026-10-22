import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import {
	ALL_PROJECTS,
	ALL_PROJECTS_EXPIRY_DAYS,
	type ApiToken,
	type CreatedApiToken,
	expiryInputInDays,
	type TokenId,
} from '../../../../domain'
import TokensScreenView from './TokensScreenView.svelte'

const mocks = vi.hoisted(() => ({
	listTokens: vi.fn(),
	createToken: vi.fn(),
	listScopeProjects: vi.fn(),
	listScopeStages: vi.fn(),
}))

vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: vi.fn(),
		deleteUser: vi.fn(),
		resetUserTotp: vi.fn(),
		createInvite: vi.fn(),
		listTokens: mocks.listTokens,
		createToken: mocks.createToken,
		revokeToken: vi.fn(),
		listScopeProjects: mocks.listScopeProjects,
		listScopeStages: mocks.listScopeStages,
	},
}))

const TOKEN: ApiToken = {
	id: 9 as TokenId,
	name: 'github-actions',
	scope: { kind: 'project', project: 'auditfy', stage: null },
	readOnly: false,
	createdBy: { kind: 'user', username: 'felipe' },
	createdAt: new Date('2026-10-10T12:00:00Z'),
	lastUsedAt: null,
	expiresAt: null,
}

const CREATED: CreatedApiToken = { token: TOKEN, secret: 'mk_secreto' }

describe('TokensScreen', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listTokens.mockResolvedValue([])
		mocks.listScopeProjects.mockResolvedValue([{ slug: 'auditfy', name: 'Auditfy' }])
		mocks.listScopeStages.mockResolvedValue([{ slug: 'staging' }])
		mocks.createToken.mockImplementation(() => {
			mocks.listTokens.mockResolvedValue([TOKEN])
			return Promise.resolve(CREATED)
		})
	})

	it('el token nuevo no se pierde con Escape: solo "Listo" lo esconde', async () => {
		const screen = await render(TokensScreenView, {
			client: createQueryClient({ retry: false }),
		})

		await screen.getByRole('button', { name: 'Crear token' }).click()
		const form = screen.getByRole('dialog', { name: 'Crear token' })
		await form.getByLabelText('Nombre').fill('github-actions')
		await expect.element(form.getByRole('option', { name: 'auditfy' })).toBeInTheDocument()
		await form.getByLabelText('Proyecto').selectOptions('auditfy')
		await form.getByRole('button', { name: 'Crear token' }).click()

		const card = screen.getByRole('heading', { name: /Token creado/ })
		await expect.element(card).toBeVisible()
		const secret = screen.getByLabelText('Token', { exact: true })
		await expect.element(secret).toHaveValue('mk_secreto')

		// Chrome deja frenar un solo Escape sin activación del usuario: varios seguidos
		// cierran un modal común. La tarjeta del token no es un modal, y no se va.
		for (let press = 0; press < 6; press += 1) await userEvent.keyboard('{Escape}')
		await expect.element(card).toBeVisible()
		await expect.element(secret).toHaveValue('mk_secreto')

		await screen.getByRole('button', { name: 'Listo' }).click()
		await expect.element(card).not.toBeInTheDocument()
	})

	it('el modal ofrece "Solo lectura" y "Todos los proyectos (solo lectura)", sin stage y a 90 días', async () => {
		const screen = await render(TokensScreenView, {
			client: createQueryClient({ retry: false }),
		})

		await screen.getByRole('button', { name: 'Crear token' }).click()
		const form = screen.getByRole('dialog', { name: 'Crear token' })
		const readOnly = form.getByRole('checkbox', { name: 'Solo lectura' })
		await expect.element(readOnly).not.toBeChecked()
		await expect.element(readOnly).toBeEnabled()
		await expect.element(form.getByText(/Recomendado para Claude Code/)).toBeVisible()
		await expect.element(form.getByRole('option', { name: 'auditfy' })).toBeInTheDocument()

		await form.getByLabelText('Proyecto').selectOptions('Todos los proyectos (solo lectura)')

		const stage = form.getByLabelText('Stage')
		await expect.element(stage).toBeDisabled()
		await expect.element(stage).toHaveDisplayValue('—')
		await expect.element(readOnly).toBeChecked()
		await expect.element(readOnly).toBeDisabled()
		const in90Days = expiryInputInDays(ALL_PROJECTS_EXPIRY_DAYS)
		await expect.element(form.getByLabelText('Vence')).toHaveValue(in90Days)
		await expect
			.element(form.getByText(/siempre es de solo lectura\. Vence en 90 días/))
			.toBeVisible()

		await form.getByLabelText('Nombre').fill('claude-debug')
		await form.getByRole('button', { name: 'Crear token' }).click()

		await expect.poll(() => mocks.createToken.mock.calls.length).toBe(1)
		expect(mocks.createToken).toHaveBeenCalledWith({
			name: 'claude-debug',
			project: ALL_PROJECTS,
			stage: '',
			readOnly: true,
			expiresOn: in90Days,
		})
	})
})
