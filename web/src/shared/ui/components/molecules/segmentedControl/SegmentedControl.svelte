<script lang="ts" generics="T extends string">
	import { group, input, option, optionSelected } from './styles.css'
	import type { ChoiceOption } from './types'

	// Una opción entre pocas, a la vista (healthcheck: http · bun-http · command · none).
	// Son radios nativos: las flechas cambian la opción como en cualquier radiogroup.
	type Props = {
		/** Nombre del grupo para lectores de pantalla; o `labelledBy` con el id de un título. */
		label?: string
		labelledBy?: string
		name: string
		options: readonly ChoiceOption<T>[]
		value: T
		disabled?: boolean
		onChange: (value: T) => void
	}

	let { label, labelledBy, name, options, value, disabled = false, onChange }: Props = $props()
</script>

<div role="radiogroup" aria-label={label} aria-labelledby={labelledBy} class={group}>
	{#each options as item (item.value)}
		<label class={[option, item.value === value && optionSelected]}>
			<input
				class={input}
				type="radio"
				{name}
				value={item.value}
				checked={item.value === value}
				disabled={disabled || item.disabled}
				onchange={() => onChange(item.value)}
			/>
			{item.label}
		</label>
	{/each}
</div>
