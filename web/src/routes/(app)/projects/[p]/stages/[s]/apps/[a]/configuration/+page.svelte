<script lang="ts">
	import { AppConfigScreen } from '@/modules/catalog'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<AppConfigScreen project={params.p} stage={params.s} app={params.a} />
