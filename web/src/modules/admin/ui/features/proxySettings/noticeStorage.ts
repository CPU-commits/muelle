// Los avisos de "Puertos del proxy" que sobreviven a recargar (la dirección nueva del panel, la
// vuelta a los puertos anteriores que ya se cerró) van en el localStorage de este origen: no
// son estado de la sesión ni secretos. Si el navegador no deja guardar (modo privado,
// bloqueado), duran lo que dure la página. Lo leído viene de afuera: quien lo lee lo valida.

function storage(): Storage | null {
	try {
		return typeof localStorage === 'undefined' ? null : localStorage
	} catch {
		return null
	}
}

export function readNotice(key: string): string | null {
	try {
		return storage()?.getItem(key) ?? null
	} catch {
		return null
	}
}

export function writeNotice(key: string, value: string): void {
	try {
		storage()?.setItem(key, value)
	} catch {
		// Sin almacenamiento: el aviso queda solo en memoria.
	}
}

export function removeNotice(key: string): void {
	try {
		storage()?.removeItem(key)
	} catch {
		// Nada que limpiar.
	}
}
