// El estado real de una app en Swarm (`GET …/apps/{a}/status`, spec §10): por servicio,
// las réplicas deseadas y corriendo, las tareas con su estado y error, y CPU% y RAM de
// cada tarea. El backend solo mide las tareas del nodo donde corre muelle: las demás
// llegan sin medida y el panel suma lo que hay.

export type TaskHealth = 'healthy' | 'unhealthy' | 'starting'

export interface ServiceTask {
	/** El id de la tarea de Swarm. */
	readonly id: string
	/** `api.1`: el slot de la réplica. Null en un servicio global. */
	readonly slot: number | null
	/** El estado de Swarm tal cual: `running`, `starting`, `shutdown`, `failed`… */
	readonly state: string
	/** `running` en las tareas vigentes; `shutdown` en las históricas. */
	readonly desiredState: string
	readonly health: TaskHealth | null
	readonly node: string | null
	/** Desde cuándo está en `state`. */
	readonly since: Date | null
	readonly error: string | null
	/** CPU de la tarea, en % de un núcleo. Null si no se midió (otro nodo, o detenida). */
	readonly cpuPercent: number | null
	/** RAM de la tarea, en bytes. Null si no se midió. */
	readonly memoryBytes: number | null
}

/**
 * `Service.UpdateStatus.State` de Swarm (`updating`, `completed`, `paused`,
 * `rollback_started`, `rollback_paused`, `rollback_completed`), o null si nunca se actualizó.
 */
export type ServiceUpdateState = string

export interface ServiceStatus {
	readonly name: string
	/** La imagen de sus tareas (`Service.Image`), con digest; null si no vino. */
	readonly image: string | null
	readonly updateState: ServiceUpdateState | null
	readonly desiredReplicas: number
	readonly runningReplicas: number
	/** El límite de RAM de CADA tarea (`resources.memory_mb`), en bytes. */
	readonly memoryLimitBytes: number | null
	readonly tasks: readonly ServiceTask[]
}

export interface AppStatus {
	readonly services: readonly ServiceStatus[]
}

/**
 * - `running`: todos los servicios con todas sus réplicas.
 * - `degraded`: corre algo, pero falta alguna réplica.
 * - `down`: hay réplicas deseadas y no corre ninguna.
 * - `stopped`: todos los servicios en 0 réplicas a propósito.
 * - `not_deployed`: no hay servicios en Swarm (nunca se desplegó).
 */
export type AppHealth = 'running' | 'degraded' | 'down' | 'stopped' | 'not_deployed'

export type ServiceHealth = Exclude<AppHealth, 'not_deployed'>

export function serviceHealth(service: ServiceStatus): ServiceHealth {
	if (service.desiredReplicas === 0) return 'stopped'
	if (service.runningReplicas === 0) return 'down'
	if (service.runningReplicas < service.desiredReplicas) return 'degraded'
	return 'running'
}

export function appHealth(status: AppStatus): AppHealth {
	if (status.services.length === 0) return 'not_deployed'
	const healths = status.services.map(serviceHealth)
	if (healths.every((health) => health === 'stopped')) return 'stopped'
	const live = healths.filter((health) => health !== 'stopped')
	if (live.every((health) => health === 'running')) return 'running'
	if (live.every((health) => health === 'down')) return 'down'
	return 'degraded'
}

export interface ReplicaCount {
	readonly running: number
	readonly desired: number
}

/** Las réplicas de toda la app ("Corriendo · 2/2"). */
export function replicaCount(status: AppStatus): ReplicaCount {
	return status.services.reduce<ReplicaCount>(
		(total, service) => ({
			running: total.running + service.runningReplicas,
			desired: total.desired + service.desiredReplicas,
		}),
		{ running: 0, desired: 0 },
	)
}

/**
 * El servicio que más le falta: el que explica un "Degradada" o un "Caída" ("Caída ·
 * gateway 0/1"). Null si todos están completos.
 */
export function weakestService(status: AppStatus): ServiceStatus | null {
	let weakest: ServiceStatus | null = null
	let worstRatio = 1
	for (const service of status.services) {
		if (service.desiredReplicas === 0) continue
		const ratio = service.runningReplicas / service.desiredReplicas
		if (ratio < worstRatio) {
			worstRatio = ratio
			weakest = service
		}
	}
	return weakest
}

/** Una tarea vigente (Swarm quiere que corra); las otras son historia. */
export function isCurrentTask(task: ServiceTask): boolean {
	return task.desiredState === 'running'
}

/** `api.1`, o `api` en un servicio global. */
export function taskLabel(service: string, task: ServiceTask): string {
	return task.slot === null ? service : `${service}.${task.slot}`
}

/** Los primeros 7 del id, como `docker service ps`. */
export function shortTaskId(id: string): string {
	return id.slice(0, 7)
}

/** Los nodos donde corren las tareas vigentes, sin repetir. */
export function serviceNodes(service: ServiceStatus): string[] {
	const nodes = service.tasks
		.filter(isCurrentTask)
		.map((task) => task.node)
		.filter((node): node is string => node !== null)
	return [...new Set(nodes)]
}

/** Las tareas de todos los servicios, vigentes primero y después las más recientes. */
export function allTasks(status: AppStatus): { service: string; task: ServiceTask }[] {
	const rows = status.services.flatMap((service) =>
		service.tasks.map((task) => ({ service: service.name, task })),
	)
	return rows.sort((a, b) => {
		const current = Number(isCurrentTask(b.task)) - Number(isCurrentTask(a.task))
		if (current !== 0) return current
		return (b.task.since?.getTime() ?? 0) - (a.task.since?.getTime() ?? 0)
	})
}

export interface ServiceUsage {
	/** CPU de las tareas medidas, sumada (% de un núcleo). Null si no se midió ninguna. */
	readonly cpuPercent: number | null
	/** RAM promedio por tarea medida, en bytes. Null si no se midió ninguna. */
	readonly memoryPerTaskBytes: number | null
	/** Cuántas tareas vigentes se midieron, y cuántas hay. */
	readonly sampled: number
	readonly current: number
}

/** CPU y RAM del servicio, juntando lo que se midió de sus tareas vigentes. */
export function serviceUsage(service: ServiceStatus): ServiceUsage {
	const current = service.tasks.filter(isCurrentTask)
	const cpu = current.flatMap((task) => (task.cpuPercent === null ? [] : [task.cpuPercent]))
	const memory = current.flatMap((task) => (task.memoryBytes === null ? [] : [task.memoryBytes]))
	const sum = (values: number[]) => values.reduce((total, value) => total + value, 0)
	return {
		cpuPercent: cpu.length > 0 ? sum(cpu) : null,
		memoryPerTaskBytes: memory.length > 0 ? sum(memory) / memory.length : null,
		sampled: Math.max(cpu.length, memory.length),
		current: current.length,
	}
}
