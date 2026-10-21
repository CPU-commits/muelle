import type { StageRef } from '../entities/scope'
import type { ScopeDeps } from '../repositories/identityRepositories'

export type ListScopeStages = (project: string, signal?: AbortSignal) => Promise<StageRef[]>

export function makeListScopeStages({ scopes }: ScopeDeps): ListScopeStages {
	return async (project, signal) => scopes.stages(project, signal)
}
