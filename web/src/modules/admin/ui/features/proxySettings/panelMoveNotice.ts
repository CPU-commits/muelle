// El aviso "La dirección del panel ahora es …" después de cambiar el puerto HTTPS: queda en
// el localStorage de este origen hasta que se cierra, así sobrevive a recargar. No es estado
// de la sesión ni un secreto (el origen del panel), y si el navegador no deja guardar, el
// aviso dura lo que dure la página.

import { parsePanelOrigin } from '../../../domain'

const STORAGE_KEY = 'muelle:admin:panel-moved'
const PREVIOUS_KEY = 'muelle:admin:panel-moved-from'

/**
 * El query param con que "Ir al panel" lleva el aviso a Ajustes del origen nuevo: el
 * localStorage de este origen no se ve desde allá.
 */
export const PANEL_MOVED_PARAM = 'panel_moved'

export interface PanelMove {
	/** A dónde se mudó el panel. */
	readonly origin: string
	/**
	 * Dónde estaba antes del cambio (contrato §3.5): a dónde volver si la nueva no responde.
	 * null si no se sabe (se llegó a la nueva con "Ir al panel").
	 */
	readonly previous: string | null
}

function storage(): Storage | null {
	try {
		return typeof localStorage === 'undefined' ? null : localStorage
	} catch {
		return null
	}
}

/** Un origen guardado, si pasa `parsePanelOrigin`; si no, null. */
function loadOrigin(key: string): string | null {
	try {
		const saved = storage()?.getItem(key)
		return saved ? parsePanelOrigin(saved) : null
	} catch {
		return null
	}
}

/**
 * El aviso guardado, si su origen es un origen `https://` con un dominio o una IP (nada de
 * rutas, otros esquemas ni caracteres raros en el host): va en el título del aviso y en el
 * link. La dirección anterior pasa el mismo filtro (va en el texto), o queda en null.
 */
export function loadPanelMove(): PanelMove | null {
	const origin = loadOrigin(STORAGE_KEY)
	return origin === null ? null : { origin, previous: loadOrigin(PREVIOUS_KEY) }
}

export function savePanelMove(origin: string, previous: string | null = null): void {
	try {
		const store = storage()
		store?.setItem(STORAGE_KEY, origin)
		// Sin la anterior, no queda la de un aviso de antes.
		if (previous === null) store?.removeItem(PREVIOUS_KEY)
		else store?.setItem(PREVIOUS_KEY, previous)
	} catch {
		// Sin almacenamiento (modo privado, bloqueado): el aviso queda solo en memoria.
	}
}

export function clearPanelMove(): void {
	try {
		storage()?.removeItem(STORAGE_KEY)
		storage()?.removeItem(PREVIOUS_KEY)
	} catch {
		// Nada que limpiar.
	}
}
