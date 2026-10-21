<script lang="ts">
	import CopyButton from '../CopyButton.svelte'

	// El token recién creado: un input de solo lectura y su botón de copiar.
	let { value }: { value: string } = $props()
</script>

<input id="token" readonly {value} aria-label="Token" />
<CopyButton {value} target="token" />
