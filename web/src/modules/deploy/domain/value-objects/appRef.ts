// Una app se identifica por sus tres slugs (spec §10: "los recursos se identifican por
// slug"). Llegan de la URL del panel, que ya es la de una app que existe: el dominio no
// vuelve a validar el formato.
export interface AppRef {
	readonly project: string
	readonly stage: string
	readonly app: string
}

/** El stack de Swarm de la app: `proyecto_stage_app` (spec §5.2). */
export function stackName(ref: AppRef): string {
	return `${ref.project}_${ref.stage}_${ref.app}`
}

/** La red overlay del stage, que comparten todas sus apps (spec §3.2). */
export function stageNetwork(ref: AppRef): string {
	return `${ref.project}_${ref.stage}`
}

/** El nombre del servicio en Swarm: `proyecto_stage_app_servicio`. */
export function swarmServiceName(ref: AppRef, service: string): string {
	return `${stackName(ref)}_${service}`
}

export function sameApp(a: AppRef, b: AppRef): boolean {
	return a.project === b.project && a.stage === b.stage && a.app === b.app
}
