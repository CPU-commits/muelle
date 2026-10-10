import { QueryClient } from '@tanstack/svelte-query'
import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api } from '@/mocks/api'
import {
	authDb,
	MOCK_INVITE_TOKEN,
	MOCK_PASSWORD,
	MOCK_SETUP_TOKEN,
	MOCK_TOTP_CODE,
	MOCK_USERNAME,
} from '@/mocks/db/auth'
import { server } from '@/mocks/server'
import { ContractError } from '@/shared/data/http/errors'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { authUseCases } from '../container'
import { loadSessionState } from '../data'
import {
	AccountLocked,
	AlreadySetUp,
	InvalidCode,
	InvalidCredentials,
	InvalidInvite,
	MfaRequired,
	NoPendingTotp,
	SessionExpired,
	UsernameTaken,
} from '../domain'

// Recorre el módulo entero —caso de uso, repositorio, datasource, cliente HTTP, schema
// Zod y mapper— contra el backend simulado por MSW (src/mocks/handlers/auth.ts).
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => authDb.reset())
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const login = () => authUseCases.login({ username: MOCK_USERNAME, password: MOCK_PASSWORD })

function enableTotpFor(username: string): void {
	const user = authDb.findUser(username)
	if (user) user.totpEnabled = true
}

describe('flujo de sesión del módulo auth', () => {
	it('sin 2FA: login → me', async () => {
		await expect(login()).resolves.toEqual({ mfaRequired: false })

		await expect(authUseCases.getMe()).resolves.toEqual({
			username: MOCK_USERNAME,
			totpEnabled: false,
			needsTotpSetup: false,
		})
	})

	it('con 2FA: login pide el código, me espera, y el código completa la sesión', async () => {
		enableTotpFor(MOCK_USERNAME)

		await expect(login()).resolves.toEqual({ mfaRequired: true })
		await expect(authUseCases.getMe()).rejects.toBeInstanceOf(MfaRequired)
		await expect(authUseCases.completeTotp('000000')).rejects.toBeInstanceOf(InvalidCode)

		await authUseCases.completeTotp(MOCK_TOTP_CODE)

		await expect(authUseCases.getMe()).resolves.toMatchObject({ totpEnabled: true })
	})

	it('cinco códigos malos borran la sesión pendiente', async () => {
		enableTotpFor(MOCK_USERNAME)
		await login()

		for (let attempt = 0; attempt < 5; attempt += 1) {
			await expect(authUseCases.completeTotp('000000')).rejects.toBeInstanceOf(InvalidCode)
		}

		await expect(authUseCases.completeTotp(MOCK_TOTP_CODE)).rejects.toBeInstanceOf(
			SessionExpired,
		)
	})

	it('una contraseña mala es InvalidCredentials y cinco bloquean al usuario', async () => {
		const bad = () => authUseCases.login({ username: MOCK_USERNAME, password: 'mala' })

		for (let attempt = 0; attempt < 5; attempt += 1) {
			await expect(bad()).rejects.toBeInstanceOf(InvalidCredentials)
		}

		await expect(login()).rejects.toBeInstanceOf(AccountLocked)
	})

	it('logout cierra la sesión', async () => {
		await login()

		await authUseCases.logout()

		await expect(authUseCases.getMe()).rejects.toBeInstanceOf(SessionExpired)
	})

	it('setup: crea el primer usuario solo si no hay ninguno', async () => {
		const account = { username: 'admin', password: 'una-contraseña-larga' }
		await expect(
			authUseCases.setup({ token: MOCK_SETUP_TOKEN, ...account }),
		).rejects.toBeInstanceOf(AlreadySetUp)

		authDb.reset({ users: [] })
		await expect(authUseCases.setup({ token: 'otro', ...account })).rejects.toBeInstanceOf(
			InvalidInvite,
		)
		await expect(authUseCases.setup({ token: MOCK_SETUP_TOKEN, ...account })).resolves.toBe(
			'admin',
		)
		await expect(authUseCases.getMe()).resolves.toMatchObject({ username: 'admin' })
	})

	it('invitación: crea la cuenta una sola vez y avisa si el nombre está tomado', async () => {
		await expect(
			authUseCases.acceptInvite({
				token: MOCK_INVITE_TOKEN,
				username: MOCK_USERNAME,
				password: 'clave-de-soporte',
			}),
		).rejects.toBeInstanceOf(UsernameTaken)

		const accept = () =>
			authUseCases.acceptInvite({
				token: MOCK_INVITE_TOKEN,
				username: 'soporte',
				password: 'clave-de-soporte',
			})
		await expect(accept()).resolves.toBe('soporte')
		await expect(accept()).rejects.toBeInstanceOf(InvalidInvite)
	})

	it('un 422 del backend llega como ValidationError con su campo', async () => {
		// El dominio ya valida la contraseña: el handler responde como lo haría el backend
		// con una regla que el panel no conoce.
		server.use(
			http.post(api('/auth/invite/:token'), () =>
				HttpResponse.json(
					{ error: { code: 'invalid', message: 'username: nombre reservado' } },
					{ status: 422 },
				),
			),
		)

		const error = await authUseCases
			.acceptInvite({ token: 'x', username: 'root', password: 'clave-de-soporte' })
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ValidationError)
		expect((error as ValidationError).field).toBe('username')
		expect((error as ValidationError).message).toBe('nombre reservado')
	})

	it('2FA: generar, confirmar, y no se puede confirmar sin generar', async () => {
		await login()
		await expect(authUseCases.enableTotp(MOCK_TOTP_CODE)).rejects.toBeInstanceOf(NoPendingTotp)

		const setup = await authUseCases.beginTotpSetup()
		expect(setup.otpauthUrl).toMatch(/^otpauth:\/\/totp\/muelle:felipe\?secret=/)

		await authUseCases.enableTotp(MOCK_TOTP_CODE)
		await expect(authUseCases.getMe()).resolves.toMatchObject({ totpEnabled: true })
	})

	it('con require_2fa, la sesión de alguien sin 2FA queda en needsTotpSetup', async () => {
		authDb.reset({ require2fa: true })
		await login()

		await expect(authUseCases.getMe()).resolves.toMatchObject({ needsTotpSetup: true })
	})

	it('una respuesta con otra forma es un ContractError, no un undefined en la UI', async () => {
		server.use(http.get(api('/auth/me'), () => HttpResponse.json({ user: 'felipe' })))

		await expect(authUseCases.getMe()).rejects.toBeInstanceOf(ContractError)
	})
})

describe('loadSessionState (los guards de las rutas)', () => {
	const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })

	it('sin sesión, con sesión y esperando el código', async () => {
		await expect(loadSessionState(client())).resolves.toEqual({ status: 'signed_out' })

		await login()
		await expect(loadSessionState(client())).resolves.toMatchObject({
			status: 'signed_in',
			me: { username: MOCK_USERNAME },
		})

		authDb.reset()
		enableTotpFor(MOCK_USERNAME)
		await login()
		await expect(loadSessionState(client())).resolves.toEqual({ status: 'mfa_pending' })
	})

	it('un backend caído se propaga en vez de mandar al login', async () => {
		server.use(http.get(api('/auth/me'), () => HttpResponse.error()))

		await expect(loadSessionState(client())).rejects.toBeInstanceOf(UnavailableError)
	})
})
