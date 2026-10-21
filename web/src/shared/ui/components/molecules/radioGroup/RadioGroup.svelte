<script lang="ts" generics="T extends string">
	import type { ChoiceOption } from '../segmentedControl/types'
	import { directions, group, input, option } from './styles.css'

	// Radios nativos con el color del diseño ("Combinar con las existentes" / "Reemplazar
	// todas" al importar un .env).
	type Props = {
		label?: string
		labelledBy?: string
		name: string
		options: readonly ChoiceOption<T>[]
		value: T
		direction?: 'row' | 'column'
		disabled?: boolean
		onChange: (value: T) => void
	}

	let {
		label,
		labelledBy,
		name,
		options,
		value,
		direction = 'row',
		disabled = false,
		onChange,
	}: Props = $props()
</script>

<div
	role="radiogroup"
	aria-label={label}
	aria-labelledby={labelledBy}
	class={[group, directions[direction]]}
>
	{#each options as item (item.value)}
		<label class={option}>
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
