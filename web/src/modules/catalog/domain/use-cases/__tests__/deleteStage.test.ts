import { describe, expect, it } from 'vitest'
import { makeDeleteStage, type ProjectSlug, StageNotEmpty, type StageSlug } from '../..'
import {
	fakeProject,
	fakeServiceApp,
	fakeStage,
	InMemoryCatalogRepository,
} from '../../repositories/__fakes__/inMemoryCatalogRepository'

const input = { project: 'auditfy' as ProjectSlug, stage: 'staging' as StageSlug }

describe('deleteStage', () => {
	it('borra un stage sin apps', async () => {
		const catalog = new InMemoryCatalogRepository({
			projects: [fakeProject('auditfy')],
			stages: [fakeStage('auditfy', 'staging'), fakeStage('auditfy', 'production')],
		})

		await makeDeleteStage({ catalog })(input)

		expect(catalog.stages.map((stage) => stage.slug)).toEqual(['production'])
	})

	it('con apps es StageNotEmpty', async () => {
		const catalog = new InMemoryCatalogRepository({
			projects: [fakeProject('auditfy')],
			stages: [fakeStage('auditfy', 'staging')],
			apps: [fakeServiceApp('auditfy', 'staging', 'api')],
		})

		await expect(makeDeleteStage({ catalog })(input)).rejects.toBeInstanceOf(StageNotEmpty)
		expect(catalog.stages).toHaveLength(1)
	})
})
