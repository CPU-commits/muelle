<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements'
	import { track } from './styles.css'

	// Sin label propio: va dentro de un <label> de la pantalla (título + descripción a la
	// izquierda, el switch a la derecha), o con `aria-label`.
	type Props = Omit<HTMLInputAttributes, 'type' | 'role'> & { checked?: boolean }

	let { checked = $bindable(false), class: className, ...rest }: Props = $props()
</script>

<input {...rest} type="checkbox" role="switch" class={[track, className]} bind:checked />
