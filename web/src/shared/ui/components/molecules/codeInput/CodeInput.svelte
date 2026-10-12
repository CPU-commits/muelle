<script lang="ts">
	import type { Attachment } from 'svelte/attachments'
	import {
		cell,
		cellActive,
		cellFilled,
		cells,
		disabled as disabledClass,
		input,
		invalid as invalidClass,
		root,
	} from './styles.css'

	// El código de 6 dígitos del diseño: seis celdas que en realidad son UN input. Así
	// pegar el código, el autocompletado del teléfono y el lector de pantalla funcionan
	// como en cualquier campo de texto.
	// `id`, `aria-invalid` y `aria-describedby` son los de <Field> (`{...a11y}`).
	type A11yProps = {
		id?: string
		'aria-invalid'?: boolean
		'aria-describedby'?: string | undefined
	}

	type Props = A11yProps & {
		value: string
		label: string
		length?: number
		disabled?: boolean
		/** Toma el foco al montarse (el paso del código, después de la contraseña). */
		focusOnMount?: boolean
		oninput: (value: string) => void
		/** Se llama cuando el código tiene todos sus dígitos. */
		oncomplete?: (value: string) => void
	}

	let {
		value,
		label,
		length = 6,
		disabled = false,
		focusOnMount = false,
		oninput,
		oncomplete,
		...a11y
	}: Props = $props()

	const isInvalid = $derived(a11y['aria-invalid'] === true)

	const slots = $derived(
		Array.from({ length }, (_, index) => ({ index, char: value.charAt(index) })),
	)
	const activeIndex = $derived(Math.min(value.length, length - 1))

	const focus: Attachment<HTMLInputElement> = (element) => {
		element.focus()
	}

	// Sin `maxlength`: el navegador cortaría "123 456" (pegado como lo muestran las apps)
	// en "123 45" antes de que se puedan sacar los espacios. Se limpia y se corta acá.
	function handleInput(event: Event & { currentTarget: HTMLInputElement }): void {
		const digits = event.currentTarget.value.replace(/\D/g, '').slice(0, length)
		event.currentTarget.value = digits
		oninput(digits)
		if (digits.length === length) oncomplete?.(digits)
	}
</script>

<div
	class={[root, isInvalid && invalidClass, disabled && disabledClass]}
	style:--code-length={length}
>
	<input
		{...a11y}
		class={input}
		type="text"
		inputmode="numeric"
		autocomplete="one-time-code"
		pattern="[0-9]*"
		aria-label={label}
		{disabled}
		{value}
		oninput={handleInput}
		{@attach focusOnMount && !disabled ? focus : undefined}
	/>
	<div class={cells} aria-hidden="true">
		{#each slots as slot (slot.index)}
			<span
				class={[
					cell,
					slot.char !== '' && cellFilled,
					slot.index === activeIndex && cellActive,
				]}>{slot.char}</span
			>
		{/each}
	</div>
</div>
