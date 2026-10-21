import { z } from 'zod'

// Las formas exactas del catálogo (src/internal/catalog/app/dto/dto.go). Las fechas son
// RFC 3339 (`time.Time` de Go).

/** `GET /projects`, `POST /projects`, `PATCH /projects/{p}`. */
export const projectDtoSchema = z.object({
	slug: z.string().min(1),
	name: z.string(),
	created_at: z.string(),
})

/** `GET /projects/{p}/stages`, `POST …/stages`. */
export const stageDtoSchema = z.object({
	project: z.string().min(1),
	slug: z.string().min(1),
	created_at: z.string(),
})

// El spec que guarda el backend ya viene normalizado (`vo.AppSpec.JSON()`): réplicas,
// command, volumes y healthcheck siempre están; los tiempos del healthcheck y los campos
// de resources se omiten cuando son 0. Igual se aceptan ausentes por si se relaja.
const healthcheckDtoSchema = z.object({
	type: z.enum(['http', 'bun-http', 'command', 'none']),
	path: z.string().optional(),
	command: z.array(z.string()).optional(),
	interval_s: z.number().optional(),
	timeout_s: z.number().optional(),
	retries: z.number().optional(),
	start_period_s: z.number().optional(),
})

const volumeDtoSchema = z.object({ name: z.string(), mount: z.string() })

const resourcesDtoSchema = z.object({
	memory_mb: z.number().optional(),
	cpus: z.number().optional(),
})

export const specDtoSchema = z.object({
	image: z.string(),
	port: z.number(),
	replicas: z.number().nullish(),
	command: z.array(z.string()).nullish(),
	healthcheck: healthcheckDtoSchema.nullish(),
	volumes: z.array(volumeDtoSchema).nullish(),
	resources: resourcesDtoSchema.nullish(),
})

/**
 * Un servicio de la app y los puertos que declara (contrato §2.12, Fase 2): el `port` del
 * spec en una de formulario, los de `expose` en una compose. Solo sugerencias para el
 * destino de un dominio: no se publican.
 */
const appServiceDtoSchema = z.object({
	name: z.string().min(1),
	ports: z.array(z.number().int().min(1).max(65535)),
})

const appBaseDtoSchema = z.object({
	project: z.string().min(1),
	stage: z.string().min(1),
	slug: z.string().min(1),
	deploy_timeout_s: z.number(),
	config_changed_at: z.string(),
	last_deployed_at: z.string().nullable(),
	pending_changes: z.boolean(),
	created_at: z.string(),
	// Siempre está, ordenado por nombre (contrato §2.12).
	services: z.array(appServiceDtoSchema),
})

/**
 * `GET …/apps`, `GET/POST/PATCH …/apps/{a}`. Las de formulario traen `spec` y las compose
 * `compose_yaml` (el otro se omite).
 */
export const appDtoSchema = z.discriminatedUnion('kind', [
	appBaseDtoSchema.extend({ kind: z.literal('service'), spec: specDtoSchema }),
	appBaseDtoSchema.extend({ kind: z.literal('compose'), compose_yaml: z.string() }),
])

export type ProjectDto = z.infer<typeof projectDtoSchema>
export type StageDto = z.infer<typeof stageDtoSchema>
export type SpecDto = z.infer<typeof specDtoSchema>
export type HealthcheckDto = z.infer<typeof healthcheckDtoSchema>
export type AppDto = z.infer<typeof appDtoSchema>

// Los cuerpos. El backend lee con json/v2 estricto: un campo de más es un 400.

/** `POST /projects`. */
export interface CreateProjectPayload {
	slug: string
	name: string
}

/** `PATCH /projects/{p}`. */
export interface RenameProjectPayload {
	name: string
}

/** `POST /projects/{p}/stages`. */
export interface CreateStagePayload {
	slug: string
}

export interface HealthcheckPayload {
	type: 'http' | 'bun-http' | 'command' | 'none'
	path?: string
	command?: string[]
	interval_s?: number
	timeout_s?: number
	retries?: number
	start_period_s?: number
}

export interface SpecPayload {
	image: string
	port: number
	replicas: number
	command: string[]
	healthcheck: HealthcheckPayload
	volumes: { name: string; mount: string }[]
	resources: { memory_mb?: number; cpus?: number } | null
}

/** `POST …/stages/{s}/apps`: `spec` o `compose_yaml` según `kind`, nunca los dos. */
export interface CreateAppPayload {
	slug: string
	kind: 'service' | 'compose'
	spec?: SpecPayload
	compose_yaml?: string
	deploy_timeout_s?: number
}

/** `PATCH …/apps/{a}`: lo que falta no se toca. */
export interface UpdateAppPayload {
	spec?: SpecPayload
	compose_yaml?: string
	deploy_timeout_s?: number
}
