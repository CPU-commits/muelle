<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { HTMLInputAttributes } from 'svelte/elements'
	import { box, label as labelClass } from './styles.css'

	type Props = Omit<HTMLInputAttributes, 'type'> & {
		/** Texto al lado de la casilla. Si no hay, pasale `aria-label`. */
		label?: string
		checked?: boolean
		children?: Snippet
	}

	let { label, checked = $bindable(false), class: className, children, ...rest }: Props = $props()
</script>

<label class={[labelClass, className]}>
	<input {...rest} type="checkbox" class={box} bind:checked />
	{#if children}
		{@render children()}
	{:else if label}
		<span>{label}</span>
	{/if}
</label>
