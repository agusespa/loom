<script lang="ts">
	import { version } from '$app/env';
	import { replaceState } from '$app/navigation';
	import { onMount } from 'svelte';
	import { fetchWeek, putDay, type Status, type Week } from '#lib/api.ts';
	import { addDays, ago, delta, fmt, isoWeek, pct } from '#lib/dates.ts';
	import GoalList from '#lib/GoalList.svelte';
	import HabitGrid from '#lib/HabitGrid.svelte';
	import InsightCards from '#lib/InsightCards.svelte';
	import PullToRefresh from '#lib/PullToRefresh.svelte';
	import SwipeNav from '#lib/SwipeNav.svelte';

	// Everything comes from GET /api/week. n8n keeps it up to date; days can
	// also be corrected here.

	let week = $state<Week | null>(null);
	let current = $state<string | null>(null); // the Monday shown, null = this week
	let latestStart = $state<string | null>(null); // this week's Monday, from the server
	let banner = $state<'offline' | 'expired' | { status: number } | null>(null);
	let loading = $state(true);
	// The band is two pieces: the header (sticky) and the insight cards (scroll away). One gradient
	// is sized across both so they read as a single band.
	let topH = $state(0);
	let restH = $state(0);
	const bandH = $derived(topH + restH ? `${topH + restH}px` : null);

	// SvelteKit's build version is the build's timestamp; it tells two builds of one release apart.
	const builtMs = Number(version);
	const builtAt = Number.isFinite(builtMs)
		? new Date(builtMs).toLocaleString(undefined, {
				day: 'numeric',
				month: 'short',
				year: 'numeric',
				hour: 'numeric',
				minute: '2-digit'
			})
		: null;

	const isCurrent = $derived(!!week && (current === null || week.start === latestStart));

	// Weeks already seen, by Monday, for this visit only: going back to one shows it at once while
	// load() fetches it again in the background.
	const seen = new Map<string, Week>();
	// Counts loads, so only the newest one is applied: tapping through weeks quickly has several
	// in flight, and an older answer arriving last mustn't overwrite the week you're on.
	let loads = 0;

	// This week as last loaded, shown at launch while load() asks the server for the latest.
	const SAVED = 'loom:week';
	function restore() {
		try {
			const saved = JSON.parse(localStorage.getItem(SAVED) ?? 'null') as Week | null;
			if (!saved || week) return;
			latestStart = saved.start;
			week = saved;
			loading = false;
		} catch {
			localStorage.removeItem(SAVED);
		}
	}

	async function load() {
		const n = ++loads;
		const res = await fetchWeek(current);
		if (n !== loads) return;
		loading = false;
		if (res.kind === 'ok') {
			if (current === null) {
				latestStart = res.week.start;
				localStorage.setItem(SAVED, JSON.stringify(res.week));
			}
			seen.set(res.week.start, res.week);
			week = res.week;
			banner = null;
		} else if (res.kind === 'error') {
			banner = { status: res.status };
		} else {
			banner = res.kind;
		}
	}

	async function edit(habit: string, date: string, status: Status | null): Promise<string | null> {
		const res = await putDay(habit, date, status);
		if (res.kind === 'ok') {
			await load();
			return null;
		}
		if (res.kind === 'expired') {
			banner = 'expired';
			return 'Sign in again to save.';
		}
		return res.kind === 'offline' ? "Couldn't reach Loom." : res.message;
	}

	function go(delta: number): Promise<void> {
		const base = current ?? latestStart;
		if (!base) return Promise.resolve();
		const next = addDays(base, delta * 7);
		current = latestStart && next >= latestStart ? null : next;
		const cached = seen.get(current ?? latestStart!);
		const fresh = load();
		if (!cached) return fresh;
		week = cached;
		return Promise.resolve();
	}

	/** Change on last week in percentage points, or null when there's nothing to compare. */
	type Tier = 'great' | 'good' | 'mixed' | 'bad' | 'failed';

	/** The week's grade: it picks both the headline and the score's color, so the two always agree. */
	function tier(adherence: number): Tier {
		if (adherence >= 0.85) return 'great';
		if (adherence >= 0.6) return 'good';
		if (adherence >= 0.4) return 'mixed';
		if (adherence > 0) return 'bad';
		return 'failed';
	}

	const HEADLINES: Record<Tier, [now: string, past: string]> = {
		great: ['Great week so far', 'Great week'],
		good: ['Good week so far', 'Good week'],
		mixed: ['Slipping behind', 'Mixed week'],
		bad: ['Badly behind', 'Bad week'],
		failed: ['Nothing done so far', 'Total failure']
	};

	function headline(w: Week): string {
		const { done, missed, planned, adherence } = w.summary;
		const now = isCurrent;
		if (done + missed + planned === 0) return now ? 'Nothing planned yet' : 'Quiet week';
		if (adherence === null) return planned ? `${planned} planned` : 'Week ahead';
		return HEADLINES[tier(adherence)][now ? 0 : 1];
	}

	onMount(() => {
		// Back from the Access login: drop the marker that let the launch skip the cached shell.
		if (location.search === '?signin') replaceState('/', {});
		restore();
		load();
		const onVisible = () => document.visibilityState === 'visible' && load();
		document.addEventListener('visibilitychange', onVisible);
		return () => document.removeEventListener('visibilitychange', onVisible);
	});
