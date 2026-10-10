import type { Settings, SettingsPatch } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'
import { parseCidrList } from '../value-objects/cidr'

/** La tarjeta "Seguridad": solo van los campos que cambiaron. */
export interface UpdateSecurityInput {
	require2fa?: boolean
	/** El textarea de "IPs permitidas", un rango por línea. */
	allowedCidrs?: string
}

/**
 * Activar "Exigir 2FA" sin tener el propio 2FA responde `TotpRequiredFirst`: quien lo
 * pide quedaría encerrado en la configuración.
 */
export type UpdateSecurity = (input: UpdateSecurityInput) => Promise<Settings>

export function makeUpdateSecurity({ settings }: SettingsDeps): UpdateSecurity {
	return async ({ require2fa, allowedCidrs }) => {
		const patch: SettingsPatch = {
			...(require2fa !== undefined ? { require2fa } : {}),
			...(allowedCidrs !== undefined
				? { panelAllowedCidrs: parseCidrList(allowedCidrs) }
				: {}),
		}
		return settings.patch(patch)
	}
}
