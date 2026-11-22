import { test, expect, type Page } from "@playwright/test";

const label = (page: Page) => page.locator(".week-label");
const prev = (page: Page) =>
  page.getByRole("button", { name: "Previous week" });

async function open(page: Page) {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).not.toHaveText(
    "Loading…",
  );
}

/** Holds answers for past weeks until the returned release() is called. */
async function holdPastWeeks(page: Page, only = Infinity) {
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  let held = 0;
  await page.route("**/api/week?start=*", async (route) => {
    if (held++ < only) await gate;
    await route.continue();
  });
  return release;
}

test("going back to a week already seen shows it before the server answers", async ({
  page,
}) => {
  await open(page);
  const thisWeek = await label(page).textContent();
  await prev(page).click();
  await expect(label(page)).not.toHaveText(thisWeek!);
  const lastWeek = await label(page).textContent();
  await page.getByRole("button", { name: "Next week" }).click();
  await expect(label(page)).toHaveText(thisWeek!);

  const release = await holdPastWeeks(page);
  const refetch = page.waitForRequest((r) => r.url().includes("/api/week?"));
  await prev(page).click();
  // Shown from the cache while its fetch is still held...
  await expect(label(page)).toHaveText(lastWeek!, { timeout: 1000 });
  // ...and fetched again all the same.
  await refetch;
  release();
});

test("a slow answer for a week you've left doesn't replace the one you're on", async ({
  page,
}) => {
  await open(page);
  const thisWeek = await label(page).textContent();
  // Hold only the first: last week's answer arrives after the week before's.
  const release = await holdPastWeeks(page, 1);

  await prev(page).click();
  await prev(page).click();
  await expect(label(page)).not.toHaveText(thisWeek!);
  const twoBack = await label(page).textContent();

  const late = page.waitForResponse((r) => r.url().includes("/api/week?"));
  release();
  await late;
  await page.evaluate(() => new Promise(requestAnimationFrame));
  await expect(label(page)).toHaveText(twoBack!);
});
