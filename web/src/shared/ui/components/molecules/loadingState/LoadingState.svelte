<script lang="ts">
	import Spinner from '../../atoms/spinner/Spinner.svelte'
	import { row } from './styles.css'

	let { label }: { label: string } = $props()
</script>

<div class={row}>
	<Spinner {label} />
	<span>{label}</span>
</div>
