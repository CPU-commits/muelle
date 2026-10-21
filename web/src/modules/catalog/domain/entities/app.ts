import type { AppSpec } from '../value-objects/appSpec'
import { summarizeCompose } from '../value-objects/composeYaml'
import type { AppSlug, ProjectSlug, StageSlug } from '../value-objects/slugs'

export type AppKind = 'service' | 'compose'

/** Dónde vive una app: lo que va en la URL (`/projects/{p}/stages/{s}/apps/{a}`). */
export interface AppRef {
	readonly project: ProjectSlug
	readonly stage: StageSlug
	readonly app: AppSlug
}

/**
 * Un servicio de la app según la configuración guardada (no según Swarm): el único de una
 * app de formulario, con el slug de la app, o cada uno del compose. `ports` son los que
 * declara (`port` del spec, `expose` del compose): una sugerencia, no se publican.
 */
export interface AppService {
	readonly name: string
	readonly ports: readonly number[]
}

interface AppBase {
	readonly project: ProjectSlug
	readonly stage: StageSlug
	readonly slug: AppSlug
	readonly deployTimeoutSeconds: number
	/** Cambia con el spec o el compose; el timeout no cuenta. */
	readonly configChangedAt: Date
	/** `null` mientras no se desplegó nunca. */
	readonly lastDeployedAt: Date | null
	/** La configuración cambió después del último deploy. Sin deploys, nunca. */
	readonly pendingChanges: boolean
	readonly createdAt: Date
	/** Sus servicios, de la configuración guardada (contrato §2.12). */
	readonly services: readonly AppService[]
}

/** Una app de formulario: un servicio descrito por su spec. */
export interface ServiceApp extends AppBase {
	readonly kind: 'service'
	readonly spec: AppSpec
}

/** Una app que es un compose completo, en YAML. */
export interface ComposeApp extends AppBase {
	readonly kind: 'compose'
	readonly composeYaml: string
}

export type App = ServiceApp | ComposeApp

/**
 * Una ref desde los parámetros de la ruta. No valida: si el slug no existe, el backend
 * responde 404 y la pantalla lo muestra.
 */
export function appRefFrom(project: string, stage: string, app: string): AppRef {
	return {
		project: project as ProjectSlug,
		stage: stage as StageSlug,
		app: app as AppSlug,
	}
}

export function appRefOf(app: Pick<App, 'project' | 'stage' | 'slug'>): AppRef {
	return { project: app.project, stage: app.stage, app: app.slug }
}

export function sameAppRef(a: AppRef, b: AppRef): boolean {
	return a.project === b.project && a.stage === b.stage && a.app === b.app
}

/** El stack de Swarm de la app, `<proyecto>_<stage>_<app>` (§5.2 del spec). */
export function appStackName(ref: AppRef): string {
	return `${ref.project}_${ref.stage}_${ref.app}`
}

/**
 * Los destinos que puede ofrecer el formulario de un dominio (`target_service:target_port`):
 * uno por cada puerto que declara cada servicio. Un servicio sin puertos no da ninguno
 * (el puerto no se adivina).
 */
export function serviceTargets(app: Pick<App, 'services'>): string[] {
	return app.services.flatMap((service) =>
		service.ports.map((port) => `${service.name}:${String(port)}`),
	)
}

export type DeployState = 'never_deployed' | 'pending_changes' | 'up_to_date'

/** Lo que el catálogo sabe del deploy sin preguntarle a Docker (Fase 1). */
export function deployStateOf(app: Pick<App, 'lastDeployedAt' | 'pendingChanges'>): DeployState {
	if (app.lastDeployedAt === null) return 'never_deployed'
	return app.pendingChanges ? 'pending_changes' : 'up_to_date'
}

export type AppContents =
	| { readonly kind: 'image'; readonly image: string }
	| { readonly kind: 'services'; readonly services: readonly string[] }

/** Qué corre la app: la imagen del spec o los servicios del compose. */
export function appContents(app: App): AppContents {
	if (app.kind === 'service') return { kind: 'image', image: app.spec.image }
	return {
		kind: 'services',
		services: summarizeCompose(app.composeYaml).map((service) => service.name),
	}
}

/**
 * Lo que identifica a una app en el encabezado de sus pantallas (Configuración y Resumen):
 * nombre, tipo, qué corre, si tiene cambios sin desplegar y dónde vive en Swarm.
 */
export interface AppSummary {
	readonly ref: AppRef
	readonly name: string
	readonly kind: AppKind
	readonly contents: AppContents
	readonly deployState: DeployState
	readonly pendingChanges: boolean
	readonly lastDeployedAt: Date | null
	readonly stackName: string
	readonly stageNetwork: string
}

export function appSummaryOf(app: App): AppSummary {
	const ref = appRefOf(app)
	return {
		ref,
		name: app.slug,
		kind: app.kind,
		contents: appContents(app),
		deployState: deployStateOf(app),
		pendingChanges: app.pendingChanges,
		lastDeployedAt: app.lastDeployedAt,
		stackName: appStackName(ref),
		stageNetwork: `${app.project}_${app.stage}`,
	}
}
