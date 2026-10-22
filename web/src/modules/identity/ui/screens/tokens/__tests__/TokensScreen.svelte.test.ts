import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { aliveAfterGc } from '@/shared/ui/components/__tests__/memory'
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

// `page` de `$app/state` no existe fuera del runtime de SvelteKit. El panel, detrás de
// muelle-caddy en un puerto HTTPS que no es 443.
vi.mock('$app/state', () => ({
	page: { url: new URL('https://panel.example:8443/settings/tokens') },
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

	describe('Conectar con Claude Code', () => {
		const MCP_URL = 'https://panel.example:8443/api/v1/mcp'
		const COMMAND = `claude mcp add --transport http --scope user muelle ${MCP_URL} --header "Authorization: Bearer mk_secreto"`

		async function createToken(created: CreatedApiToken = CREATED) {
			mocks.createToken.mockImplementation(() => {
				mocks.listTokens.mockResolvedValue([created.token])
				return Promise.resolve(created)
			})
			const screen = await render(TokensScreenView, {
				client: createQueryClient({ retry: false }),
			})
			await screen.getByRole('button', { name: 'Crear token' }).click()
			const form = screen.getByRole('dialog', { name: 'Crear token' })
			await form.getByLabelText('Nombre').fill(created.token.name)
			await expect.element(form.getByRole('option', { name: 'auditfy' })).toBeInTheDocument()
			await form.getByLabelText('Proyecto').selectOptions('auditfy')
			await form.getByRole('button', { name: 'Crear token' }).click()
			// Con `expect.poll` y no `expect.element`, que se queda con el elemento que encontró
			// (y la prueba de memoria lo vería vivo).
			await expect
				.poll(() => document.querySelector('h4')?.textContent)
				.toBe('Conectar con Claude Code')
			return screen
		}

		const textOf = (label: string) =>
			document.querySelector(`[aria-label="${label}"]`)?.textContent ?? null

		it('muestra el comando con el token y el .mcp.json, cada uno con su botón de copiar', async () => {
			const writeText = vi
				.spyOn(navigator.clipboard, 'writeText')
				.mockResolvedValue(undefined)
			const screen = await createToken()

			expect(textOf('Comando para agregar muelle a Claude Code')).toBe(COMMAND)
			const json = textOf('Ejemplo de .mcp.json') ?? ''
			expect(JSON.parse(json)).toEqual({
				mcpServers: {
					muelle: {
						type: 'http',
						url: MCP_URL,
						headers: { Authorization: 'Bearer ${MUELLE_TOKEN}' },
					},
				},
			})
			await expect
				.element(screen.getByText(/cada persona exporta MUELLE_TOKEN en su shell/))
				.toBeVisible()

			// El token, el comando y el .mcp.json: tres botones de copiar, en ese orden.
			const copies = screen.getByRole('button', { name: 'Copiar' })
			await expect.poll(() => copies.elements().length).toBe(3)
			await copies.nth(1).click()
			expect(writeText).toHaveBeenLastCalledWith(COMMAND)
			await copies.nth(2).click()
			expect(writeText).toHaveBeenLastCalledWith(json)
		})

		it('si no se puede copiar, deja seleccionado el comando para copiarlo a mano', async () => {
			vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denegado'))
			vi.spyOn(document, 'execCommand').mockReturnValue(false)
			const screen = await createToken()

			await screen.getByRole('button', { name: 'Copiar' }).nth(1).click()

			await expect.poll(() => window.getSelection()?.toString()).toBe(COMMAND)
		})

		it('un token de escritura trae el ejemplo de GitHub Actions y el aviso de escritura', async () => {
			const screen = await createToken()

			await expect
				.element(screen.getByLabelText('Ejemplo de uso en GitHub Actions'))
				.toBeVisible()
			await expect
				.element(screen.getByText(/Este token puede desplegar y cambiar variables/))
				.toBeVisible()
		})

		it('uno de solo lectura: sin Actions ni aviso, y la fila lo marca', async () => {
			const readOnly: ApiToken = { ...TOKEN, name: 'claude', readOnly: true }
			const screen = await createToken({ token: readOnly, secret: 'mk_secreto' })

			expect(textOf('Comando para agregar muelle a Claude Code')).toBe(COMMAND)
			expect(textOf('Ejemplo de uso en GitHub Actions')).toBeNull()
			await expect
				.element(screen.getByText(/Este token puede desplegar/))
				.not.toBeInTheDocument()
			const list = screen.getByRole('list', { name: 'Tokens activos' })
			await expect.element(list.getByText('solo lectura', { exact: true })).toBeVisible()
		})

		it('después de "Listo" no queda vivo nada de lo que mostraba el token', async () => {
			const screen = await createToken()
			const refs = [
				'Token',
				'Comando para agregar muelle a Claude Code',
				'Ejemplo de .mcp.json',
			].map((label) => {
				const element = document.querySelector(`[aria-label="${label}"]`)
				if (!element) throw new Error(`falta ${label}`)
				return new WeakRef(element)
			})

			await screen.getByRole('button', { name: 'Listo' }).click()
			await expect
				.poll(() => document.querySelector('[aria-label="Ejemplo de .mcp.json"]'))
				.toBeNull()
			// Svelte guarda el último evento delegado (el clic en "Listo") hasta el próximo.
			await userEvent.click(screen.getByRole('heading', { name: 'Tokens de API' }))

			expect(await aliveAfterGc(refs)).toBe(0)
		})
	})
})
