import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SetupPage } from "./SetupPage";

afterEach(cleanup);

describe("SetupPage", () => {
  it("読み込み中はフォームを表示しない", () => {
    render(
      <SetupPage
        loading
        candidates={[]}
        status="ready"
        onRefresh={vi.fn()}
        onConnect={vi.fn()}
        onAuthenticate={vi.fn()}
        onContinue={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole("status").textContent).toContain("読み込んでいます");
    expect(screen.queryByRole("button", { name: "接続" })).toBeNull();
  });
});
