<script lang="ts">
	import type { Snippet } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		field,
		label as labelClass,
		message,
		messageError,
		optional as optionalClass,
	} from './styles.css'
	import type { FieldA11y } from './types'

	type Props = {
		label: string
		/** Texto de ayuda bajo el control. El error, si hay, lo reemplaza. */
		hint?: string | null
		error?: string | null
		/** Agrega "· opcional" al label. */
		optional?: boolean
		children: Snippet<[FieldA11y]>
	}

	let { label, hint = null, error = null, optional = false, children }: Props = $props()

	const id = $props.id()
	const text = $derived(error ?? hint)
</script>

<div class={field}>
	<label for={id} class={labelClass}>
		{label}
		{#if optional}
			<span class={optionalClass}>· {m.common_optional()}</span>
		{/if}
	</label>
	{@render children({
		id,
		'aria-invalid': error !== null,
		'aria-describedby': text ? `${id}-desc` : undefined,
	})}
	{#if text}
		<p id={`${id}-desc`} class={[message, error !== null && messageError]}>{text}</p>
	{/if}
</div>
