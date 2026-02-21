<script lang="ts">
	import '../app.css';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import { initMetricsStream, stopMetricsStream } from '$lib/stores/metrics';
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	onMount(() => {
		initMetricsStream();
		return stopMetricsStream;
	});
</script>

<div class="flex min-h-screen">
	<Sidebar />
	<main class="ml-56 flex-1 p-6">
		{@render children()}
	</main>
</div>
