import type {
	AuditEvent,
	AuditEventId,
	AuditQuery,
	AuditRepository,
	NewRegistry,
	ProxySettings,
	Registry,
	RegistryCredentials,
	RegistryRepository,
	Settings,
	SettingsPatch,
	SettingsRepository,
} from '../..'
import { RegistryExists, TotpRequiredFirst } from '../..'

export function fakeProxySettings(overrides: Partial<ProxySettings> = {}): ProxySettings {
	return {
		cloudflare: { tokenSet: true, zones: 4 },
		publicIpv4: '203.0.113.24',
		publicIpv6: '',
		acmeEmail: 'ops@tudominio.cl',
		panelDomain: 'panel.tudominio.cl',
		httpPort: 80,
		httpsPort: 443,
		...overrides,
	}
}

export function fakeSettings(overrides: Partial<Settings> = {}): Settings {
	return {
		require2fa: false,
		proxy: fakeProxySettings(),
		panelAccess: { allowedCidrs: [], clientIp: '198.51.100.17' },
		...overrides,
	}
}

export interface FakeSettingsSeed {
	settings?: Settings
	/** Si el usuario de la sesión tiene 2FA: sin ella no puede exigirla. */
	sessionHasTotp?: boolean
}

// Settings en memoria con la regla de `PatchSettings` del backend: exigir 2FA exige
// tenerla. Guarda los patches para que los tests vean qué se mandó.
export class InMemorySettingsRepository implements SettingsRepository {
	settings: Settings
	sessionHasTotp: boolean
	readonly patches: SettingsPatch[] = []

	constructor(seed: FakeSettingsSeed = {}) {
		this.settings = seed.settings ?? fakeSettings()
		this.sessionHasTotp = seed.sessionHasTotp ?? true
	}

	get(): Promise<Settings> {
		return Promise.resolve(this.settings)
	}

	patch(patch: SettingsPatch): Promise<Settings> {
		this.patches.push(patch)
		if (patch.require2fa === true && !this.sessionHasTotp) {
			return Promise.reject(new TotpRequiredFirst())
		}
		const { proxy, panelAccess } = this.settings
		this.settings = {
			require2fa: patch.require2fa ?? this.settings.require2fa,
			proxy: proxy && {
				...proxy,
				publicIpv4: patch.publicIpv4 ?? proxy.publicIpv4,
				publicIpv6: patch.publicIpv6 ?? proxy.publicIpv6,
				acmeEmail: patch.acmeEmail ?? proxy.acmeEmail,
				httpPort: patch.httpPort ?? proxy.httpPort,
				httpsPort: patch.httpsPort ?? proxy.httpsPort,
				cloudflare:
					patch.cloudflareToken !== undefined
						? { tokenSet: true, zones: proxy.cloudflare.zones }
						: proxy.cloudflare,
			},
			panelAccess: panelAccess && {
				...panelAccess,
				allowedCidrs: patch.panelAllowedCidrs ?? panelAccess.allowedCidrs,
			},
		}
		return Promise.resolve(this.settings)
	}
}

export function fakeAuditEvents(
	count: number,
	at = new Date('2026-10-10T12:00:00Z'),
): AuditEvent[] {
	// Del más nuevo (id = count) al más viejo (id = 1), como los devuelve el backend.
	return Array.from({ length: count }, (_, index) => {
		const id = count - index
		return {
			id: id as AuditEventId,
			actor: { kind: 'user', id: 1 },
			action: 'project.update',
			target: `project:p${id}`,
			at: new Date(at.getTime() - index * 60_000),
		}
	})
}

export class InMemoryAuditRepository implements AuditRepository {
	readonly events: AuditEvent[]
	readonly queries: AuditQuery[] = []

	constructor(events: AuditEvent[] = []) {
		this.events = events
	}

	list(query: AuditQuery): Promise<AuditEvent[]> {
		this.queries.push(query)
		const before = query.before
		const older = before === undefined ? this.events : this.events.filter((e) => e.id < before)
		return Promise.resolve(older.slice(0, query.limit))
	}
}

export class InMemoryRegistryRepository implements RegistryRepository {
	registries: (Registry & { password: string })[]
	readonly calls = { create: 0, update: 0 }

	constructor(registries: (Registry & { password: string })[] = []) {
		this.registries = registries
	}

	list(): Promise<Registry[]> {
		return Promise.resolve(this.registries.map(({ host, username }) => ({ host, username })))
	}

	create(registry: NewRegistry): Promise<Registry> {
		this.calls.create += 1
		if (this.registries.some((r) => r.host === registry.host)) {
			return Promise.reject(new RegistryExists())
		}
		this.registries = [...this.registries, { ...registry }]
		return Promise.resolve({ host: registry.host, username: registry.username })
	}

	update(host: string, credentials: RegistryCredentials): Promise<Registry> {
		this.calls.update += 1
		this.registries = this.registries.map((r) =>
			r.host === host ? { host, ...credentials } : r,
		)
		return Promise.resolve({ host, username: credentials.username })
	}

	remove(host: string): Promise<void> {
		this.registries = this.registries.filter((r) => r.host !== host)
		return Promise.resolve()
	}
}
