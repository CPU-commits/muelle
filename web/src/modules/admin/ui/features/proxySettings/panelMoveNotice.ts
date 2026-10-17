// El aviso "La dirección del panel ahora es …" después de cambiar el puerto HTTPS: queda en
// el localStorage de este origen hasta que se cierra, así sobrevive a recargar. No es estado
// de la sesión ni un secreto (el origen del panel), y si el navegador no deja guardar, el
// aviso dura lo que dure la página.

import { parsePanelOrigin } from '../../../domain'

const STORAGE_KEY = 'muelle:admin:panel-moved'

/**
 * El query param con que "Ir al panel" lleva el aviso a Ajustes del origen nuevo: el
 * localStorage de este origen no se ve desde allá.
 */
export const PANEL_MOVED_PARAM = 'panel_moved'

function storage(): Storage | null {
	try {
		return typeof localStorage === 'undefined' ? null : localStorage
	} catch {
		return null
	}
}

/**
 * El origen guardado, si es un origen `https://` con un dominio o una IP (nada de rutas, otros
 * esquemas ni caracteres raros en el host): va en el título del aviso y en el link.
 */
export function loadPanelMove(): string | null {
	try {
		const saved = storage()?.getItem(STORAGE_KEY)
		return saved ? parsePanelOrigin(saved) : null
	} catch {
		return null
	}
}

export function savePanelMove(origin: string): void {
	try {
		storage()?.setItem(STORAGE_KEY, origin)
	} catch {
		// Sin almacenamiento (modo privado, bloqueado): el aviso queda solo en memoria.
	}
}

export function clearPanelMove(): void {
	try {
		storage()?.removeItem(STORAGE_KEY)
	} catch {
		// Nada que limpiar.
	}
}
