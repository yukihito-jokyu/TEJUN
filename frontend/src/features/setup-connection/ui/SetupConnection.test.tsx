import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentCandidate, SetupConnectionProps } from "./SetupConnection";
import { SetupConnection } from "./SetupConnection";

const candidate: AgentCandidate = {
  candidateKey: "codex",
  displayName: "Codex ACP",
  command: "codex-acp",
  args: [],
  transport: "stdio",
  source: "path",
  warnings: [],
};

afterEach(cleanup);

function props(overrides: Partial<SetupConnectionProps> = {}): SetupConnectionProps {
  return {
    candidates: [candidate],
    status: "ready",
    onRefresh: vi.fn(),
    onConnect: vi.fn(),
    onAuthenticate: vi.fn(),
    onContinue: vi.fn(),
    onRetry: vi.fn(),
    ...overrides,
  };
}

describe("SetupConnection", () => {
  it("設定項目を表示せず候補へ接続する", () => {
    const input = props();
    render(<SetupConnection {...input} />);

    fireEvent.click(screen.getByRole("button", { name: "接続" }));

    expect(input.onConnect).toHaveBeenCalledWith(candidate);
    expect(screen.queryByLabelText("コマンド")).toBeNull();
    expect(screen.queryByText("接続設定を編集")).toBeNull();
  });

  it("必要な場合だけ認証方法を表示する", () => {
    const input = props({
      selected: { ...candidate, environmentOverrides: [] },
      authState: "required",
      authMethods: [
        { type: "agent", authMethodId: "oauth", name: "ブラウザ", environmentNames: [] },
        { type: "terminal", authMethodId: "login", name: "CLI", environmentNames: [] },
      ],
    });
    render(<SetupConnection {...input} />);

    fireEvent.click(screen.getByRole("button", { name: "認証する" }));
    fireEvent.click(screen.getByRole("button", { name: "ターミナルで認証" }));

    expect(input.onAuthenticate).toHaveBeenNthCalledWith(1, "oauth");
    expect(input.onAuthenticate).toHaveBeenNthCalledWith(2, "login");
  });

  it("回復可能なエラーから再試行できる", () => {
    const input = props({
      error: { code: "acp_not_compatible", message: "protocol mismatch", retryable: true },
    });
    render(<SetupConnection {...input} />);

    fireEvent.click(screen.getByRole("button", { name: "再試行" }));

    expect(input.onRetry).toHaveBeenCalledOnce();
  });

  it("接続中は操作を無効にする", () => {
    render(
      <SetupConnection
        {...props({ selected: { ...candidate, environmentOverrides: [] }, status: "saving" })}
      />,
    );

    expect((screen.getByRole("button", { name: "接続" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByRole("status").textContent).toContain("保存しています");
  });

  it("接続完了後は認証欄に遷移ボタンを表示する", () => {
    const input = props({ connected: true });
    render(<SetupConnection {...input} />);

    fireEvent.click(screen.getByRole("button", { name: /プロジェクト一覧へ/ }));

    expect(screen.getByText("エージェントを接続しました")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "認証" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "使用するエージェント" })).toBeTruthy();
    expect(input.onContinue).toHaveBeenCalledOnce();
  });
});
