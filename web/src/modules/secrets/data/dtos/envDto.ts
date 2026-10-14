import { z } from 'zod'

// Las formas exactas de secrets (src/internal/secrets/infra/in/http/res.go y form/env.go).

/**
 * Una variable de `GET …/env`. `value` es null si y solo si es secreta. `updated_at` es un
 * `time.Time` de Go: RFC 3339 con la zona del servidor y hasta nanosegundos.
 */
export const envVarDtoSchema = z
	.object({
		key: z.string().min(1),
		value: z.string().nullable(),
		secret: z.boolean(),
		updated_at: z.iso.datetime({ offset: true }),
	})
	.refine((dto) => dto.secret === (dto.value === null), {
		message: 'value debe ser null si y solo si la variable es secreta',
		path: ['value'],
	})

/** `GET …/stages/{s}/env` y `GET …/apps/{a}/env`. */
export const envDtoSchema = z.object({
	vars: z.array(envVarDtoSchema),
})

export type EnvVarDto = z.infer<typeof envVarDtoSchema>
export type EnvDto = z.infer<typeof envDtoSchema>

/** Cuerpo de `PUT …/env`. `secret` aplica a todas; el panel siempre lo manda. */
export interface ReplaceEnvPayload {
	vars: Record<string, string>
	secret: boolean
}

/**
 * Cuerpo de `PATCH …/env`. `secret` aplica a las de `set`. El backend rechaza campos
 * desconocidos (json/v2 estricto): solo estos tres.
 */
export interface PatchEnvPayload {
	set?: Record<string, string>
	unset?: string[]
	secret?: boolean
}
