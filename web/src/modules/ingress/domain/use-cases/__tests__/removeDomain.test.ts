import { describe, expect, it } from 'vitest'
import { DomainNotFound } from '../../errors/ingressErrors'
import { APP, makeDomain } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { asHostname } from '../../value-objects/hostname'
import { makeRemoveDomain } from '../removeDomain'

describe('removeDomain', () => {
	it('borra el dominio', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [makeDomain('api.tudominio.cl')]

		await makeRemoveDomain({ domains: repo })({
			app: APP,
			hostname: asHostname('api.tudominio.cl'),
		})

		expect(repo.domains).toEqual([])
	})

	it('uno que no existe es DomainNotFound', async () => {
		await expect(
			makeRemoveDomain({ domains: new InMemoryDomainRepository() })({
				app: APP,
				hostname: asHostname('nada.tudominio.cl'),
			}),
		).rejects.toBeInstanceOf(DomainNotFound)
	})
})
