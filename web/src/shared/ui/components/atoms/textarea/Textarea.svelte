<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements'
	import { isInvalid } from '../control'
	import { control, invalid, mono as monoClass, multiline } from '../control.css'

	type Props = HTMLTextareaAttributes & {
		/** Geist Mono: `.env`, CIDRs, YAML. */
		mono?: boolean
		value?: HTMLTextareaAttributes['value']
	}

	let { mono = false, value = $bindable(), class: className, ...rest }: Props = $props()
</script>

<textarea
	{...rest}
	bind:value
	class={[
		control,
		multiline,
		mono && monoClass,
		isInvalid(rest['aria-invalid']) && invalid,
		className,
	]}
></textarea>
