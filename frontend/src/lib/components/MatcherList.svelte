<script lang="ts">
	import type { MatcherEntry } from '$lib/api';
	import { ExternalLink } from 'lucide-svelte';

	interface Props {
		entries: MatcherEntry[];
		mode: 'matched' | 'failed';
	}

	let { entries, mode }: Props = $props();

	function timeAgo(unix: number): string {
		if (unix === 0) return 'Never';
		const seconds = Math.floor(Date.now() / 1000 - unix);
		if (seconds < 60) return `${seconds}s ago`;
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		return `${Math.floor(seconds / 86400)}d ago`;
	}

	function truncateName(name: string, max = 60): string {
		if (name.length <= max) return name;
		return name.slice(0, max).trimEnd() + '...';
	}

	function categoryColor(cat: string): string {
		switch (cat) {
			case 'movie':
				return 'bg-accent-blue/20 text-accent-blue';
			case 'tv':
				return 'bg-accent-green/20 text-accent-green';
			case 'anime':
				return 'bg-accent-purple/20 text-accent-purple';
			default:
				return 'bg-bg-tertiary text-text-secondary';
		}
	}

	function retryTime(matchAfter: number): string {
		if (matchAfter === 0) return 'Eligible now';
		const now = Math.floor(Date.now() / 1000);
		if (matchAfter <= now) return 'Eligible now';
		const diff = matchAfter - now;
		if (diff < 3600) return `${Math.floor(diff / 60)}m`;
		if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
		return `${Math.floor(diff / 86400)}d`;
	}
</script>

{#if entries.length === 0}
	<div class="py-8 text-center text-sm text-text-secondary">
		{mode === 'matched' ? 'No matched torrents yet' : 'No failed matches'}
	</div>
{:else}
	<div class="divide-y divide-border">
		{#each entries as entry (entry.info_hash)}
			<div class="flex items-center gap-3 px-4 py-3">
				<div class="min-w-0 flex-1">
					<div class="truncate text-sm font-medium text-text-primary" title={entry.name}>
						{truncateName(entry.name)}
					</div>
					<div class="mt-1 flex items-center gap-2">
						<span
							class="inline-flex rounded px-1.5 py-0.5 text-[10px] font-medium uppercase {categoryColor(entry.category)}"
						>
							{entry.category}
						</span>

						{#if mode === 'matched'}
							{#if entry.imdb_id}
								<a
									href="https://www.imdb.com/title/{entry.imdb_id}"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-0.5 text-[10px] text-accent-amber hover:underline"
								>
									IMDb<ExternalLink class="h-2.5 w-2.5" />
								</a>
							{/if}
							{#if entry.tmdb_id}
								<a
									href="https://www.themoviedb.org/{entry.category === 'movie' ? 'movie' : 'tv'}/{entry.tmdb_id}"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-0.5 text-[10px] text-accent-blue hover:underline"
								>
									TMDB<ExternalLink class="h-2.5 w-2.5" />
								</a>
							{/if}
							{#if entry.tvdb_id}
								<a
									href="https://thetvdb.com/?id={entry.tvdb_id}&tab=series"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-0.5 text-[10px] text-accent-green hover:underline"
								>
									TVDB<ExternalLink class="h-2.5 w-2.5" />
								</a>
							{/if}
							{#if entry.anilist_id}
								<a
									href="https://anilist.co/anime/{entry.anilist_id}"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-0.5 text-[10px] text-accent-purple hover:underline"
								>
									AniList<ExternalLink class="h-2.5 w-2.5" />
								</a>
							{/if}
						{:else}
							<span class="text-[10px] text-text-secondary">
								{entry.match_attempts} attempt{entry.match_attempts !== 1 ? 's' : ''}
							</span>
							<span class="text-[10px] text-text-secondary">
								Retry: {retryTime(entry.match_after)}
							</span>
						{/if}
					</div>
				</div>

				<div class="shrink-0 text-right text-[11px] text-text-secondary">
					{timeAgo(entry.updated_at)}
				</div>
			</div>
		{/each}
	</div>
{/if}
