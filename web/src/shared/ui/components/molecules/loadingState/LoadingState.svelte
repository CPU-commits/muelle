<script lang="ts">
	import Spinner from '../../atoms/spinner/Spinner.svelte'
	import { row } from './styles.css'

	let { label }: { label: string } = $props()
</script>

<!-- Un solo status con el texto; el spinner es decorativo (si no, se anuncia dos veces). -->
<div class={row} role="status">
	<Spinner decorative />
	<span>{label}</span>
</div>
