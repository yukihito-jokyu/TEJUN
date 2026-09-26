import { expect, test } from "@playwright/test";

test("アプリケーションの入口を表示する", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveTitle("TEJUN");
  await expect(page.getByRole("heading", { name: "TEJUN" })).toBeVisible();
  await expect(page.getByRole("main", { name: "作業領域" })).toBeVisible();
});
