import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, apiError, invalid } from '@/mocks/api'
import { adminDb } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { identityDb } from '@/mocks/db/identity'
import { server } from '@/mocks/server'
import { ApiError } from '@/shared/data/http/errors'
import { UnauthorizedError, ValidationError } from '@/shared/domain'
import { identityUseCases } from '../container'
import { identityHttpDataSource } from '../data/datasources/identityHttpDataSource'
import {
	CannotDeleteSelf,
	type TokenId,
	TokenNotFound,
	TokenScopeNotFound,
	UserNotFound,
} from '../domain'

// Recorre el módulo entero —caso de uso, repositorio, datasource, cliente HTTP, schema
// Zod y mapper— contra el backend simulado por MSW (src/mocks/handlers/identity.ts).
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset()
	identityDb.reset()
	adminDb.reset()
	signIn()
})
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

function signIn(): void {
	authDb.state.session = { username: MOCK_USERNAME, mfaPending: false, badCodes: 0 }
	authDb.commit()
}

function addUser(username: string, totpEnabled = true): void {
	authDb.state.users = [
		...authDb.state.users,
		{ username, password: 'clave-de-soporte', totpEnabled, createdAt: '2026-10-09T15:00:00Z' },
	]
}

const lastAudit = () => adminDb.state.audit.at(-1)

describe('usuarios', () => {
	it('lista las cuentas de authDb', async () => {
		addUser('soporte')

		await expect(identityUseCases.listUsers()).resolves.toEqual([
			{ username: 'felipe', totpEnabled: false, createdAt: new Date('2026-10-09T12:00:00Z') },
			{ username: 'soporte', totpEnabled: true, createdAt: new Date('2026-10-09T15:00:00Z') },
		])
	})

	it('borra a otro usuario: sus tokens siguen, sin creador, y queda auditado', async () => {
		addUser('soporte')
		identityDb.state.tokens = identityDb.state.tokens.map((token) =>
			token.id === 1 ? { ...token, createdBy: 'soporte' } : token,
		)

		await identityUseCases.deleteUser({ username: 'soporte', currentUsername: 'felipe' })

		const users = await identityUseCases.listUsers()
		expect(users.map((user) => user.username)).toEqual(['felipe'])
		const tokens = await identityUseCases.listTokens()
		expect(tokens.find((token) => token.id === 1)?.createdBy).toEqual({ kind: 'deleted' })
		expect(lastAudit()).toMatchObject({ action: 'user.delete', target: 'user:soporte' })
	})

	it('el backend no deja borrarse a sí mismo (cannot_delete_self)', async () => {
		await expect(
			identityUseCases.deleteUser({ username: MOCK_USERNAME, currentUsername: null }),
		).rejects.toBeInstanceOf(CannotDeleteSelf)
	})

	it('un usuario que ya no existe es UserNotFound', async () => {
		await expect(
			identityUseCases.deleteUser({ username: 'nadie', currentUsername: 'felipe' }),
		).rejects.toBeInstanceOf(UserNotFound)
		await expect(identityUseCases.resetUserTotp('nadie')).rejects.toBeInstanceOf(UserNotFound)
	})

	it('quitar el 2FA de otro lo deja sin 2FA', async () => {
		addUser('soporte', true)

		await identityUseCases.resetUserTotp('soporte')

		const users = await identityUseCases.listUsers()
		expect(users.find((user) => user.username === 'soporte')?.totpEnabled).toBe(false)
		expect(lastAudit()).toMatchObject({ action: 'user.totp_reset' })
	})

	it('sin sesión, todo responde UnauthorizedError', async () => {
		authDb.state.session = null

		await expect(identityUseCases.listUsers()).rejects.toBeInstanceOf(UnauthorizedError)
	})
})

describe('invitaciones', () => {
	it('crea una invitación de 24 h que después se puede aceptar', async () => {
		const before = Date.now()

		const invite = await identityUseCases.createInvite()

		const token = invite.url.split('/invite/')[1] ?? ''
		expect(invite.url).toMatch(/^http:\/\/localhost\/invite\/[\w-]{43}$/)
		expect(authDb.state.invites).toContain(token)
		const hours = (invite.expiresAt.getTime() - before) / 3_600_000
		expect(hours).toBeGreaterThan(23.9)
		expect(hours).toBeLessThanOrEqual(24.01)
	})
})

