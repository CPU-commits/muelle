import { describe, expect, it } from 'vitest'
import { makeListScopeProjects, makeListScopeStages, TokenScopeNotFound } from '../..'
import { InMemoryIdentityRepository } from '../../repositories/__fakes__/inMemoryIdentityRepository'

describe('listScopeStages', () => {
	const repo = new InMemoryIdentityRepository({
		projects: { auditfy: ['staging', 'production'], extractor: [] },
	})

	it('lista los proyectos y los stages de uno', async () => {
		await expect(makeListScopeProjects({ scopes: repo.asScopes() })()).resolves.toEqual([
			{ slug: 'auditfy', name: 'auditfy' },
			{ slug: 'extractor', name: 'extractor' },
		])
		await expect(makeListScopeStages({ scopes: repo.asScopes() })('auditfy')).resolves.toEqual([
			{ slug: 'staging' },
			{ slug: 'production' },
		])
	})

	it('un proyecto sin stages da una lista vacía, y uno que no existe un error', async () => {
		const stages = makeListScopeStages({ scopes: repo.asScopes() })

		await expect(stages('extractor')).resolves.toEqual([])
		await expect(stages('nadie')).rejects.toBeInstanceOf(TokenScopeNotFound)
	})
})
