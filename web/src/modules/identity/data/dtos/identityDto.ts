import { z } from 'zod'

// Las formas exactas de identity (src/internal/identity/infra/in/http/res.go y
// form/forms.go) y lo poco que este módulo lee de catalog (proyectos y stages).

// El backend manda UTC con milisegundos (`2026-10-10T19:36:53.770Z`); una versión anterior
// mandaba RFC 3339 completo con el offset local. Se aceptan las dos.
const timestamp = z.iso.datetime({ offset: true })

/** Un elemento de `GET /users`. */
export const userDtoSchema = z.object({
	username: z.string().min(1),
	totp_enabled: z.boolean(),
	created_at: timestamp,
})

/** `POST /invites` (201). */
export const inviteDtoSchema = z.object({
	url: z.string().min(1),
	expires_at: timestamp,
})

/** Un elemento de `GET /tokens`. */
export const tokenDtoSchema = z.object({
	id: z.number().int().positive(),
	name: z.string(),
	project: z.string().min(1),
	/** `omitempty`: falta si el token es de todo el proyecto. */
	stage: z.string().optional(),
	/**
	 * El username de quien lo creó, o null si ese usuario se borró (el token sigue). Falta
	 * en un backend anterior a este campo.
	 */
	created_by: z.string().nullable().optional(),
	created_at: timestamp,
	last_used_at: timestamp.nullable(),
	expires_at: timestamp.nullable(),
})

/** `POST /tokens` (201): el token más su valor en claro, que se muestra una vez. */
export const createdTokenDtoSchema = tokenDtoSchema.extend({
	token: z.string().min(1),
})

/** Un elemento de `GET /projects` (catalog). Solo se lee lo que hace falta. */
export const projectRefDtoSchema = z.object({
	slug: z.string().min(1),
	name: z.string(),
})

/** Un elemento de `GET /projects/{p}/stages` (catalog). */
export const stageRefDtoSchema = z.object({
	slug: z.string().min(1),
})

export type UserDto = z.infer<typeof userDtoSchema>
export type InviteDto = z.infer<typeof inviteDtoSchema>
export type TokenDto = z.infer<typeof tokenDtoSchema>
export type CreatedTokenDto = z.infer<typeof createdTokenDtoSchema>
export type ProjectRefDto = z.infer<typeof projectRefDtoSchema>
export type StageRefDto = z.infer<typeof stageRefDtoSchema>

/**
 * Cuerpo de `POST /tokens` (`form.CreateToken`). `stage` vacío es todo el proyecto y
 * `expires_in_days` 0 es que no vence.
 */
export interface CreateTokenPayload {
	name: string
	project: string
	stage: string
	expires_in_days: number
}
