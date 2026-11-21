import { test, expect, type Page } from "@playwright/test";

// The app only adds its own pull-to-refresh in the home-screen app, so pretend to be one.
test.beforeEach(async ({ page }) => {
	await page.addInitScript(() => Object.defineProperty(navigator, "standalone", { value: true }));
});

/** Drag a finger down from near the top of the page by `distance` px and let go. */
async function pull(page: Page, distance: number) {
	await page.evaluate((distance) => {
		// Plain events with a `touches` list: desktop WebKit has no Touch constructor.
		const fire = (type: string, y: number) => {
			const e = new Event(type, { bubbles: true, cancelable: true });
			const touch = { identifier: 1, clientX: 180, clientY: y };
			Object.defineProperty(e, "touches", { value: type === "touchend" ? [] : [touch] });
			document.body.dispatchEvent(e);
		};
		fire("touchstart", 100);
		for (let y = 110; y <= 100 + distance; y += 10) fire("touchmove", y);
		fire("touchend", 100 + distance);
	}, distance);
}

/** Marks the current page so a reload can be told apart from staying put. */
const mark = (page: Page) => page.evaluate(() => (document.body.dataset.marked = "yes"));
const marked = (page: Page) => page.evaluate(() => document.body.dataset.marked ?? null);

async function open(page: Page) {
	await page.goto("/");
	await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("Loading…");
}

test("pulling down far enough fetches the week again without reloading", async ({ page }) => {
	await open(page);
	// On last week, so the refresh has to keep where you are.
	await page.getByRole("button", { name: "Previous week" }).click();
	await expect(page.getByRole("button", { name: "Next week" })).toBeEnabled();
	await mark(page);

	const refetch = page.waitForRequest((r) => r.url().includes("/api/week?"));
	await pull(page, 220);
	await refetch;

	await expect(page.getByTestId("pull-to-refresh")).toHaveCSS("opacity", "0");
	expect(await marked(page)).toBe("yes");
	await expect(page.getByRole("button", { name: "Next week" })).toBeEnabled();
});

test("a short pull springs back without fetching", async ({ page }) => {
	await open(page);
	let fetched = false;
	page.on("request", (r) => r.url().includes("/api/week") && (fetched = true));

	await pull(page, 60);
	await expect(page.getByTestId("pull-to-refresh")).toHaveCSS("opacity", "0");
	expect(fetched).toBe(false);
});
