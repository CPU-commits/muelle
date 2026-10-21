import { beforeNavigate } from '$app/navigation'
import { m } from '@/shared/ui/i18n/paraglide/messages'

/**
 * Pide confirmación antes de salir con cambios sin guardar: un tab, el breadcrumb, un link
 * o "atrás" (`window.confirm`, que frena la navegación hasta que se responde). Al recargar
 * o cerrar la pestaña, `cancel()` hace que el navegador muestre su propio aviso: es el
 * `beforeunload` de SvelteKit. Los `goto` del propio panel (después de borrar la app) no
 * preguntan. Se llama durante la inicialización del componente (en el viewModel).
 */
export function guardUnsavedChanges(isDirty: () => boolean): void {
	beforeNavigate((navigation) => {
		if (navigation.type === 'goto' || !isDirty()) return
		if (navigation.willUnload) {
			navigation.cancel()
			return
		}
		if (!window.confirm(m.catalog_unsaved_confirm())) navigation.cancel()
	})
}
