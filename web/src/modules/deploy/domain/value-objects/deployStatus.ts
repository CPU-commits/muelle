// Los estados de un deploy (spec §5.1 y §7). Un deploy nace `queued`; si llega otro
// mientras espera, queda `superseded` sin empezar. El worker de la app lo pasa a
// `running` y termina en `succeeded` o `failed`.
export const DEPLOY_STATUSES = ['queued', 'running', 'succeeded', 'failed', 'superseded'] as const

export type DeployStatus = (typeof DEPLOY_STATUSES)[number]

const TERMINAL: ReadonlySet<DeployStatus> = new Set(['succeeded', 'failed', 'superseded'])

/** Ya no cambia: su log está completo. */
export function isTerminalStatus(status: DeployStatus): boolean {
	return TERMINAL.has(status)
}

/** Todavía puede escribir en el log o cambiar de estado. */
export function isActiveStatus(status: DeployStatus): boolean {
	return !TERMINAL.has(status)
}

export function isDeployStatus(raw: string): raw is DeployStatus {
	return (DEPLOY_STATUSES as readonly string[]).includes(raw)
}

export const DEPLOY_TRIGGERS = ['panel', 'api', 'rollback'] as const
export type DeployTrigger = (typeof DEPLOY_TRIGGERS)[number]

export const ACTOR_TYPES = ['user', 'token'] as const
export type ActorType = (typeof ACTOR_TYPES)[number]
