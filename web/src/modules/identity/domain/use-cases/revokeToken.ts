import type { TokenDeps } from '../repositories/identityRepositories'
import type { TokenId } from '../value-objects/tokenId'

/** Lo que use el token recibe 401 desde ya. No se puede deshacer. */
export type RevokeToken = (id: TokenId) => Promise<void>

export function makeRevokeToken({ tokens }: TokenDeps): RevokeToken {
	return async (id) => tokens.revoke(id)
}
