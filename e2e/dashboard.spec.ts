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
	await put(`/api/days/${day(0)}/workout`, { status: 'done' });
	await put(`/api/days/${day(0)}/dance`, { status: 'missed' });
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
	const menu = (name: string) => page.getByRole('menu', { name: `Set ${name}` });
	await expect(menu('Workout').getByRole('menuitemradio', { name: /Done/ })).toHaveAttribute('aria-checked', 'true');
	await page.keyboard.press('Escape');
	await expect(page.getByRole('menu')).toBeHidden();

	// The missed dance actually happened: fix it from the cell's menu.
	const dance = page.getByRole('row').filter({ has: page.getByRole('rowheader', { name: 'Dance' }) });
	await dance.getByRole('cell', { name: /Monday: Missed/ }).click();
	await menu('Dance').getByRole('menuitemradio', { name: /Done/ }).click();
	await expect(page.getByRole('menu')).toBeHidden();
	await expect(dance.getByRole('cell', { name: /Monday: Done/ })).toBeVisible();
	await expect(page.locator('.score .big')).toHaveText('100%');
	// And clear it again.
	await dance.getByRole('cell', { name: /Monday: Done/ }).click();
	await menu('Dance').getByRole('menuitemradio', { name: /Nothing/ }).click();
	await expect(dance.getByRole('cell', { name: /Monday: nothing/ })).toBeVisible();

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

	// The running release and its build time, as in Gympass and Dayspec.
	await expect(page.getByTestId('app-version')).toContainText(/Version dev\s*Built .*\d{4}/);

	// An error nothing handled shows as a toast instead of failing silently.
	await page.evaluate(() => setTimeout(() => Promise.reject(new Error('boom'))));
	await expect(page.getByRole('status').filter({ hasText: 'Something went wrong: boom' })).toBeVisible();
});
