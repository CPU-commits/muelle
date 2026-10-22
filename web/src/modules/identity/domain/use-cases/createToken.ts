import type { ValidationError } from '@/shared/domain'
import { type CreatedApiToken, parseTokenScope } from '../entities/apiToken'
import { InvalidTokenExpiry, InvalidTokenName, MissingTokenProject } from '../errors/identityErrors'
import type { TokenDeps } from '../repositories/identityRepositories'
import { expiresInDays, tokenExpiryProblem } from '../value-objects/tokenExpiry'
import { parseTokenName, tokenNameProblem } from '../value-objects/tokenName'

export interface CreateTokenInput {
	name: string
	project: string
	/** `''`: todo el proyecto. */
	stage: string
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
	return async (input) =>
		tokens.create({
			name: parseTokenName(input.name),
			scope: parseTokenScope(input.project, input.stage),
			readOnly: false,
			expiresInDays: expiresInDays(input.expiresOn, today()),
		})
}
