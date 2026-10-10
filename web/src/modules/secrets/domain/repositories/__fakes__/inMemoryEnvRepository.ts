import type { EnvKey, EnvOwner, EnvPatch, EnvReplacement, EnvRepository, EnvVar } from '../..'
import {
	EnvLimitExceeded,
	EnvOwnerNotFound,
	envVarSize,
	MAX_ENV_VARS,
	MAX_OWNER_BYTES,
	ownerLabel,
	sortEnvVars,
} from '../..'

/** Una variable guardada: el fake, como el backend, conoce el valor de las secretas. */
export interface StoredEnvVar {
	key: string
	value: string
	secret: boolean
	updatedAt: Date
}

export interface FakeEnvSeed {
	/** Por `ownerLabel` (`stage:p/s`, `app:p/s/a`). Un dueño que no está, no existe. */
	owners?: Record<string, ReadonlyArray<Omit<StoredEnvVar, 'updatedAt'> & { updatedAt?: Date }>>
	now?: () => Date
}

export const FAKE_EPOCH = new Date('2026-10-01T12:00:00.000Z')

// Las variables en memoria con las reglas que importan al panel: los secretos vuelven
// enmascarados, un PATCH no reescribe lo que no cambia, los límites del dueño se revisan
// con lo guardado y un dueño que no existe es EnvOwnerNotFound.
export class InMemoryEnvRepository implements EnvRepository {
	readonly owners = new Map<string, StoredEnvVar[]>()
	readonly now: () => Date
	readonly calls = {
		list: 0,
		replace: [] as { owner: EnvOwner; replacement: EnvReplacement }[],
		patch: [] as { owner: EnvOwner; patch: EnvPatch }[],
	}

	constructor(seed: FakeEnvSeed = {}) {
		this.now = seed.now ?? (() => FAKE_EPOCH)
		for (const [label, vars] of Object.entries(seed.owners ?? {})) {
			this.owners.set(
				label,
				vars.map((envVar) => ({ ...envVar, updatedAt: envVar.updatedAt ?? FAKE_EPOCH })),
			)
		}
	}

	stored(owner: EnvOwner): StoredEnvVar[] {
		const vars = this.owners.get(ownerLabel(owner))
		if (!vars) throw new EnvOwnerNotFound()
		return vars
	}

	list(owner: EnvOwner): Promise<EnvVar[]> {
		this.calls.list += 1
		try {
			return Promise.resolve(sortEnvVars(this.stored(owner).map(mask)))
		} catch (error) {
			return Promise.reject(error)
		}
	}

	replace(owner: EnvOwner, replacement: EnvReplacement): Promise<void> {
		this.calls.replace.push({ owner, replacement })
		try {
			this.stored(owner)
			const at = this.now()
			this.owners.set(
				ownerLabel(owner),
				[...replacement.vars].map(([key, value]) => ({
					key,
					value,
					secret: replacement.secret,
					updatedAt: at,
				})),
			)
			return Promise.resolve()
		} catch (error) {
			return Promise.reject(error)
		}
	}

	patch(owner: EnvOwner, patch: EnvPatch): Promise<void> {
		this.calls.patch.push({ owner, patch })
		try {
			const stored = this.stored(owner)
			const byKey = new Map(stored.map((envVar) => [envVar.key, envVar]))
			const changed = [...patch.set].filter(([key, value]) => {
				const current = byKey.get(key)
				return !current || current.value !== value || current.secret !== patch.secret
			})
			if (changed.length === 0 && !patch.unset.some((key) => byKey.has(key))) {
				return Promise.resolve()
			}
			for (const key of patch.unset) byKey.delete(key)
			const at = this.now()
			for (const [key, value] of changed) {
				byKey.set(key, { key, value, secret: patch.secret, updatedAt: at })
			}
			if (changed.length > 0) checkTotals([...byKey.values()])
			this.owners.set(ownerLabel(owner), [...byKey.values()])
			return Promise.resolve()
		} catch (error) {
			return Promise.reject(error)
		}
	}
}

function mask(envVar: StoredEnvVar): EnvVar {
	const key = envVar.key as EnvKey
	return envVar.secret
		? { key, secret: true, value: null, updatedAt: envVar.updatedAt }
		: { key, secret: false, value: envVar.value, updatedAt: envVar.updatedAt }
}

function checkTotals(vars: readonly StoredEnvVar[]): void {
	if (vars.length > MAX_ENV_VARS) throw new EnvLimitExceeded('count', 'set')
	const size = vars.reduce((total, envVar) => total + envVarSize(envVar.key, envVar.value), 0)
	if (size > MAX_OWNER_BYTES) throw new EnvLimitExceeded('size', 'set')
}
