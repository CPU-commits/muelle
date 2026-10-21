<script lang="ts">
	import { avatar, sizes, type AvatarSize } from './styles.css'

	// La inicial del usuario en un círculo. Decorativo: el nombre va al lado, o en el
	// `aria-label` del botón que lo envuelve.
	let { name, size = 'md' }: { name: string; size?: AvatarSize } = $props()

	const initial = $derived(name.trim().charAt(0) || '?')
</script>

<span aria-hidden="true" class={[avatar, sizes[size]]}>{initial}</span>
