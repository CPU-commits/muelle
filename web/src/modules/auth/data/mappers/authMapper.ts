import type {
	InviteToken,
	LoginCredentials,
	LoginOutcome,
	Me,
	NewAccount,
	TotpCode,
	TotpSetup,
	Username,
} from '../../domain'
import type {
	CodePayload,
	CredentialsPayload,
	MeDto,
	MfaResponseDto,
	SetupPayload,
	TotpSetupDto,
	UsernameResponseDto,
} from '../dtos/authDto'

// El nombre que devuelve el backend ya pasó su validación: se marca como Username sin
// volver a aplicar la regla (si la regla cambia allá, el panel no deja de funcionar).
function asUsername(raw: string): Username {
	return raw as Username
}

export function meFromDto(dto: MeDto): Me {
	return {
		username: asUsername(dto.username),
		totpEnabled: dto.totp_enabled,
		needsTotpSetup: dto.needs_totp_setup,
	}
}

export function loginOutcomeFromDto(dto: MfaResponseDto): LoginOutcome {
	return { mfaRequired: dto.mfa_required }
}

export function usernameFromDto(dto: UsernameResponseDto): Username {
	return asUsername(dto.username)
}

export function totpSetupFromDto(dto: TotpSetupDto): TotpSetup {
	return { secret: dto.secret, otpauthUrl: dto.url }
}

export function credentialsPayload(credentials: LoginCredentials | NewAccount): CredentialsPayload {
	return { username: credentials.username, password: credentials.password }
}

export function setupPayload(token: InviteToken, account: NewAccount): SetupPayload {
	return { token, ...credentialsPayload(account) }
}

export function codePayload(code: TotpCode): CodePayload {
	return { code }
}