describe('tokens', () => {
	it('lista con alcance, creador, último uso y vencimiento', async () => {
		const tokens = await identityUseCases.listTokens()

		expect(tokens.map((token) => token.name)).toEqual([
			'github-actions',
			'extractor-ci',
			'extractor-prod-ci',
			'aumi-viejo',
			'claude-auditfy',
			'claude-debug',
		])
		expect(tokens[0]).toMatchObject({
			scope: { kind: 'project', project: 'auditfy', stage: null },
			readOnly: false,
			createdBy: { kind: 'user', username: 'felipe' },
			expiresAt: null,
		})
		expect(tokens[0]?.lastUsedAt).toBeInstanceOf(Date)
		expect(tokens[2]?.createdBy).toEqual({ kind: 'deleted' })
		// Los de Claude Code: uno de solo lectura de un stage y uno de todos los proyectos.
		expect(tokens[4]).toMatchObject({
			scope: { kind: 'project', project: 'auditfy', stage: 'staging' },
			readOnly: true,
		})
		expect(tokens[5]).toMatchObject({ scope: { kind: 'allProjects' }, readOnly: true })
	})

	it('crea un token de todos los proyectos: de solo lectura, sin proyecto', async () => {
		const created = await identityUseCases.createToken({
			name: 'claude-todo',
			project: '*',
			stage: '',
			// "*" lo fuerza: no hay un token de escritura de todos los proyectos.
			readOnly: false,
			expiresOn: '',
		})

		expect(created.token).toMatchObject({
			name: 'claude-todo',
			scope: { kind: 'allProjects' },
			readOnly: true,
		})
		const tokens = await identityUseCases.listTokens()
		expect(tokens.at(-1)).toEqual(created.token)
		expect(lastAudit()).toMatchObject({
			action: 'token.create',
			details: { all_projects: true, read_only: true },
		})
	})

	it('el backend rechaza un token de todos los proyectos con proyecto, o de escritura', async () => {
		const body = {
			name: 'claude',
			project: '',
			stage: '',
			read_only: true,
			all_projects: true,
			expires_in_days: 0,
		}
		const rejection = (payload: typeof body) =>
			identityHttpDataSource.createToken(payload).catch((error: unknown) => error)

		const withProject = await rejection({ ...body, project: 'auditfy' })
		expect(withProject).toBeInstanceOf(ApiError)
		expect(withProject).toMatchObject({ status: 422, code: 'invalid' })
		expect((withProject as ApiError).message).toMatch(/^project: /)
		expect(await rejection({ ...body, stage: 'staging' })).toMatchObject({ status: 422 })

		const writable = await rejection({ ...body, read_only: false })
		expect(writable).toMatchObject({ status: 422, code: 'invalid' })
		expect((writable as ApiError).message).toMatch(/^read_only: /)
		expect(identityDb.state.tokens).toHaveLength(6)
	})

	it('un tipo equivocado en read_only es invalid_json, como en json/v2', async () => {
		const error = await identityHttpDataSource
			.createToken({
				name: 'ci',
				project: 'auditfy',
				stage: '',
				read_only: 'sí' as unknown as boolean,
				all_projects: false,
				expires_in_days: 0,
			})
			.catch((caught: unknown) => caught)

		expect(error).toMatchObject({ status: 400, code: 'invalid_json' })
	})

	it('crea un token: el valor en claro sale una vez y el token aparece en la lista', async () => {
		const created = await identityUseCases.createToken({
			name: 'github-actions-aumi',
			project: 'aumi-agents',
			stage: 'staging',
			readOnly: false,
			expiresOn: '',
		})

		expect(created.secret).toMatch(/^mk_[\w-]{43}$/)
		expect(created.token).toMatchObject({
			name: 'github-actions-aumi',
			scope: { kind: 'project', project: 'aumi-agents', stage: 'staging' },
			readOnly: false,
			createdBy: { kind: 'user', username: 'felipe' },
			lastUsedAt: null,
			expiresAt: null,
		})
		const tokens = await identityUseCases.listTokens()
		expect(tokens.at(-1)?.id).toBe(created.token.id)
		expect(JSON.stringify(adminDb.state.audit)).not.toContain(created.secret)
	})

	it('crea un token de solo lectura de un stage, y la lista lo dice', async () => {
		const created = await identityUseCases.createToken({
			name: 'claude-staging',
			project: 'aumi-agents',
			stage: 'staging',
			readOnly: true,
			expiresOn: '',
		})

		expect(created.token).toMatchObject({
			scope: { kind: 'project', project: 'aumi-agents', stage: 'staging' },
			readOnly: true,
		})
		const tokens = await identityUseCases.listTokens()
		expect(tokens.at(-1)).toEqual(created.token)
		expect(lastAudit()).toMatchObject({
			action: 'token.create',
			details: { project: 'aumi-agents', read_only: true, stage: 'staging' },
		})
	})

	it('un 422 del backend llega al campo del formulario', async () => {
		server.use(
			http.post(api('/tokens'), () =>
				invalid('expires_in_days', 'debe estar entre 0 y 3650'),
			),
		)

		const error = await identityUseCases
			.createToken({
				name: 'ci',
				project: 'auditfy',
				stage: '',
				readOnly: false,
				expiresOn: '',
			})
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ValidationError)
		expect((error as ValidationError).field).toBe('expiresOn')
		expect((error as ValidationError).message).toBe('debe estar entre 0 y 3650')
	})

	it('un proyecto o stage que se borró es TokenScopeNotFound', async () => {
		server.use(http.post(api('/tokens'), () => apiError(404, 'not_found', 'no existe')))

		await expect(
			identityUseCases.createToken({
				name: 'ci',
				project: 'x',
				stage: '',
				readOnly: false,
				expiresOn: '',
			}),
		).rejects.toBeInstanceOf(TokenScopeNotFound)
	})

	it('revoca, y revocar otra vez es TokenNotFound', async () => {
		await identityUseCases.revokeToken(2 as TokenId)

		const tokens = await identityUseCases.listTokens()
		expect(tokens.map((token) => token.id)).not.toContain(2)
		await expect(identityUseCases.revokeToken(2 as TokenId)).rejects.toBeInstanceOf(
			TokenNotFound,
		)
		expect(lastAudit()).toMatchObject({ action: 'token.delete', target: 'token:2' })
	})

	it('un token sin created_by no es del contrato (ContractError)', async () => {
		server.use(
			http.get(api('/tokens'), () =>
				HttpResponse.json([
					{
						id: 1,
						name: 'ci',
						project: 'auditfy',
						read_only: false,
						all_projects: false,
						created_at: '2026-10-10T16:36:53.123456-03:00',
						last_used_at: null,
						expires_at: null,
					},
				]),
			),
		)

		// El backend lo manda siempre (null si se borró el usuario que lo creó).
		await expect(identityUseCases.listTokens()).rejects.toMatchObject({
			name: 'ContractError',
		})
	})
})

describe('alcance de un token (proyectos y stages de catalog)', () => {
	// Los handlers de /projects son de catalog: acá se simulan las respuestas del backend.
	beforeEach(() => {
		server.use(
			http.get(api('/projects'), () =>
				HttpResponse.json([
					{ slug: 'auditfy', name: 'Auditfy', created_at: '2026-10-01T12:00:00.000Z' },
				]),
			),
			http.get(api('/projects/:p/stages'), ({ params }) =>
				params.p === 'auditfy'
					? HttpResponse.json([
							{
								project: 'auditfy',
								slug: 'staging',
								created_at: '2026-10-01T12:00:00Z',
							},
						])
					: apiError(404, 'not_found', 'no existe'),
			),
		)
	})

	it('lee los proyectos y los stages de uno', async () => {
		await expect(identityUseCases.listScopeProjects()).resolves.toEqual([
			{ slug: 'auditfy', name: 'Auditfy' },
		])
		await expect(identityUseCases.listScopeStages('auditfy')).resolves.toEqual([
			{ slug: 'staging' },
		])
		await expect(identityUseCases.listScopeStages('otro')).rejects.toBeInstanceOf(
			TokenScopeNotFound,
		)
	})
})
