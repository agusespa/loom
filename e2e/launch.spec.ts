import { test, expect, type Page } from "@playwright/test";

/** Load the app and wait for the service worker to control the page. */
async function openApp(page: Page) {
  await page.goto("/");
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready;
    if (!navigator.serviceWorker.controller) {
      await new Promise((r) =>
        navigator.serviceWorker.addEventListener("controllerchange", r, {
          once: true,
        }),
      );
    }
  });
  await expect(page.getByRole("heading", { level: 1 })).not.toHaveText(
    "Loading…",
  );
}

test("launches from the cached shell and last week without the network", async ({
  page,
  context,
}) => {
  await openApp(page);
  const label = await page.locator(".week-label").textContent();

  await context.setOffline(true);
  await page.reload();

  await expect(page.locator(".week-label")).toHaveText(label!);
  await expect(
    page.getByRole("rowheader", { name: "Workout" }),
  ).toBeVisible();
  await context.setOffline(false);
});

test("sign-in navigations skip the cached shell and come back clean", async ({
  page,
}) => {
  await openApp(page);
  expect((await page.goto("/"))!.fromServiceWorker()).toBe(true);

  // "Sign in again" links here, so an expired Access session reaches its login page.
  expect((await page.goto("/?signin"))!.fromServiceWorker()).toBe(false);
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { level: 1 })).not.toHaveText(
    "Loading…",
  );
});
