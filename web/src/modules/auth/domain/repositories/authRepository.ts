import type { Me } from '../entities/me'
import type { LoginOutcome } from '../entities/session'
import type { TotpSetup } from '../entities/totpSetup'
import type { LoginCredentials, NewAccount } from '../value-objects/credentials'
import type { InviteToken } from '../value-objects/inviteToken'
import type { TotpCode } from '../value-objects/totpCode'
import type { Username } from '../value-objects/username'

// El puerto de la sesión. La implementación HTTP traduce los `error.code` del backend a
// los errores de `errors/authErrors.ts`; ningún método devuelve un status.
export interface AuthRepository {
	me(signal?: AbortSignal): Promise<Me>
	/** Abre una sesión, completa o pendiente del código de 2FA. */
	login(credentials: LoginCredentials): Promise<LoginOutcome>
	/** Cambia la sesión pendiente por una completa. */
	completeTotp(code: TotpCode): Promise<void>
	logout(): Promise<void>
	/** Crea el primer usuario con el token del link de setup y abre su sesión. */
	setup(token: InviteToken, account: NewAccount): Promise<Username>
	/** Crea un usuario con una invitación y abre su sesión. */
	acceptInvite(token: InviteToken, account: NewAccount): Promise<Username>
	/** Genera un secreto TOTP pendiente para el usuario de la sesión. */
	beginTotpSetup(): Promise<TotpSetup>
	/** Confirma el secreto pendiente con un código y lo activa. */
	enableTotp(code: TotpCode): Promise<void>
}

export interface AuthDeps {
	readonly auth: AuthRepository
}
