<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { HTMLSelectAttributes } from 'svelte/elements'
	import { isInvalid } from '../control'
	import {
		control,
		controlSizes,
		invalid,
		select as selectClass,
		type ControlSize,
	} from '../control.css'

	type Props = HTMLSelectAttributes & {
		inputSize?: ControlSize
		value?: HTMLSelectAttributes['value']
		children: Snippet
	}

	let {
		inputSize = 'md',
		value = $bindable(),
		class: className,
		children,
		...rest
	}: Props = $props()
</script>

<select
	{...rest}
	bind:value
	class={[
		control,
		controlSizes[inputSize],
		selectClass,
		isInvalid(rest['aria-invalid']) && invalid,
		className,
	]}
>
	{@render children()}
</select>
