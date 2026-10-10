import type { Brand } from '@/shared/domain'
import { MissingInviteToken } from '../errors/authErrors'

// El token de un link de un solo uso: el de setup (`/setup?token=…`, lo imprime
// `muelle serve`) o el de una invitación (`/invite/<token>`). El panel no lo interpreta:
// solo exige que esté.
export type InviteToken = Brand<string, 'InviteToken'>

export function parseInviteToken(raw: string | null | undefined): InviteToken {
	const token = (raw ?? '').trim()
	if (token.length === 0) throw new MissingInviteToken()
	return token as InviteToken
}
