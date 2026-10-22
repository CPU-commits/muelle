import type { ValidationError } from '@/shared/domain'
import { type CreatedApiToken, parseTokenScope } from '../entities/apiToken'
import { InvalidTokenExpiry, InvalidTokenName, MissingTokenProject } from '../errors/identityErrors'
import type { TokenDeps } from '../repositories/identityRepositories'
import { expiresInDays, tokenExpiryProblem } from '../value-objects/tokenExpiry'
import { parseTokenName, tokenNameProblem } from '../value-objects/tokenName'

export interface CreateTokenInput {
	name: string
	/** Un slug, o `ALL_PROJECTS` (todos los proyectos, siempre de solo lectura). */
	project: string
	/** `''`: todo el proyecto. Con `ALL_PROJECTS` no cuenta. */
	stage: string
	/** Solo lee: no despliega ni cambia variables. Con `ALL_PROJECTS` es siempre true. */
	readOnly: boolean
	/** El valor del `<input type="date">`; `''` es que no vence. */
	expiresOn: string
}

/**
 * Todo lo que está mal en el formulario a la vez (el caso de uso corta en el primero):
 * así la pantalla marca cada campo de una vez y no de a uno por intento.
 */
export function createTokenProblems(
	input: CreateTokenInput,
	today: Date = new Date(),
): ValidationError[] {
	const problems: ValidationError[] = []
	const name = tokenNameProblem(input.name)
	if (name) problems.push(new InvalidTokenName(name))
	if (input.project.trim() === '') problems.push(new MissingTokenProject())
	const expiry = tokenExpiryProblem(input.expiresOn, today)
	if (expiry) problems.push(new InvalidTokenExpiry(expiry))
	return problems
}

/** Valida todo antes de mandarlo y devuelve el token con su valor en claro. */
export type CreateToken = (input: CreateTokenInput) => Promise<CreatedApiToken>

export function makeCreateToken(
	{ tokens }: TokenDeps,
	today: () => Date = () => new Date(),
): CreateToken {
	return async (input) => {
		const name = parseTokenName(input.name)
		const scope = parseTokenScope(input.project, input.stage)
		return tokens.create({
			name,
			scope,
			// No hay un token de escritura de todos los proyectos (contrato §8.1).
			readOnly: scope.kind === 'allProjects' || input.readOnly,
			expiresInDays: expiresInDays(input.expiresOn, today()),
		})
	}
}
