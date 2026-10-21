import type { AppDomain, DomainChanges, NewDomain } from '../../entities/appDomain'
import { domainState } from '../../entities/appDomain'
import { DomainNotFound, HostnameTaken, NothingToAdopt } from '../../errors/ingressErrors'
import type { AppRef } from '../../value-objects/appRef'
import type { Hostname } from '../../value-objects/hostname'
import type { DomainRepository } from '../domainRepository'
import { makeDomain } from './domainFixtures'

// El fake del puerto para los tests de dominio: una lista en memoria, sin red.
export class InMemoryDomainRepository implements DomainRepository {
	domains: AppDomain[] = []
	readonly calls: string[] = []

	async list(): Promise<readonly AppDomain[]> {
		this.calls.push('list')
		return this.domains
	}

	async create(_app: AppRef, domain: NewDomain): Promise<AppDomain> {
		this.calls.push(`create ${domain.hostname}`)
		if (this.domains.some((existing) => existing.hostname === domain.hostname)) {
			throw new HostnameTaken()
		}
		const created = makeDomain(domain.hostname, {
			target: domain.target,
			proxied: domain.proxied,
			status: 'pending',
			certificate: null,
		})
		this.domains = [...this.domains, created]
		return created
	}

	async update(_app: AppRef, hostname: Hostname, changes: DomainChanges): Promise<AppDomain> {
		this.calls.push(`update ${hostname}`)
		const updated = { ...this.#find(hostname), ...changes }
		this.#replace(updated)
		return updated
	}

	async remove(_app: AppRef, hostname: Hostname): Promise<void> {
		this.calls.push(`remove ${hostname}`)
		this.#find(hostname)
		this.domains = this.domains.filter((domain) => domain.hostname !== hostname)
	}

	async adopt(_app: AppRef, hostname: Hostname): Promise<AppDomain> {
		this.calls.push(`adopt ${hostname}`)
		const domain = this.#find(hostname)
		if (domainState(domain) !== 'record_exists') throw new NothingToAdopt()
		const adopted: AppDomain = {
			...domain,
			recordId: 'adoptado',
			status: 'pending',
			errorKind: null,
			lastError: null,
		}
		this.#replace(adopted)
		return adopted
	}

	async retry(_app: AppRef, hostname: Hostname): Promise<AppDomain> {
		this.calls.push(`retry ${hostname}`)
		const domain = this.#find(hostname)
		const retried: AppDomain = {
			...domain,
			status: 'pending',
			errorKind: null,
			lastError: null,
		}
		this.#replace(retried)
		return retried
	}

	#find(hostname: Hostname): AppDomain {
		const domain = this.domains.find((candidate) => candidate.hostname === hostname)
		if (!domain) throw new DomainNotFound()
		return domain
	}

	#replace(updated: AppDomain): void {
		this.domains = this.domains.map((domain) =>
			domain.hostname === updated.hostname ? updated : domain,
		)
	}
}
