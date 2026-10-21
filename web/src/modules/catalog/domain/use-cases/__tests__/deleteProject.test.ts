import { describe, expect, it } from 'vitest'
import { makeDeleteProject, ProjectNotEmpty, type ProjectSlug } from '../..'
import {
	fakeProject,
	fakeStage,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

describe('deleteProject', () => {
	it('borra un proyecto vacío', async () => {
		const catalog = new InMemoryCatalogRepository({ projects: [fakeProject('vacio')] })

		await makeDeleteProject({ catalog })('vacio' as ProjectSlug)

		expect(catalog.projects).toEqual([])
	})

	it('con stages es ProjectNotEmpty y no borra nada', async () => {
		const catalog = new InMemoryCatalogRepository({
			projects: [fakeProject('auditfy')],
			stages: [fakeStage('auditfy', 'staging')],
		})

		await expect(
			makeDeleteProject({ catalog })('auditfy' as ProjectSlug),
		).rejects.toBeInstanceOf(ProjectNotEmpty)
		expect(catalog.projects).toHaveLength(1)
	})
})
