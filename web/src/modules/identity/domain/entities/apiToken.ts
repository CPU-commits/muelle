import { MissingTokenProject } from '../errors/identityErrors'
import type { TokenId } from '../value-objects/tokenId'
import type { TokenName } from '../value-objects/tokenName'

/** Un proyecto entero (`stage` null) o un proyecto + stage. */
export interface ProjectTokenScope {
	readonly kind: 'project'
	readonly project: string
	readonly stage: string | null
}

/**
 * Lo que ve un token: un proyecto, un proyecto + stage, o todos los proyectos (contrato
 * §8.1). Uno de todos los proyectos no tiene stage y siempre es de solo lectura.
 */
export type TokenScope = ProjectTokenScope | { readonly kind: 'allProjects' }

/** El valor del select de proyecto que elige "Todos los proyectos (solo lectura)". */
export const ALL_PROJECTS = '*'

/**
 * Quién creó el token. Un token no se revoca cuando se borra su creador (es del proyecto,
 * y el CI depende de él): entonces queda `deleted`.
 */
export type TokenCreator =
	| { readonly kind: 'user'; readonly username: string }
	| { readonly kind: 'deleted' }

/** Un token de API (`GET /tokens`). El valor en claro no vuelve nunca: solo su hash. */
export interface ApiToken {
	readonly id: TokenId
	readonly name: string
	readonly scope: TokenScope
	/** Solo lee: no despliega ni cambia variables (`403 read_only_token`). */
	readonly readOnly: boolean
	readonly createdBy: TokenCreator
	readonly createdAt: Date
	readonly lastUsedAt: Date | null
	readonly expiresAt: Date | null
}

/** Lo que devuelve crear un token: el token y su valor en claro, que se muestra una vez. */
export interface CreatedApiToken {
	readonly token: ApiToken
	readonly secret: string
}

/** Lo que se le pide al backend; `expiresInDays` 0 es que no vence. */
export interface NewApiToken {
	readonly name: TokenName
	readonly scope: TokenScope
	readonly readOnly: boolean
	readonly expiresInDays: number
}

/** `stage` vacío es "todo el proyecto"; `ALL_PROJECTS` es todos, y no tiene stage. */
export function parseTokenScope(project: string, stage: string): TokenScope {
	const projectSlug = project.trim()
	if (projectSlug === '') throw new MissingTokenProject()
	if (projectSlug === ALL_PROJECTS) return { kind: 'allProjects' }
	const stageSlug = stage.trim()
	return { kind: 'project', project: projectSlug, stage: stageSlug === '' ? null : stageSlug }
}

/**
 * "auditfy/staging", o "auditfy" si es de todo el proyecto. El de todos los proyectos lo
 * pone la pantalla, traducido.
 */
export function scopeLabel(scope: ProjectTokenScope): string {
	return scope.stage ? `${scope.project}/${scope.stage}` : scope.project
}

/** Solo un token que no es de solo lectura despliega (y sirve para la GitHub Action). */
export function canDeploy(token: ApiToken): boolean {
	return !token.readOnly
}

export function isTokenExpired(token: ApiToken, now: Date = new Date()): boolean {
	return token.expiresAt !== null && token.expiresAt.getTime() <= now.getTime()
}

/** Los vigentes y los vencidos por separado, en el orden en que llegaron. */
export function partitionTokens(
	tokens: readonly ApiToken[],
	now: Date = new Date(),
): { active: ApiToken[]; expired: ApiToken[] } {
	const active: ApiToken[] = []
	const expired: ApiToken[] = []
	for (const token of tokens) (isTokenExpired(token, now) ? expired : active).push(token)
	return { active, expired }
}

/**
 * El `app:` del ejemplo de GitHub Actions (`proyecto/stage/app`, spec §14). El token no
 * sabe de qué app es, y si es de todo el proyecto tampoco de qué stage: esas partes quedan
 * como marcadores.
 */
export function actionAppPath(scope: ProjectTokenScope): string {
	return `${scope.project}/${scope.stage ?? '<stage>'}/<app>`
}
