import { render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";

import { AppHeader } from "./AppHeader";

it("両画面のブランドに同じiconを使い、初回セットアップだけbadgeを表示する", () => {
  const view = render(<AppHeader title="TEJUN" badge="初回セットアップ" />);
  const header = screen.getByRole("banner");
  expect(within(header).getByText("TEJUN")).toBeTruthy();
  expect(within(header).getByText("初回セットアップ")).toBeTruthy();
  expect(header.querySelector("[aria-hidden='true'] svg")).not.toBeNull();

  view.rerender(<AppHeader title="Procedure Studio" />);
  expect(within(header).getByText("Procedure Studio")).toBeTruthy();
  expect(within(header).queryByText("初回セットアップ")).toBeNull();
  expect(header.querySelector("[aria-hidden='true'] svg")).not.toBeNull();
});
