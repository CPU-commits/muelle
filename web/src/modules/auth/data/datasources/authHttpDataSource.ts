import { http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import {
	type CodePayload,
	type CredentialsPayload,
	type MeDto,
	meDtoSchema,
	type MfaResponseDto,
	mfaResponseDtoSchema,
	type SetupPayload,
	type TotpSetupDto,
	totpSetupDtoSchema,
	type UsernameResponseDto,
	usernameResponseDtoSchema,
} from '../dtos/authDto'

// En los endpoints de acceso un 401 es una respuesta que la pantalla maneja (contraseña
// mala, código malo, "todavía no hay sesión"), no una sesión vencida: no redirigen. El
// único que sí redirige es `totp/setup`, que solo responde 401 si la sesión se cayó.
const ACCESS = { redirectOn401: false } as const

export const authHttpDataSource = {
	async me(signal?: AbortSignal): Promise<MeDto> {
		const payload = await http.get<unknown>('/auth/me', {
			...ACCESS,
			...(signal ? { signal } : {}),
		})
		return parseOrThrow(meDtoSchema, payload, 'auth.me')
	},

	async login(body: CredentialsPayload): Promise<MfaResponseDto> {
		const payload = await http.post<unknown>('/auth/login', { ...ACCESS, body })
		return parseOrThrow(mfaResponseDtoSchema, payload, 'auth.login')
	},

	async completeTotp(body: CodePayload): Promise<MfaResponseDto> {
		const payload = await http.post<unknown>('/auth/totp', { ...ACCESS, body })
		return parseOrThrow(mfaResponseDtoSchema, payload, 'auth.totp')
	},

	async logout(): Promise<void> {
		await http.post<unknown>('/auth/logout', ACCESS)
	},

	async setup(body: SetupPayload): Promise<UsernameResponseDto> {
		const payload = await http.post<unknown>('/auth/setup', { ...ACCESS, body })
		return parseOrThrow(usernameResponseDtoSchema, payload, 'auth.setup')
	},

	async acceptInvite(token: string, body: CredentialsPayload): Promise<UsernameResponseDto> {
		const payload = await http.post<unknown>(`/auth/invite/${encodeURIComponent(token)}`, {
			...ACCESS,
			body,
		})
		return parseOrThrow(usernameResponseDtoSchema, payload, 'auth.invite')
	},

	async beginTotpSetup(): Promise<TotpSetupDto> {
		const payload = await http.post<unknown>('/auth/totp/setup')
		return parseOrThrow(totpSetupDtoSchema, payload, 'auth.totp.setup')
	},

	async enableTotp(body: CodePayload): Promise<void> {
		await http.post<unknown>('/auth/totp/enable', { ...ACCESS, body })
	},
}
