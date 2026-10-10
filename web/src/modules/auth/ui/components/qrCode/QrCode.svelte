<script lang="ts">
	import { qrPath } from './qrPath'
	import { frame, modules } from './styles.css'

	// La URL `otpauth://` como QR. Oscuro sobre blanco aunque el panel sea oscuro: no
	// todos los lectores leen un QR invertido.
	let { value, label }: { value: string; label: string } = $props()

	const qr = $derived(qrPath(value))
</script>

<svg
	class={frame}
	viewBox={`0 0 ${qr.size} ${qr.size}`}
	role="img"
	aria-label={label}
	shape-rendering="crispEdges"
>
	<path class={modules} d={qr.d} />
</svg>