</script>

<PullToRefresh onrefresh={load} />

<main>
	<div class="band band-top" style:--band-h={bandH} bind:offsetHeight={topH}>
		<header class="top">
			<div class="top-mid">
				<div class="week-nav muted small">
					<button class="nav" id="prev" aria-label="Previous week" disabled={!week} onclick={() => go(-1)}
						><svg class="chev" viewBox="0 0 9 14" aria-hidden="true"><path d="M7.5 1.5 2 7l5.5 5.5" /></svg
						></button
					>
					<span class="week-label">
						{#if week}
							Week {isoWeek(week.start)} · {fmt(week.start, { day: 'numeric', month: 'short' })}–{fmt(
								week.end,
								{ day: 'numeric', month: 'short' }
							)}
						{:else}&nbsp;{/if}
					</span>
					<button
						class="nav"
						id="next"
						aria-label="Next week"
						disabled={!week || isCurrent}
						onclick={() => go(1)}
						><svg class="chev" viewBox="0 0 9 14" aria-hidden="true"><path d="M1.5 1.5 7 7l-5.5 5.5" /></svg
						></button
					>
				</div>
				<h1>{week ? headline(week) : loading ? 'Loading…' : 'Loom'}</h1>
			</div>
			<!-- No score yet (nothing planned done or missed): no score card at all. -->
			{#if week && week.summary.adherence !== null}
				{@const a = week.summary.adherence}
				{@const d = delta(a, week.prev_adherence)}
				<div class="score">
					<div class="pill {tier(a)}">
						<span class="num">{Math.round(a * 100)}<span class="unit">%</span></span>
						{#if d !== null}
							<span
								class="delta"
								class:up={d > 0}
								class:down={d < 0}
								title="vs {pct(week.prev_adherence)} last week"
								aria-label="{d > 0 ? 'Up' : d < 0 ? 'Down' : 'Level'} {Math.abs(d)} points on last week"
							>
								<span class="arrow" aria-hidden="true">{d > 0 ? '▲' : d < 0 ? '▼' : '='}</span>{d === 0
									? ''
									: Math.abs(d)}
							</span>
						{/if}
					</div>
				</div>
			{/if}
		</header>
	</div>
	{#if week}
		<div class="band band-rest" style:--band-h={bandH} bind:offsetHeight={restH}>
			<InsightCards insights={week.insights} />
		</div>
	{/if}

	{#if banner}
		<div class="banner" role="status">
			{#if banner === 'offline'}
				Couldn't reach Loom. Check your connection.
			{:else if banner === 'expired'}
				Your session expired. <a href="/?signin">Sign in again</a>
			{:else}
				Couldn't load this week (HTTP {banner.status}).
			{/if}
		</div>
	{/if}

	{#if week}
		<SwipeNav canPrev={!!week} canNext={!!week && !isCurrent} onswipe={go}>
			<section>
				<div class="sec"><span>Habits</span></div>
				<HabitGrid habits={week.habits} today={week.today} {isCurrent} onedit={edit} />
			</section>

			<section>
				<div class="sec"><span>Short-term goals</span></div>
				<GoalList goals={week.goals.short_term} today={week.today} />
			</section>

			<section>
				<div class="sec"><span>Perennial goals</span></div>
				<GoalList goals={week.goals.perennial} today={week.today} />
			</section>
		</SwipeNav>
	{/if}

	<!-- Always shown, so you can tell which build is installed even when loading fails. -->
	<footer class="muted small">
		{#if week}<p>{week.updated_at ? `Updated ${ago(week.updated_at)}` : 'Nothing logged yet'}</p>{/if}
		<p class="version" data-testid="app-version">
			Version {__APP_VERSION__}{#if builtAt}<br />Built {builtAt}{/if}
		</p>
	</footer>
</main>
