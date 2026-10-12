<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements'
	import { box, boxInvalid, input, unit as unitClass } from './styles.css'

	// Un número con su unidad a la vista ("s", "MB"). La unidad es decorativa: el label
	// del campo ya la nombra ("Intervalo"), así que no la lee el lector de pantalla.
	type Props = HTMLInputAttributes & { unit: string }

	let { unit, ...rest }: Props = $props()
</script>

<div class={[box, rest['aria-invalid'] === true && boxInvalid]}>
	<input {...rest} class={input} />
	<span class={unitClass} aria-hidden="true">{unit}</span>
</div>
