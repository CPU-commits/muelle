import { http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent, notFound } from '../api'
import { adminDb } from '../db/admin'
import { authDb } from '../db/auth'
import { identityDb, type MockToken } from '../db/identity'
import { requireSession } from '../session'

// `/users`, `/invites` y `/tokens` de identity (admin_handlers.go), con sus códigos de
// error. Los usuarios y las invitaciones son los de `authDb`, así una invitación creada
// acá se puede aceptar en /invite/<token>. Cada cambio deja su evento en la auditoría.

const INVITE_TTL_MS = 24 * 60 * 60 * 1000
const MAX_TOKEN_NAME = 64
const MAX_TOKEN_DAYS = 3650
const INVISIBLE = /[\p{Cc}\p{Cf}]/u

function randomToken(prefix: string): string {
	const bytes = crypto.getRandomValues(new Uint8Array(32))
	const base64 = btoa(String.fromCharCode(...bytes))
	return prefix + base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function tokenRes(token: MockToken) {
	return {
		id: token.id,
		name: token.name,
		project: token.project,
		// `omitempty` en el backend: sin stage, la clave no viene.
		...(token.stage ? { stage: token.stage } : {}),
		created_by: token.createdBy,
		created_at: token.createdAt,
		last_used_at: token.lastUsedAt,
		expires_at: token.expiresAt,
	}
}

interface CreateTokenBody {
	name?: unknown
	project?: unknown
	stage?: unknown
	expires_in_days?: unknown
}

const CREATE_TOKEN_FIELDS = new Set(['name', 'project', 'stage', 'expires_in_days'])

// `validate.Name("name", …, 64)` y el rango de `expires_in_days` de CreateToken.
function validateToken(body: CreateTokenBody) {
	const name = typeof body.name === 'string' ? body.name : ''
	if (name.trim() === '') return invalid('name', 'es obligatorio')
	if ([...name].length > MAX_TOKEN_NAME) {
		return invalid('name', `no puede superar ${MAX_TOKEN_NAME} caracteres`)
	}
	if (INVISIBLE.test(name)) {
		return invalid('name', 'no puede contener caracteres de control ni invisibles')
	}
	const days = body.expires_in_days ?? 0
	if (typeof days !== 'number' || !Number.isInteger(days) || days < 0 || days > MAX_TOKEN_DAYS) {
		return invalid('expires_in_days', `debe estar entre 0 y ${MAX_TOKEN_DAYS}`)
	}
	if (typeof body.project !== 'string' || body.project === '') return notFound()
	return undefined
}

export const identityHandlers: RequestHandler[] = [
	http.get(api('/users'), () => {
		const denied = requireSession()
		if (denied) return denied
		return HttpResponse.json(
			authDb.state.users.map((user) => ({
				username: user.username,
				totp_enabled: user.totpEnabled,
				created_at: user.createdAt,
			})),
		)
	}),

	http.delete(api('/users/:username'), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const username = String(params.username)
		const target = authDb.findUser(username)
		if (!target) return notFound()
		if (username === identityDb.sessionUsername()) {
			return apiError(409, 'cannot_delete_self', 'no puedes borrar tu propio usuario')
		}
		authDb.state.users = authDb.state.users.filter((user) => user.username !== username)
		authDb.commit()
		identityDb.forgetCreator(username)
		adminDb.record('user.delete', `user:${username}`)
		return noContent()
	}),

	http.delete(api('/users/:username/totp'), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const username = String(params.username)
		const target = authDb.findUser(username)
		if (!target) return notFound()
		target.totpEnabled = false
		if (authDb.state.pendingTotpFor === username) authDb.state.pendingTotpFor = null
		// Cierra todas sus sesiones: en el mock hay una sola, la del navegador.
		if (authDb.state.session?.username === username) authDb.state.session = null
		authDb.commit()
		adminDb.record('user.totp_reset', `user:${username}`)
		return noContent()
	}),

	http.post(api('/invites'), ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		const token = randomToken('')
		authDb.addInvite(token)
		adminDb.record('invite.create', `user:${identityDb.sessionUsername() ?? ''}`)
		return HttpResponse.json(
			{
				url: `${new URL(request.url).origin}/invite/${token}`,
				expires_at: new Date(Date.now() + INVITE_TTL_MS).toISOString(),
			},
			{ status: 201 },
		)
	}),

	http.get(api('/tokens'), () => {
		const denied = requireSession()
		if (denied) return denied
		return HttpResponse.json(identityDb.state.tokens.map(tokenRes))
	}),

	http.post(api('/tokens'), async ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		let body: CreateTokenBody
		try {
			body = (await request.json()) as CreateTokenBody
		} catch {
			return apiError(400, 'invalid_json', 'el cuerpo no es JSON válido')
		}
		// json/v2 estricto: un campo desconocido es un error.
		if (Object.keys(body).some((key) => !CREATE_TOKEN_FIELDS.has(key))) {
			return apiError(400, 'invalid_json', 'campo desconocido')
		}
		const problem = validateToken(body)
		if (problem) return problem
		// El alcance (proyecto y stage) lo valida catalog; este mock no tiene sus datos y
		// acepta cualquier slug.
		const days = typeof body.expires_in_days === 'number' ? body.expires_in_days : 0
		const now = new Date()
		const expires = new Date(now)
		expires.setDate(expires.getDate() + days)
		const token: MockToken = {
			id: identityDb.state.nextTokenId,
			name: String(body.name),
			project: String(body.project),
			stage: typeof body.stage === 'string' ? body.stage : '',
			createdBy: identityDb.sessionUsername(),
			createdAt: now.toISOString(),
			lastUsedAt: null,
			expiresAt: days > 0 ? expires.toISOString() : null,
		}
		identityDb.state.tokens = [...identityDb.state.tokens, token]
		identityDb.state.nextTokenId += 1
		identityDb.commit()
		adminDb.record('token.create', `token:${token.id}`, {
			project: token.project,
			stage: token.stage,
		})
		return HttpResponse.json({ ...tokenRes(token), token: randomToken('mk_') }, { status: 201 })
	}),

	http.delete(api('/tokens/:id'), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const id = Number(params.id)
		if (!Number.isInteger(id)) return invalid('id', 'debe ser un número')
		if (!identityDb.state.tokens.some((token) => token.id === id)) return notFound()
		identityDb.state.tokens = identityDb.state.tokens.filter((token) => token.id !== id)
		identityDb.commit()
		adminDb.record('token.delete', `token:${id}`)
		return noContent()
	}),
]
