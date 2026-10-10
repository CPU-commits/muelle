import { type CreatedApiToken, parseTokenScope } from '../entities/apiToken'
import type { TokenDeps } from '../repositories/identityRepositories'
import { expiresInDays } from '../value-objects/tokenExpiry'
import { parseTokenName } from '../value-objects/tokenName'

export interface CreateTokenInput {
	name: string
	project: string
	/** `''`: todo el proyecto. */
	stage: string
	/** El valor del `<input type="date">`; `''` es que no vence. */
	expiresOn: string
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
			expiresInDays: expiresInDays(input.expiresOn, today()),
		})
}
