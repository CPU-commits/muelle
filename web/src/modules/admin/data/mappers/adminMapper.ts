import type {
	AuditActor,
	AuditEvent,
	AuditEventId,
	NewRegistry,
	PanelAccess,
	ProxySettings,
	Registry,
	RegistryCredentials,
	Settings,
	SettingsPatch,
} from '../../domain'
import type {
	AuditEventDto,
	NewRegistryPayload,
	RegistryCredentialsPayload,
	RegistryDto,
	SettingsDto,
	SettingsPatchPayload,
} from '../dtos/adminDto'

const DEFAULT_HTTP_PORT = 80
const DEFAULT_HTTPS_PORT = 443

// Los campos de la Fase 3 llegan juntos. `public_ip` marca si el backend ya la tiene: sin
// él la sección es `null` y la pantalla muestra "Llega con la Fase 3".
function proxyFromDto(dto: SettingsDto): ProxySettings | null {
	if (dto.public_ip === undefined) return null
	return {
		cloudflare: {
			tokenSet: dto.cloudflare_token_set ?? false,
			zones: dto.cloudflare_zones ?? null,
		},
		publicIpv4: dto.public_ip,
		publicIpv6: dto.public_ipv6 ?? '',
		acmeEmail: dto.acme_email ?? '',
		panelDomain: dto.panel_domain ?? '',
		httpPort: dto.proxy_http_port ?? DEFAULT_HTTP_PORT,
		httpsPort: dto.proxy_https_port ?? DEFAULT_HTTPS_PORT,
	}
}

function panelAccessFromDto(dto: SettingsDto): PanelAccess | null {
	if (dto.panel_allowed_cidrs === undefined) return null
	return { allowedCidrs: dto.panel_allowed_cidrs, clientIp: dto.client_ip ?? '' }
}

export function settingsFromDto(dto: SettingsDto): Settings {
	return {
		require2fa: dto.require_2fa,
		proxy: proxyFromDto(dto),
		panelAccess: panelAccessFromDto(dto),
	}
}

/** Solo las claves presentes: el backend rechaza las que no conoce (json/v2 estricto). */
export function settingsPatchPayload(patch: SettingsPatch): SettingsPatchPayload {
	return {
		...(patch.require2fa !== undefined ? { require_2fa: patch.require2fa } : {}),
		...(patch.publicIpv4 !== undefined ? { public_ip: patch.publicIpv4 } : {}),
		...(patch.publicIpv6 !== undefined ? { public_ipv6: patch.publicIpv6 } : {}),
		...(patch.acmeEmail !== undefined ? { acme_email: patch.acmeEmail } : {}),
		...(patch.httpPort !== undefined ? { proxy_http_port: patch.httpPort } : {}),
		...(patch.httpsPort !== undefined ? { proxy_https_port: patch.httpsPort } : {}),
		...(patch.panelAllowedCidrs !== undefined
			? { panel_allowed_cidrs: [...patch.panelAllowedCidrs] }
			: {}),
		...(patch.cloudflareToken !== undefined ? { cloudflare_token: patch.cloudflareToken } : {}),
		...(patch.confirmLockout ? { confirm_lockout: true } : {}),
	}
}

function actorFromDto(dto: AuditEventDto): AuditActor {
	const name = dto.actor_name ?? ''
	switch (dto.actor_type) {
		case 'user':
			return { kind: 'user', id: dto.actor_id, name }
		case 'token':
			return { kind: 'token', id: dto.actor_id, name }
		case 'system':
			return { kind: 'system' }
		default:
			return { kind: 'other', type: dto.actor_type }
	}
}

export function auditEventFromDto(dto: AuditEventDto): AuditEvent {
	return {
		id: dto.id as AuditEventId,
		actor: actorFromDto(dto),
		action: dto.action,
		target: dto.target,
		at: new Date(dto.at),
	}
}

export function registryFromDto(dto: RegistryDto): Registry {
	return {
		host: dto.host,
		username: dto.username,
		createdAt: new Date(dto.created_at),
		updatedAt: new Date(dto.updated_at),
	}
}

export function newRegistryPayload(registry: NewRegistry): NewRegistryPayload {
	return { host: registry.host, username: registry.username, password: registry.password }
}

export function registryCredentialsPayload(
	credentials: RegistryCredentials,
): RegistryCredentialsPayload {
	return { username: credentials.username, password: credentials.password }
}
