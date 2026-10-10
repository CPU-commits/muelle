import { z } from 'zod'

// Las formas exactas de identity (src/internal/identity/infra/in/http/res.go y form/forms.go).

/** `GET /auth/me`. */
export const meDtoSchema = z.object({
	username: z.string().min(1),
	totp_enabled: z.boolean(),
	needs_totp_setup: z.boolean(),
})

/** `POST /auth/login` y `POST /auth/totp`. */
export const mfaResponseDtoSchema = z.object({
	mfa_required: z.boolean(),
})

/** `POST /auth/setup` y `POST /auth/invite/{token}` (201). */
export const usernameResponseDtoSchema = z.object({
	username: z.string().min(1),
})

/** `POST /auth/totp/setup`. */
export const totpSetupDtoSchema = z.object({
	secret: z.string().min(1),
	url: z.string().startsWith('otpauth://'),
})

export type MeDto = z.infer<typeof meDtoSchema>
export type MfaResponseDto = z.infer<typeof mfaResponseDtoSchema>
export type UsernameResponseDto = z.infer<typeof usernameResponseDtoSchema>
export type TotpSetupDto = z.infer<typeof totpSetupDtoSchema>

/** Cuerpo de `POST /auth/login` y `POST /auth/invite/{token}`. */
export interface CredentialsPayload {
	username: string
	password: string
}

/** Cuerpo de `POST /auth/setup`. */
export interface SetupPayload extends CredentialsPayload {
	token: string
}

/** Cuerpo de `POST /auth/totp` y `POST /auth/totp/enable`. */
export interface CodePayload {
	code: string
}
