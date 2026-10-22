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

const tokenFields = z.object({
	id: z.number().int().positive(),
	name: z.string(),
	/** Falta solo en un token de todos los proyectos (contrato §8.1). */
	project: z.string().min(1).optional(),
	/** `omitempty`: falta si el token es de todo el proyecto o de todos los proyectos. */
	stage: z.string().optional(),
	/** Siempre vienen: el panel y el backend van en el mismo binario. */
	read_only: z.boolean(),
	all_projects: z.boolean(),
	/** El username de quien lo creó, o null si ese usuario se borró (el token sigue). */
	created_by: z.string().nullable(),
	created_at: timestamp,
	last_used_at: timestamp.nullable(),
	expires_at: timestamp.nullable(),
})

/**
 * El alcance que permite la base (migración 0007): un proyecto, o todos los proyectos, que
 * no lleva stage y siempre es de solo lectura.
 */
function validScope(token: z.infer<typeof tokenFields>): boolean {
	if (!token.all_projects) return token.project !== undefined
	return token.project === undefined && token.stage === undefined && token.read_only
}

const SCOPE_ERROR = 'un token es de un proyecto o de todos los proyectos (de solo lectura)'

/** Un elemento de `GET /tokens`. */
export const tokenDtoSchema = tokenFields.refine(validScope, SCOPE_ERROR)

/** `POST /tokens` (201): el token más su valor en claro, que se muestra una vez. */
export const createdTokenDtoSchema = tokenFields
	.extend({ token: z.string().min(1) })
	.refine(validScope, SCOPE_ERROR)

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
 * `expires_in_days` 0 es que no vence. Con `all_projects`, `project` y `stage` van vacíos y
 * `read_only` en true (contrato §8.1).
 */
export interface CreateTokenPayload {
	name: string
	project: string
	stage: string
	read_only: boolean
	all_projects: boolean
	expires_in_days: number
}
