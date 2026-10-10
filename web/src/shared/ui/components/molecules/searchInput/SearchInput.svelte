<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements'
	import Icon from '../../atoms/icon/Icon.svelte'
	import Input from '../../atoms/input/Input.svelte'
	import type { ControlSize } from '../../atoms/control.css'
	import { icon, input, root } from './styles.css'

	// El buscador del diseño (lupa a la izquierda). `label` es el `aria-label`: el
	// placeholder no cuenta como nombre accesible.
	type Props = Omit<HTMLInputAttributes, 'type'> & {
		label: string
		inputSize?: ControlSize
		value?: string
	}

	let {
		label,
		inputSize = 'md',
		value = $bindable(''),
		class: className,
		...rest
	}: Props = $props()
</script>

<div class={[root, className]}>
	<span class={icon}><Icon name="search" size={inputSize === 'sm' ? 15 : 16} /></span>
	<Input {...rest} type="search" aria-label={label} {inputSize} class={input} bind:value />
</div>
