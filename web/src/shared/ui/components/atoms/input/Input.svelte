<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements'
	import { isInvalid } from '../control'
	import {
		control,
		controlSizes,
		invalid,
		mono as monoClass,
		type ControlSize,
	} from '../control.css'

	type Props = HTMLInputAttributes & {
		/** Alto: `sm` 32 (buscadores de lista), `md` 36 (por defecto), `lg` 40 (acceso). */
		inputSize?: ControlSize
		/** Geist Mono 13 px: imágenes, slugs, claves, IPs. */
		mono?: boolean
		value?: HTMLInputAttributes['value']
	}

	let {
		inputSize = 'md',
		mono = false,
		value = $bindable(),
		class: className,
		...rest
	}: Props = $props()
</script>

<input
	{...rest}
	bind:value
	class={[
		control,
		controlSizes[inputSize],
		mono && monoClass,
		isInvalid(rest['aria-invalid']) && invalid,
		className,
	]}
/>
