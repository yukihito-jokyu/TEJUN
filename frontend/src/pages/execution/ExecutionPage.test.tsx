import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ExecutionPage } from "./ExecutionPage";
import type { ExecutionView } from "@/shared/api/wails/execution";

const view = {
  project: { name: "Example" },
  execution: { executionId: "e", revision: 1 },
  checks: [
    {
      checkId: "c",
      sequence: 1,
      title: "起動",
      instruction: "起動する",
      expectedResult: "成功",
      ai: { required: true, status: "completed", checked: true, failureSummary: "" },
      human: { required: true, status: "pending", checked: false },
      humanEvidenceRequirement: "説明",
      evidence: { ai: [], human: [] },
    },
  ],
  conversation: { items: [], hasPrevious: false },
  pendingPermissions: [],
  readiness: {
    canGenerateProcedure: false,
    blockingReasons: [{ code: "evidence", message: "証跡が必要" }],
  },
} as unknown as ExecutionView;

afterEach(cleanup);
describe("ExecutionPage", () => {
  it("送信中に編集した文面を保持する", async () => {
    let finish!: (ok: boolean) => void;
    const onMessage = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    render(
      <ExecutionPage
        view={view}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onMessage={onMessage}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    const input = screen.getByLabelText("AIにメッセージを送る") as HTMLTextAreaElement;
    fireEvent.change(input, { target: { value: "元の文面" } });
    fireEvent.click(screen.getByRole("button", { name: "送信" }));
    fireEvent.change(input, { target: { value: "新しい文面" } });
    await act(async () => finish(true));
    expect(input.value).toBe("新しい文面");
    expect(onMessage).toHaveBeenCalledWith("元の文面");
  });

  it.each([
    ["テキスト証跡", "テキストを添付", "text", "元の証跡", "新しい証跡"],
    ["画像ファイルのパス", "画像を添付", "image", "/tmp/old.png", "/tmp/new.png"],
  ])("%sを送信中に編集しても保持する", async (label, button, kind, original, updated) => {
    let finish!: (ok: boolean) => void;
    const onEvidence = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    render(
      <ExecutionPage
        view={view}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onMessage={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={onEvidence}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "証跡を追加" }));
    const input = screen.getByLabelText(label) as HTMLInputElement | HTMLTextAreaElement;
    fireEvent.change(input, { target: { value: original } });
    fireEvent.click(screen.getByRole("button", { name: button }));
    fireEvent.change(input, { target: { value: updated } });
    await act(async () => finish(true));
    expect(input.value).toBe(updated);
    expect(onEvidence).toHaveBeenCalledWith(
      "c",
      kind,
      kind === "text" ? original : "",
      kind === "image" ? original : "",
    );
  });

  it("AIと人間を区別し、未完了時の生成を禁止する", () => {
    const onHumanCheck = vi.fn();
    render(
      <ExecutionPage
        view={{
          ...view,
          checks: [{ ...view.checks[0], ai: { ...view.checks[0].ai, checked: false } }],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onMessage={vi.fn()}
        onHumanCheck={onHumanCheck}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByRole("heading", { name: /起動/ })).toBeTruthy();
    expect((screen.getByLabelText("AI確認済み") as HTMLInputElement).disabled).toBe(true);
    expect(
      (screen.getByRole("button", { name: "手順書の下書きを生成" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByLabelText("確認済み") as HTMLInputElement).disabled).toBe(true);
    expect(onHumanCheck).not.toHaveBeenCalled();
  });

  it("任意のAIチェックも失敗後に再実行できる", () => {
    const onRun = vi.fn();
    render(
      <ExecutionPage
        view={{
          ...view,
          checks: [
            {
              ...view.checks[0],
              ai: {
                ...view.checks[0].ai,
                required: false,
                status: "failed",
                checked: false,
                failureSummary: "接続が切れました",
              },
            },
          ],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={onRun}
        onMessage={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "AIチェックを実行" }));
    expect(onRun).toHaveBeenCalledOnce();
    expect(screen.getByText("接続が切れました")).toBeTruthy();
  });

  it("実行中の項目数と経過時間を表示する", () => {
    render(
      <ExecutionPage
        view={{
          ...view,
          activeRun: {
            runId: "run",
            status: "running",
            targetedCheckIds: ["c"],
            startedAt: new Date(Date.now() - 65_000).toISOString(),
            finishedAt: null,
          },
        }}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onMessage={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByRole("status").textContent).toContain("1件を確認中");
    expect(screen.getByRole("status").textContent).toContain("1分");
    expect(
      (screen.getByRole("button", { name: "AIチェックを実行" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});
