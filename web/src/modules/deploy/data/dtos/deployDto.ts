import { z } from 'zod'
import { ACTOR_TYPES, DEPLOY_STATUSES, DEPLOY_TRIGGERS } from '../../domain'

// La Fase 2 todavía no existe en el backend: estas formas salen del spec (§5.1 tabla
// `deploys`, §7 y §10) y de las suposiciones anotadas en ../../README.md, que es lo que
// la Fase 2 tiene que respetar (o cambiar acá y en el README). snake_case como el resto
// de la API. Los objetos de Zod descartan los campos que no conocen: si el backend suma
// algo, el panel no se rompe.

const timestamp = z.iso.datetime({ offset: true })

export const deployStatusDtoSchema = z.enum(DEPLOY_STATUSES)

/** Una tarea de Swarm de un servicio. */
export const taskDtoSchema = z.object({
	id: z.string().min(1),
	slot: z.number().int().nullish(),
	state: z.string().min(1),
	desired_state: z.string().min(1),
	health: z.enum(['healthy', 'unhealthy', 'starting']).nullish(),
	node: z.string().nullish(),
	since: timestamp.nullish(),
	error: z.string().nullish(),
	// Solo las tareas del nodo de muelle se miden: el resto, null.
	cpu_percent: z.number().nonnegative().nullish(),
	memory_bytes: z.number().nonnegative().nullish(),
})

export const serviceStatusDtoSchema = z.object({
	name: z.string().min(1),
	replicas_desired: z.number().int().nonnegative(),
	replicas_running: z.number().int().nonnegative(),
	memory_limit_bytes: z.number().nonnegative().nullish(),
	tasks: z.array(taskDtoSchema),
})

/** `GET …/apps/{a}/status`. */
export const appStatusDtoSchema = z.object({
	services: z.array(serviceStatusDtoSchema),
})

/** Una fila de `deploys` (sin el log). `images` es `servicio → ref@digest`. */
export const deployDtoSchema = z.object({
	id: z.number().int().positive(),
	status: deployStatusDtoSchema,
	images: z.record(z.string(), z.string()),
	trigger: z.enum(DEPLOY_TRIGGERS),
	actor_type: z.enum(ACTOR_TYPES),
	// Una copia del nombre al crear el deploy: nunca null, puede ser "".
	actor_name: z.string(),
	error: z.string().nullish(),
	created_at: timestamp,
	started_at: timestamp.nullish(),
	finished_at: timestamp.nullish(),
	// El deploy más nuevo que dejó a este `superseded`.
	superseded_by: z.number().int().positive().nullish(),
})

/** `GET …/apps/{a}/deploys`: del más nuevo al más viejo. */
export const deployListDtoSchema = z.array(deployDtoSchema)

/** `POST …/deploys` y `POST …/rollback` (202). */
export const deployAcceptedDtoSchema = z.object({
	id: z.number().int().positive(),
	status_url: z.string().optional(),
})

/** `GET …/deploys/{id}/log?offset=N`. */
export const logPageDtoSchema = z.object({
	chunk: z.string(),
	next_offset: z.number().int().nonnegative(),
	status: deployStatusDtoSchema,
})

/** El `data` de cada evento `log` del SSE de `GET …/apps/{a}/logs`. */
export const appLogLineDtoSchema = z.object({
	time: timestamp.nullish(),
	task: z.string().nullish(),
	stream: z.enum(['stdout', 'stderr']).default('stdout'),
	line: z.string(),
})

export type TaskDto = z.infer<typeof taskDtoSchema>
export type ServiceStatusDto = z.infer<typeof serviceStatusDtoSchema>
export type AppStatusDto = z.infer<typeof appStatusDtoSchema>
export type DeployDto = z.infer<typeof deployDtoSchema>
export type DeployAcceptedDto = z.infer<typeof deployAcceptedDtoSchema>
export type LogPageDto = z.infer<typeof logPageDtoSchema>
export type AppLogLineDto = z.infer<typeof appLogLineDtoSchema>

/** Cuerpo de `POST …/deploys`: `{}` (redeploy), `{image}` o `{images}`. */
export type CreateDeployPayload =
	| Record<string, never>
	| { image: string }
	| { images: Record<string, string> }

// Los eventos del SSE (ver el README): `log` con una línea por evento y el offset
// siguiente en el `id`; `status` con el estado del deploy en el `data`.
export const DEPLOY_LOG_EVENT = 'log'
export const DEPLOY_STATUS_EVENT = 'status'
export const APP_LOG_EVENT = 'log'
