import type { ApiToken } from '../entities/apiToken'
import type { TokenDeps } from '../repositories/identityRepositories'

export type ListTokens = (signal?: AbortSignal) => Promise<ApiToken[]>

export function makeListTokens({ tokens }: TokenDeps): ListTokens {
	return async (signal) => tokens.list(signal)
}
