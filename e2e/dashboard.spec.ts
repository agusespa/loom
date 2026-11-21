import { test, expect, type APIRequestContext } from "@playwright/test";

/** Write this week the way n8n would. */
async function seed(request: APIRequestContext) {
	const res = await request.get('/api/week');
	expect(res.ok()).toBe(true);
	const { start } = await res.json();
	const day = (n: number) => {
		const d = new Date(start + 'T00:00:00Z');
		d.setUTCDate(d.getUTCDate() + n);
		return d.toISOString().slice(0, 10);
	};
	const put = async (path: string, data: unknown) => {
		const r = await request.put(path, { data });
		expect(r.ok(), `${path}: ${r.status()}`).toBe(true);
	};
	await put(`/api/days/${day(0)}/workout`, { status: 'done', note: 'Upper-body' });
	await put(`/api/days/${day(0)}/dance`, { status: 'missed' });
	await put(`/api/days/${day(0)}/meditation`, { status: 'done', value: 12 });
	await put('/api/goals/short_term', {
		goals: [
			{ id: 's1', name: 'Bench 100 kg', priority: 'high', progress: 0.5, url: 'https://www.notion.so/s1' }
		]
	});
	await put('/api/goals/perennial', { goals: [] });
}

test('shows the week, its details and the goals', async ({ page, request }) => {
	await seed(request);
	await page.goto('/');

	await expect(page.getByRole('heading', { level: 1 })).not.toHaveText('Loading…');
	// One workout done, one dance missed.
	await expect(page.locator('.score .big')).toHaveText('50%');

	const workout = page.getByRole('row').filter({ has: page.getByRole('rowheader', { name: 'Workout' }) });
	await workout.getByRole('cell', { name: /Monday: Done/ }).click();
	await expect(page.getByText(/Workout · Done · Upper-body/)).toBeVisible();

	const goal = page.getByRole('link', { name: /Bench 100 kg/ });
	await expect(goal).toHaveAttribute('href', 'https://www.notion.so/s1');
	await expect(goal).toContainText('50%');
	await expect(page.getByText('No open goals.')).toBeVisible();

	// Last week has nothing, and there's no going past this week.
	await page.getByRole('button', { name: 'Previous week' }).click();
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('Quiet week');
	await page.getByRole('button', { name: 'Next week' }).click();
	await expect(page.getByText(/^Week \d+ · /)).toBeVisible();
	// Disabled nav buttons are hidden.
	await expect(page.locator('#next')).toBeHidden();
});
