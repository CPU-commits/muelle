/** Las invitaciones son de un solo uso y vencen a las 24 h (spec §10.1). */
export const INVITE_TTL_HOURS = 24

/**
 * Una invitación recién creada (`POST /invites`). La URL lleva el token en claro: el
 * backend solo guarda su hash, así que se muestra esta única vez.
 */
export interface Invite {
	readonly url: string
	readonly expiresAt: Date
}

const HOUR_MS = 60 * 60 * 1000

/** Horas que le quedan, redondeadas hacia arriba ("vence en 24 h"); 0 si ya venció. */
export function inviteHoursLeft(invite: Invite, now: Date = new Date()): number {
	const left = invite.expiresAt.getTime() - now.getTime()
	return left <= 0 ? 0 : Math.ceil(left / HOUR_MS)
}
