import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { AIUpdateRipple } from "./AIUpdateRipple";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("波紋の開始時だけUIを屈折させ、終了時に元へ戻す", () => {
  const { container } = render(
    <main>
      <AIUpdateRipple />
      <div className="preparation-ai-update">目的</div>
    </main>,
  );
  vi.spyOn(window, "getComputedStyle").mockReturnValue({
    animationDuration: "0.96s",
  } as CSSStyleDeclaration);
  const target = container.querySelector("div")!;
  const animation = container.querySelector("animate")!;
  const begin = vi.fn();
  Object.defineProperty(animation, "beginElement", { value: begin });
  const start = Object.assign(new Event("animationstart", { bubbles: true }), {
    animationName: "ai-update-ripple",
    pseudoElement: "::after",
  });
  target.dispatchEvent(start);
  expect(target.style.filter).toContain("#");
  expect(begin).toHaveBeenCalledOnce();
  target.dispatchEvent(
    Object.assign(new Event("animationend", { bubbles: true }), {
      animationName: "ai-update-ripple",
    }),
  );
  expect(target.style.filter).toBe("");
});
