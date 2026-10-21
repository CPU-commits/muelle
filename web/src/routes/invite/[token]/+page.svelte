<script lang="ts">
	import { InviteScreen } from '@/modules/auth'
	import type { PageProps } from './$types'

	// El link que arma `POST /invites`: `<panel>/invite/<token>`.
	let { params }: PageProps = $props()
</script>

<InviteScreen token={params.token} />
