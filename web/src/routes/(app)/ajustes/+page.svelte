<script lang="ts">
	import { page } from '$app/state'
	import { PANEL_MOVED_PARAM, SettingsScreen } from '@/modules/admin'

	// `?panel_moved=1`: se llegó con "Ir al panel" después de cambiar el puerto HTTPS en la
	// dirección anterior (Proxy y DNS lo avisa acá también).
</script>

<SettingsScreen panelMoved={page.url.searchParams.has(PANEL_MOVED_PARAM)} />
