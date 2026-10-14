<script lang="ts">
	import Modal from '../../modal/Modal.svelte'
	import CopyButton from '../CopyButton.svelte'

	// El modal del token nuevo (o de la URL de una invitación): el valor y su botón de
	// copiar adentro de un <dialog> modal, con el resto de la página inerte. `open` lo cierra
	// el test, como el botón "Listo" del dueño.
	let { value, open = true }: { value: string; open?: boolean } = $props()
</script>

<p>Tokens de API</p>

<Modal {open} title="Token creado" dismissible={false} onClose={() => {}}>
	<input id="nuevo-token" readonly {value} aria-label="Token" />
	<CopyButton {value} target="nuevo-token" />
</Modal>
