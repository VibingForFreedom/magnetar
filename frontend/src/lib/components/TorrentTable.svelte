<script lang="ts">
	import { Copy, Check } from 'lucide-svelte';
	import type { TorrentResult } from '$lib/api';
	import { formatBytes, formatTimestamp } from '$lib/utils';

	interface Props {
		results: TorrentResult[];
	}

	let { results }: Props = $props();
	let copiedHash = $state<string | null>(null);

	function copyMagnet(hash: string, name: string) {
		const magnet = `magnet:?xt=urn:btih:${hash}&dn=${encodeURIComponent(name)}`;
		if (navigator.clipboard && window.isSecureContext) {
			navigator.clipboard.writeText(magnet);
		} else {
			const ta = document.createElement('textarea');
			ta.value = magnet;
			ta.style.position = 'fixed';
			ta.style.opacity = '0';
			document.body.appendChild(ta);
			ta.select();
			document.execCommand('copy');
			document.body.removeChild(ta);
		}
		copiedHash = hash;
		setTimeout(() => {
			copiedHash = null;
		}, 2000);
	}

	function categoryLabel(cat: string): string {
		switch (cat) {
			case 'movie': return 'Movie';
			case 'tv': return 'TV';
			case 'anime': return 'Anime';
			case 'unknown': return 'Other';
			default: return cat;
		}
	}

	function categoryColor(cat: string): string {
		switch (cat) {
			case 'movie': return 'bg-accent-blue/15 text-accent-blue';
			case 'tv': return 'bg-accent-green/15 text-accent-green';
			case 'anime': return 'bg-accent-purple/15 text-accent-purple';
			default: return 'bg-border/50 text-text-secondary';
		}
	}

	function qualityLabel(q: string): string {
		return q.toUpperCase();
	}

	function qualityColor(q: string): string {
		switch (q) {
			case 'uhd': return 'bg-accent-amber/15 text-accent-amber';
			case 'fhd': return 'bg-accent-green/15 text-accent-green';
			case 'hd': return 'bg-accent-blue/15 text-accent-blue';
			default: return 'bg-border text-text-secondary';
		}
	}
</script>

<div class="overflow-x-auto">
	<table class="w-full text-sm">
		<thead>
			<tr class="border-b border-border text-left text-xs uppercase tracking-wider text-text-secondary">
				<th class="px-4 py-3 font-medium">Name</th>
				<th class="px-4 py-3 font-medium">Size</th>
				<th class="px-4 py-3 font-medium">Category</th>
				<th class="px-4 py-3 font-medium">Quality</th>
				<th class="px-4 py-3 font-medium">Peers</th>
				<th class="px-4 py-3 font-medium">Discovered</th>
				<th class="px-4 py-3 font-medium w-10"></th>
			</tr>
		</thead>
		<tbody>
			{#each results as torrent (torrent.info_hash)}
				<tr class="border-b border-border/50 transition-colors hover:bg-bg-tertiary/30">
					<td class="max-w-md truncate px-4 py-3 text-text-primary" title={torrent.name}>
						{torrent.name}
					</td>
					<td class="whitespace-nowrap px-4 py-3 text-text-secondary">
						{formatBytes(torrent.size)}
					</td>
					<td class="px-4 py-3">
						<span class="inline-block rounded px-2 py-0.5 text-xs font-medium {categoryColor(torrent.category)}">
							{categoryLabel(torrent.category)}
						</span>
					</td>
					<td class="px-4 py-3">
						{#if torrent.quality && torrent.quality !== 'unknown'}
							<span class="inline-block rounded px-2 py-0.5 text-xs font-medium {qualityColor(torrent.quality)}">
								{qualityLabel(torrent.quality)}
							</span>
						{/if}
					</td>
					<td class="whitespace-nowrap px-4 py-3 text-text-secondary">
						<span class="{torrent.seeders > 0 ? 'text-accent-green' : ''}">{torrent.seeders}</span>
					</td>
					<td class="whitespace-nowrap px-4 py-3 text-text-secondary">
						{formatTimestamp(torrent.discovered_at)}
					</td>
					<td class="px-4 py-3">
						<button
							onclick={() => copyMagnet(torrent.info_hash, torrent.name)}
							class="rounded p-1 text-text-secondary transition-colors hover:bg-bg-tertiary hover:text-text-primary"
							title="Copy magnet link"
						>
							{#if copiedHash === torrent.info_hash}
								<Check class="h-4 w-4 text-accent-green" />
							{:else}
								<Copy class="h-4 w-4" />
							{/if}
						</button>
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
