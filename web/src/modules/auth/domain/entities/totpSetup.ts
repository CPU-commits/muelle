/** Un secreto TOTP pendiente: queda activo cuando se confirma con un código. */
export interface TotpSetup {
	/** Base32, para escribirlo a mano en la app. */
	readonly secret: string
	/** `otpauth://totp/…`: lo que codifica el QR. */
	readonly otpauthUrl: string
}

/** "JBSWY3DPEHPK3PXP" → "JBSW Y3DP EHPK 3PXP": más fácil de copiar a mano. */
export function groupSecret(secret: string, size = 4): string {
	const clean = secret.replace(/\s+/g, '')
	const groups: string[] = []
	for (let index = 0; index < clean.length; index += size) {
		groups.push(clean.slice(index, index + size))
	}
	return groups.join(' ')
}
