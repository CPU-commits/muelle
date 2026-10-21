import type { Me } from '../entities/me'
import type { AuthDeps } from '../repositories/authRepository'

export type GetMe = (signal?: AbortSignal) => Promise<Me>

export function makeGetMe({ auth }: AuthDeps): GetMe {
	return async (signal) => auth.me(signal)
}
