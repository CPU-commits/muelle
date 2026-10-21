import { describe, expect, it } from 'vitest'
import { InvalidProjectName, makeRenameProject, ProjectNotFound, type ProjectSlug } from '../..'
import {
	fakeProject,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const auditfy = 'auditfy' as ProjectSlug

describe('renameProject', () => {
	it('cambia el nombre y deja el slug', async () => {
		const catalog = new InMemoryCatalogRepository({ projects: [fakeProject('auditfy')] })

		await expect(
			makeRenameProject({ catalog })({ project: auditfy, name: ' Auditfy SA ' }),
		).resolves.toMatchObject({ slug: 'auditfy', name: 'Auditfy SA' })
	})

	it('valida el nombre y propaga el proyecto que no existe', async () => {
		const catalog = new InMemoryCatalogRepository()
		const rename = makeRenameProject({ catalog })

		await expect(rename({ project: auditfy, name: '' })).rejects.toBeInstanceOf(
			InvalidProjectName,
		)
		expect(catalog.calls.renameProject).toBe(0)
		await expect(rename({ project: auditfy, name: 'X' })).rejects.toBeInstanceOf(
			ProjectNotFound,
		)
	})
})
