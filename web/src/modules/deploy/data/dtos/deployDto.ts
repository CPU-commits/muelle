import { z } from 'zod'
import { ACTOR_TYPES, DEPLOY_STATUSES, DEPLOY_TRIGGERS } from '../../domain'

// Las formas del contrato HTTP de la Fase 2 (`docs/superpowers/specs/2026-10-10-contrato-
// http-fases-2-3.md`, §2, en este árbol), resumido en ../../README.md. snake_case como
// el resto de la API. Los objetos de Zod descartan los campos que no conocen: si el
// backend suma algo, el panel no se rompe.

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
	// Solo las tareas del nodo de muelle se miden: el resto, null. El panel lee estas.
	cpu_percent: z.number().nonnegative().nullish(),
	memory_bytes: z.number().nonnegative().nullish(),
})

export const serviceStatusDtoSchema = z.object({
	name: z.string().min(1),
	image: z.string().nullish(),
	// `Service.UpdateStatus.State`, o null si nunca se actualizó.
	update_state: z.string().nullish(),
	// Las sumas de las tareas medidas, para otros clientes: el panel suma las de las tareas.
	cpu_percent: z.number().nonnegative().nullish(),
	memory_bytes: z.number().nonnegative().nullish(),
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
	actor_id: z.number().int().nullish(),
	// Una copia del nombre al crear el deploy: nunca null, puede ser "".
	actor_name: z.string(),
	error: z.string().nullish(),
	created_at: timestamp,
	started_at: timestamp.nullish(),
	finished_at: timestamp.nullish(),
	// El deploy más nuevo que dejó a este `superseded`.
	superseded_by: z.number().int().positive().nullish(),
	// `servicio → ref pedida` (§2.1): con digest solo si alguien lo fijó (un rollback, un
	// override o la configuración). `{}` hasta que se resolvieron las imágenes; nunca null.
	requested_images: z.record(z.string(), z.string()),
	// El paso 7 no pudo actualizar las rutas de Caddy y escribió la advertencia (§2.11).
	// Nunca null.
	routes_warning: z.boolean(),
})

/** `GET …/apps/{a}/deploys`: del más nuevo al más viejo. */
export const deployListDtoSchema = z.array(deployDtoSchema)

/** `POST …/deploys` y `POST …/rollback` (202). */
export const deployAcceptedDtoSchema = z.object({
	id: z.number().int().positive(),
	status: deployStatusDtoSchema.optional(),
	status_url: z.string().optional(),
})

/** `GET …/deploys/{id}/log?offset=N`. */
export const logPageDtoSchema = z.object({
	chunk: z.string(),
	next_offset: z.number().int().nonnegative(),
	status: deployStatusDtoSchema,
	superseded_by: z.number().int().positive().nullish(),
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

// Los eventos del SSE (contrato §2.6 y §2.9): `log` con una línea por evento y el offset
// siguiente en el `id`; `status` con el estado del deploy en el `data`; en los logs de una
// app, `end` (`eof` o `timeout`) antes de cerrar.
export const DEPLOY_LOG_EVENT = 'log'
export const DEPLOY_STATUS_EVENT = 'status'
export const APP_LOG_EVENT = 'log'
export const APP_LOG_END_EVENT = 'end'
