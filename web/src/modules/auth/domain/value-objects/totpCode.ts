import type { Brand } from '@/shared/domain'
import { MalformedCode } from '../errors/authErrors'

export type TotpCode = Brand<string, 'TotpCode'>

export const TOTP_CODE_LENGTH = 6

// Las apps muestran el código como "123 456": los espacios no cuentan.
export function normalizeTotpCode(raw: string): string {
	return raw.replace(/\s+/g, '')
}

export function parseTotpCode(raw: string): TotpCode {
	const code = normalizeTotpCode(raw)
	if (!/^\d{6}$/.test(code)) throw new MalformedCode()
	return code as TotpCode
}
