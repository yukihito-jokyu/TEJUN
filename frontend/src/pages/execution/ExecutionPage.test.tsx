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
      suggestedCommand: "task dev",
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
  it("AI判定の変更時だけ該当欄を強調する", () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    const input = {
      view,
      loading: false,
      busy: false,
      error: "",
      onRun: vi.fn(),
      onHumanCheck: vi.fn(),
      onEvidence: vi.fn(),
      onPermission: vi.fn(),
      onGenerate: vi.fn(),
    };
    const { container, rerender } = render(<ExecutionPage {...input} />);
    expect(container.querySelector(".execution-ai-update")).toBeNull();
    rerender(<ExecutionPage {...input} view={{ ...view }} />);
    expect(container.querySelector(".execution-ai-update")).toBeNull();
    rerender(
      <ExecutionPage
        {...input}
        view={{
          ...view,
          checks: [
            {
              ...view.checks[0],
              ai: {
                ...view.checks[0].ai,
                status: "failed",
                checked: false,
                failureSummary: "失敗",
              },
            },
          ],
        }}
      />,
    );
    expect(container.querySelector(".execution-check-side.execution-ai-update")).toBeTruthy();
    vi.unstubAllGlobals();
  });
  it("必要な証跡が保存されるまで人間チェックを押せない", () => {
    const { rerender } = render(
      <ExecutionPage
        view={{
          ...view,
          checks: [{ ...view.checks[0], humanEvidenceRequirement: "text_or_image" }],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByRole("link", { name: "一覧に戻る" }).getAttribute("href")).toBe(
      "#/projects",
    );
    expect((screen.getByRole("checkbox", { name: "確認済み" }) as HTMLInputElement).disabled).toBe(
      true,
    );
    rerender(
      <ExecutionPage
        view={
          {
            ...view,
            checks: [
              {
                ...view.checks[0],
                humanEvidenceRequirement: "text_or_image",
                evidence: {
                  ai: [],
                  human: [
                    {
                      evidenceId: "h",
                      actor: "human",
                      kind: "text",
                      text: "確認した",
                      displayName: "",
                      mimeType: "text/plain",
                      size: 0,
                      previewUrl: "",
                      createdAt: "2026-09-27T00:00:00Z",
                    },
                  ],
                },
              },
            ],
          } as ExecutionView
        }
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect((screen.getByRole("checkbox", { name: "確認済み" }) as HTMLInputElement).disabled).toBe(
      false,
    );
  });

  it("画像を貼り付け・ドロップして添付できる", () => {
    const onImageEvidence = vi.fn().mockResolvedValue(true);
    const { container } = render(
      <ExecutionPage
        view={{
          ...view,
          checks: [{ ...view.checks[0], humanEvidenceRequirement: "text_or_image" }],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onImageEvidence={onImageEvidence}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "証跡を追加" }));
    const editor = container.querySelector(".execution-evidence-editor");
    expect(editor).not.toBeNull();
    const file = new File(["image"], "pasted.png", { type: "image/png" });
    fireEvent.paste(editor!, { clipboardData: { items: [{ getAsFile: () => file }] } });
    fireEvent.drop(editor!, { dataTransfer: { files: [file] } });
    expect(onImageEvidence).toHaveBeenCalledTimes(2);
    expect(onImageEvidence).toHaveBeenCalledWith("c", file);
  });

  it("AIのテキスト証跡は種類名ではなく本文を表示する", () => {
    render(
      <ExecutionPage
        view={{
          ...view,
          checks: [
            {
              ...view.checks[0],
              evidence: {
                ai: [
                  {
                    evidenceId: "ai-evidence",
                    actor: "ai",
                    kind: "text",
                    text: "git version 2.45\n終了コード0",
                    displayName: "",
                    mimeType: "text/plain",
                    size: 0,
                    previewUrl: "",
                    createdAt: "2026-09-27T00:00:00Z",
                  },
                ],
                human: [],
              },
            },
          ],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByText(/git version 2.45/).textContent).toContain("終了コード0");
    expect(screen.queryByText("text")).toBeNull();
  });

  it("AIチェックが失敗した場合は証跡を表示しない", () => {
    render(
      <ExecutionPage
        view={{
          ...view,
          checks: [
            {
              ...view.checks[0],
              ai: {
                ...view.checks[0].ai,
                status: "failed",
                checked: false,
                failureSummary: "確認できませんでした",
              },
              evidence: {
                ai: [
                  {
                    evidenceId: "failed-evidence",
                    actor: "ai",
                    kind: "text",
                    text: "以前の結果",
                    displayName: "",
                    mimeType: "text/plain",
                    size: 0,
                    previewUrl: "",
                    createdAt: "2026-09-27T00:00:00Z",
                  },
                ],
                human: [],
              },
            },
          ],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByText("確認できませんでした")).toBeTruthy();
    expect(screen.queryByText("以前の結果")).toBeNull();
    expect(screen.getByRole("region", { name: "起動のAIチェック" }).textContent).not.toContain(
      "証跡",
    );
  });

  it("実行中のツールと回答を別の吹き出しで表示する", () => {
    render(
      <ExecutionPage
        view={
          {
            ...view,
            activeRun: { runId: "run", status: "running", targetedCheckIds: ["c"] },
            conversation: {
              items: [
                {
                  messageId: "start",
                  role: "system",
                  status: "completed",
                  content: [{ type: "text", text: "1. 開始" }],
                },
              ],
              hasPrevious: false,
            },
            activity: {
              turnId: "run",
              phase: "tool",
              items: [
                {
                  messageId: "tool",
                  role: "system",
                  status: "streaming",
                  content: [{ type: "text", text: "git --version" }],
                },
                {
                  messageId: "answer",
                  role: "agent",
                  status: "streaming",
                  content: [{ type: "text", text: "確認中" }],
                },
              ],
            },
          } as ExecutionView
        }
        loading={false}
        busy={false}
        error=""
        onRun={vi.fn()}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByText("完了：1. 開始")).toBeTruthy();
    expect(screen.getByText("実行中：git --version")).toBeTruthy();
    expect(screen.getByText("確認中")).toBeTruthy();
    expect(screen.getByText("ツールを実行中")).toBeTruthy();
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

  it("任意のAIチェックが未実行でも開始できる", () => {
    const onRun = vi.fn();
    render(
      <ExecutionPage
        view={{
          ...view,
          checks: [
            {
              ...view.checks[0],
              ai: { ...view.checks[0].ai, required: false, status: "not_required", checked: false },
            },
          ],
        }}
        loading={false}
        busy={false}
        error=""
        onRun={onRun}
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    const button = screen.getByRole("button", { name: "AIチェックを実行" }) as HTMLButtonElement;
    expect(button.disabled).toBe(false);
    fireEvent.click(button);
    expect(onRun).toHaveBeenCalledOnce();
    expect(screen.getByText("任意")).toBeTruthy();
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
        onHumanCheck={vi.fn()}
        onEvidence={vi.fn()}
        onPermission={vi.fn()}
        onGenerate={vi.fn()}
      />,
    );
    expect(screen.getByRole("status").textContent).toContain("1 / 1件を確認");
    expect(screen.getByRole("status").textContent).toContain("1分");
    expect(
      (screen.getByRole("button", { name: "AIチェックを実行" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});
