<script lang="ts">
	import type { Goal } from './api.ts';
	import { fmt, pct } from './dates.ts';

	let { goals, today }: { goals: Goal[]; today: string } = $props();

	function dueLabel(due: string): string {
		const sameYear = due.slice(0, 4) === today.slice(0, 4);
		return fmt(due, { day: 'numeric', month: 'short', year: sameYear ? undefined : 'numeric' });
	}
</script>

{#if goals.length === 0}
	<div class="empty-list">No open goals.</div>
{:else}
	<div class="grid">
		{#each goals as g (g.id)}
			{@const overdue = !!g.due && g.due < today && (g.progress ?? 0) < 1}
			<a class="goal" href={g.url} target="_blank" rel="noopener">
				<div class="goal-top">
					<span class="goal-name">
						{#if g.priority}<span class="prio {g.priority}" title="{g.priority} priority"></span>{/if}{g.name}
					</span>
					<span class="goal-meta">
						{#if g.progress !== null}{pct(g.progress)}{/if}
						{#if g.progress !== null && g.due}&nbsp;·&nbsp;{/if}
						{#if g.due}<span class:overdue>{dueLabel(g.due)}</span>{/if}
					</span>
				</div>
				<div class="bar"><span style:width="{Math.round(Math.min(1, g.progress ?? 0) * 100)}%"></span></div>
			</a>
		{/each}
	</div>
{/if}
