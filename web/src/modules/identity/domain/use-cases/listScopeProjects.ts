import type { ProjectRef } from '../entities/scope'
import type { ScopeDeps } from '../repositories/identityRepositories'

export type ListScopeProjects = (signal?: AbortSignal) => Promise<ProjectRef[]>

export function makeListScopeProjects({ scopes }: ScopeDeps): ListScopeProjects {
	return async (signal) => scopes.projects(signal)
}
