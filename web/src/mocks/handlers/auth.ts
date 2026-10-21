import { http, HttpResponse } from 'msw'
import { api, apiError, invalid, noContent } from '../api'
import {
	authDb,
	LOCK_MS,
	MAX_BAD_CODES,
	MAX_FAILED_LOGINS,
	MOCK_TOTP_CODE,
	MOCK_TOTP_SECRET,
	type MockUser,
} from '../db/auth'

// `/auth/*` de identity (routes.go), con sus códigos de error. La sesión vive en la db
// del mock en vez de en una cookie: así funciona igual en node (tests) y en el navegador.

const USERNAME = /^[a-z0-9][a-z0-9._-]{2,31}$/

interface CredentialsBody {
	username?: string
	password?: string
	token?: string
	code?: string
}

async function readBody(request: Request): Promise<CredentialsBody> {
	try {
		return (await request.json()) as CredentialsBody
	} catch {
		return {}
	}
}

// La validación de `validate.Username` y `validate.Password`, con sus mensajes.
function validateAccount(body: CredentialsBody) {
	if (!USERNAME.test(body.username ?? '')) {
		return invalid('username', "entre 3 y 32 caracteres: a-z, 0-9, '.', '_' o '-'")
	}
	if ([...(body.password ?? '')].length < 12) {
		return invalid('password', 'debe tener al menos 12 caracteres')
	}
	return undefined
}

function openSession(user: MockUser, mfaPending: boolean): void {
	authDb.state.session = { username: user.username, mfaPending, badCodes: 0 }
	authDb.commit()
}

function createUser(body: CredentialsBody): MockUser {
	const user: MockUser = {
		username: body.username ?? '',
		password: body.password ?? '',
		totpEnabled: false,
		createdAt: new Date().toISOString(),
	}
	authDb.state.users = [...authDb.state.users, user]
	openSession(user, false)
	return user
}

const unauthenticated = () => apiError(401, 'unauthenticated', 'inicia sesión')

export const authHandlers = [
	http.post(api('/auth/login'), async ({ request }) => {
		const state = authDb.state
		if (Date.now() < state.lockedUntil) {
			return apiError(423, 'locked', 'usuario bloqueado temporalmente por intentos fallidos')
		}
		const body = await readBody(request)
		const user = authDb.findUser(body.username ?? '')
		if (!user || user.password !== body.password) {
			state.failedLogins += 1
			if (state.failedLogins >= MAX_FAILED_LOGINS) {
				state.failedLogins = 0
				state.lockedUntil = Date.now() + LOCK_MS
			}
			authDb.commit()
			return apiError(401, 'invalid_credentials', 'usuario o contraseña incorrectos')
		}
		state.failedLogins = 0
		openSession(user, user.totpEnabled)
		return HttpResponse.json({ mfa_required: user.totpEnabled })
	}),

	http.post(api('/auth/totp'), async ({ request }) => {
		const session = authDb.state.session
		if (!session) return unauthenticated()
		if (!session.mfaPending) {
			return apiError(400, 'no_mfa_pending', 'esta sesión no espera un código')
		}
		const body = await readBody(request)
		if (body.code !== MOCK_TOTP_CODE) {
			session.badCodes += 1
			if (session.badCodes >= MAX_BAD_CODES) authDb.state.session = null
			authDb.commit()
			return apiError(401, 'invalid_code', 'código incorrecto')
		}
		session.mfaPending = false
		authDb.commit()
		return HttpResponse.json({ mfa_required: false })
	}),

	http.post(api('/auth/logout'), () => {
		if (!authDb.state.session) return unauthenticated()
		authDb.state.session = null
		authDb.commit()
		return noContent()
	}),

	http.get(api('/auth/me'), () => {
		const session = authDb.state.session
		if (!session) return unauthenticated()
		if (session.mfaPending) return apiError(401, 'mfa_required', 'falta el código de 2FA')
		const user = authDb.findUser(session.username)
		if (!user) return unauthenticated()
		return HttpResponse.json({
			username: user.username,
			totp_enabled: user.totpEnabled,
			needs_totp_setup: authDb.needsTotpSetup(user),
		})
	}),

	http.post(api('/auth/setup'), async ({ request }) => {
		const body = await readBody(request)
		if (authDb.state.users.length > 0) {
			return apiError(409, 'already_set_up', 'muelle ya tiene usuarios; usa una invitación')
		}
		const problem = validateAccount(body)
		if (problem) return problem
		if (body.token !== authDb.state.setupToken) {
			return apiError(400, 'invalid_invite', 'la invitación no existe, ya se usó o venció')
		}
		const user = createUser(body)
		return HttpResponse.json({ username: user.username }, { status: 201 })
	}),

	http.post(api('/auth/invite/:token'), async ({ params, request }) => {
		const token = String(params.token)
		const body = await readBody(request)
		const problem = validateAccount(body)
		if (problem) return problem
		if (!authDb.state.invites.includes(token)) {
			return apiError(400, 'invalid_invite', 'la invitación no existe, ya se usó o venció')
		}
		if (authDb.findUser(body.username ?? '')) return apiError(409, 'conflict', 'ya existe')
		authDb.state.invites = authDb.state.invites.filter((invite) => invite !== token)
		const user = createUser(body)
		return HttpResponse.json({ username: user.username }, { status: 201 })
	}),

	http.post(api('/auth/totp/setup'), () => {
		const user = authDb.currentUser()
		if (!user) return unauthenticated()
		authDb.state.pendingTotpFor = user.username
		authDb.commit()
		return HttpResponse.json({
			secret: MOCK_TOTP_SECRET,
			url: `otpauth://totp/muelle:${user.username}?secret=${MOCK_TOTP_SECRET}&issuer=muelle`,
		})
	}),

	http.post(api('/auth/totp/enable'), async ({ request }) => {
		const user = authDb.currentUser()
		if (!user) return unauthenticated()
		if (authDb.state.pendingTotpFor !== user.username) {
			return apiError(409, 'no_pending_totp', 'primero inicia la configuración de 2FA')
		}
		const body = await readBody(request)
		if (body.code !== MOCK_TOTP_CODE) return apiError(401, 'invalid_code', 'código incorrecto')
		user.totpEnabled = true
		authDb.state.pendingTotpFor = null
		authDb.commit()
		return noContent()
	}),
]
