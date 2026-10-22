import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import type { TokensViewModel } from '..'
import { UnavailableError } from '@/shared/domain'
import { type ApiToken, type TokenId, TokenNotFound } from '../../../../domain'
import TokensHarness from './TokensHarness.svelte'

const mocks = vi.hoisted(() => ({
	listTokens: vi.fn(),
	revokeToken: vi.fn(),
}))

vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: vi.fn(),
		deleteUser: vi.fn(),
		resetUserTotp: vi.fn(),
		createInvite: vi.fn(),
		listTokens: mocks.listTokens,
		createToken: vi.fn(),
		revokeToken: mocks.revokeToken,
		listScopeProjects: vi.fn(),
		listScopeStages: vi.fn(),
	},
}))

function token(overrides: Partial<ApiToken>): ApiToken {
	return {
		id: 1 as TokenId,
		name: 'github-actions',
		scope: { kind: 'project', project: 'auditfy', stage: null },
		readOnly: false,
		createdBy: { kind: 'user', username: 'felipe' },
		createdAt: new Date('2026-10-01T12:00:00Z'),
		lastUsedAt: null,
		expiresAt: null,
		...overrides,
	}
}

const TOKENS: ApiToken[] = [
	token({ id: 1 as TokenId }),
	token({
		id: 2 as TokenId,
		name: 'extractor-ci',
		scope: { kind: 'project', project: 'extractor', stage: 'staging' },
		createdBy: { kind: 'deleted' },
		lastUsedAt: new Date(Date.now() - 12 * 60_000),
	}),
	token({
		id: 3 as TokenId,
		name: 'viejo',
		createdBy: { kind: 'user', username: 'soporte' },
		expiresAt: new Date('2026-01-01T00:00:00Z'),
	}),
]

async function mountViewModel(): Promise<TokensViewModel> {
	const client = createQueryClient({ retry: false })
	let vm: TokensViewModel | undefined
	await render(TokensHarness, { client, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	await vi.waitUntil(() => !vm?.isLoading)
	return vm
}

describe('TokensViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		vi.restoreAllMocks()
		mocks.listTokens.mockResolvedValue(TOKENS)
		mocks.revokeToken.mockResolvedValue(undefined)
	})

	it('separa vigentes de vencidos con alcance, uso, vencimiento y creador', async () => {
		const vm = await mountViewModel()

		expect(vm.activeRows.map((row) => row.token.name)).toEqual([
			'github-actions',
			'extractor-ci',
		])
		expect(vm.expiredRows.map((row) => row.token.name)).toEqual(['viejo'])

		const [first, second] = vm.activeRows
		expect(first).toMatchObject({
			scope: 'auditfy',
			lastUsed: m.identity_tokens_never_used(),
			expiry: m.identity_tokens_no_expiry(),
			creator: m.identity_tokens_created_by({ username: 'felipe' }),
			isNew: false,
		})
		expect(second?.scope).toBe('extractor/staging')
		expect(second?.creator).toBe(m.identity_tokens_created_by_deleted())
		expect(second?.lastUsed).toMatch(/12/)
		// Los vencidos también dicen quién los creó.
		expect(vm.expiredRows[0]?.creator).toBe(
			m.identity_tokens_created_by({ username: 'soporte' }),
		)
	})

	it('el alcance de un token de todos los proyectos es "todos los proyectos"', async () => {
		mocks.listTokens.mockResolvedValue([
			token({ id: 7 as TokenId, scope: { kind: 'allProjects' }, readOnly: true }),
		])
		const vm = await mountViewModel()

		expect(vm.activeRows[0]?.scope).toBe(m.identity_tokens_scope_all())
	})

	it('el token creado se muestra una vez, marcado como nuevo, con su ejemplo', async () => {
		const vm = await mountViewModel()
		const created = token({
			id: 2 as TokenId,
			name: 'extractor-ci',
			scope: { kind: 'project', project: 'extractor', stage: 'staging' },
		})

		vm.openCreate()
		vm.onCreated({ token: created, secret: 'mk_secreto' })

		expect(vm.createOpen).toBe(false)
		expect(vm.created?.secret).toBe('mk_secreto')
		expect(vm.activeRows.find((row) => row.token.id === 2)?.isNew).toBe(true)
		// El recién creado va arriba de la lista; el resto, en el orden del backend.
		expect(vm.activeRows.map((row) => row.token.id)).toEqual([2, 1])
		// La versión mayor de la action, que se mueve en cada release (spec §14): `@v0`
		// mientras no haya 1.0.
		expect(vm.snippet).toContain('- uses: CPU-commits/muelle/action@v0\n')
		expect(vm.snippet).toContain('token: ${{ secrets.MUELLE_TOKEN }}')
		expect(vm.snippet).toContain('app: extractor/staging/<app>')

		vm.dismissCreated()
		expect(vm.created).toBeNull()
		expect(vm.snippet).toBe('')
	})

	it('revocar pide confirmación y después revoca', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel()
		const target = vm.activeRows[0]?.token
		if (!target) throw new Error('falta el token')

		vm.askRevoke(target)
		expect(mocks.revokeToken).not.toHaveBeenCalled()
		await vm.confirmRevoke()

		expect(mocks.revokeToken).toHaveBeenCalledWith(1)
		expect(vm.revoking).toBeNull()
		expect(success).toHaveBeenCalledWith(m.identity_tokens_revoked())
	})

	it('un token ya revocado no es un error', async () => {
		const info = vi.spyOn(toast, 'info')
		mocks.revokeToken.mockRejectedValue(new TokenNotFound())
		const vm = await mountViewModel()
		const target = vm.activeRows[0]?.token
		if (!target) throw new Error('falta el token')

		vm.askRevoke(target)
		await vm.confirmRevoke()

		expect(info).toHaveBeenCalledWith(m.identity_tokens_already_revoked())
	})

	it('un error al revocar queda adentro de la confirmación', async () => {
		mocks.revokeToken.mockRejectedValue(new UnavailableError('caído'))
		const vm = await mountViewModel()
		const target = vm.activeRows[0]?.token
		if (!target) throw new Error('falta el token')

		vm.askRevoke(target)
		await vm.confirmRevoke()

		expect(vm.revoking).toEqual(target)
		expect(vm.revokeError).toBe(m.common_unavailable())
		vm.cancelRevoke()
		expect(vm.revokeError).toBeNull()
	})

	it('sin tokens, el estado vacío', async () => {
		mocks.listTokens.mockResolvedValue([])
		const vm = await mountViewModel()

		expect(vm.isEmpty).toBe(true)
		expect(vm.loadError).toBeNull()
	})
})
