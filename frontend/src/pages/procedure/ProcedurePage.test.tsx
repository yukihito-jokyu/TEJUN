import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ProcedureView } from "@/shared/api/wails/procedure";
import { ProcedurePage } from "./ProcedurePage";

afterEach(cleanup);

const view: ProcedureView = {
  project: { projectId: "project-1", name: "Sample" },
  procedure: {
    procedureId: "procedure-1",
    revision: 3,
    revisionNumber: 1,
    status: "draft",
    document: {
      title: "元の題名",
      overview: "概要",
      prerequisites: [],
      steps: [
        { clientKey: "step-1", title: "確認", description: "説明", notes: [], evidenceRefs: [] },
      ],
    },
  },
  source: { executionId: "execution-1", executionRevision: 1, checkCount: 1, evidenceCount: 2 },
  evidence: {
    ai: [
      {
        evidenceId: "ai-1",
        actor: "ai",
        kind: "command_output",
        displayName: "AI実行",
        createdAt: "2026-09-26",
      },
    ],
    human: [
      {
        evidenceId: "human-1",
        actor: "human",
        kind: "image",
        displayName: "人間確認",
        createdAt: "2026-09-26",
      },
    ],
  },
  integrity: { status: "valid", issues: [] },
  conversation: { items: [], hasPrevious: false },
  changeSequence: 1,
};

function props() {
  return {
    view,
    onReload: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    onRequestRevision: vi.fn().mockResolvedValue(undefined),
    onCancelRevision: vi.fn().mockResolvedValue(undefined),
    onRespondElicitation: vi.fn().mockResolvedValue(undefined),
    onComplete: vi.fn().mockResolvedValue(undefined),
    onExport: vi.fn().mockResolvedValue(true),
    onLoadEvidence: vi.fn().mockResolvedValue({
      summary: view.evidence.ai[0],
      source: { actor: "ai", runId: "run-1" },
      command: "pwd",
      exitCode: 0,
      integrity: "verified",
      textPage: { content: "/tmp", truncated: false },
    }),
  };
}

describe("ProcedurePage", () => {
  it("別の手順書へ切り替えると編集と古い保存応答を破棄する", async () => {
    let finishSave!: () => void;
    const input = props();
    input.onSave = vi
      .fn()
      .mockImplementation(() => new Promise<void>((resolve) => (finishSave = resolve)));
    const { rerender } = render(<ProcedurePage {...input} />);
    fireEvent.change(screen.getByLabelText("手順書タイトル"), { target: { value: "旧編集" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    const next = {
      ...view,
      project: { projectId: "project-2", name: "Next" },
      procedure: {
        ...view.procedure,
        procedureId: "procedure-2",
        document: { ...view.procedure.document, title: "新手順書" },
      },
    };
    rerender(<ProcedurePage {...input} view={next} />);
    expect(screen.getByLabelText("手順書タイトル")).toHaveProperty("value", "新手順書");
    await act(async () => finishSave());
    expect(input.onReload).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "新手順書" })).toBeTruthy();
  });

  it("証跡の続きを二重取得せず、失敗後も既存本文を表示する", async () => {
    HTMLDialogElement.prototype.showModal = function () {
      this.open = true;
    };
    const input = props();
    let failPage!: (error: Error) => void;
    input.onLoadEvidence = vi
      .fn()
      .mockResolvedValueOnce({
        summary: view.evidence.ai[0],
        source: { actor: "ai", runId: "run-1" },
        integrity: "verified",
        textPage: { content: "最初", nextCursor: "next", truncated: true },
      })
      .mockImplementationOnce(() => new Promise((_, reject) => (failPage = reject)));
    render(<ProcedurePage {...input} />);
    fireEvent.click(screen.getByRole("button", { name: "AI実行" }));
    await screen.findByText("最初");
    const more = screen.getByRole("button", { name: "続きを読む" });
    fireEvent.click(more);
    fireEvent.click(more);
    expect(input.onLoadEvidence).toHaveBeenCalledTimes(2);
    failPage(new Error("取得失敗"));
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", "取得失敗");
    expect(screen.getByText("最初")).toBeTruthy();
  });

  it("閉じて開き直した証跡に古い取得結果を表示しない", async () => {
    HTMLDialogElement.prototype.showModal = function () {
      this.open = true;
    };
    HTMLDialogElement.prototype.close = function () {
      this.open = false;
      fireEvent(this, new Event("close"));
    };
    const input = props();
    let finishOld!: (detail: unknown) => void;
    input.onLoadEvidence = vi
      .fn()
      .mockImplementationOnce(() => new Promise((resolve) => (finishOld = resolve)))
      .mockResolvedValueOnce({
        summary: view.evidence.human[0],
        source: { actor: "human" },
        integrity: "verified",
        textPage: { content: "新しい証跡", truncated: false },
      });
    render(<ProcedurePage {...input} />);
    fireEvent.click(screen.getByRole("button", { name: "AI実行" }));
    fireEvent.click(screen.getByRole("button", { name: "閉じる" }));
    fireEvent.click(screen.getByRole("button", { name: "人間確認" }));
    expect(await screen.findByText("新しい証跡")).toBeTruthy();
    await act(async () =>
      finishOld({
        summary: view.evidence.ai[0],
        source: { actor: "ai", runId: "old" },
        integrity: "verified",
        textPage: { content: "古い証跡", truncated: false },
      }),
    );
    expect(screen.queryByText("古い証跡")).toBeNull();
  });

  it("未保存の入力を保持し、revision付きで保存する", async () => {
    const input = props();
    render(<ProcedurePage {...input} />);
    fireEvent.change(
      within(screen.getByRole("region", { name: "手順書本文" })).getByLabelText("手順書タイトル"),
      { target: { value: "編集後" } },
    );
    expect(
      screen.getAllByRole("status").some((status) => status.textContent?.includes("未保存")),
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(input.onSave).toHaveBeenCalledWith(
        expect.objectContaining({ title: "編集後" }),
        3,
        expect.any(String),
      ),
    );
  });

  it("同じ手順書の外部更新後も編集開始時のrevisionで保存し、競合時に入力を残す", async () => {
    const input = props();
    input.onSave = vi.fn().mockRejectedValue(new Error("revision conflict"));
    const { rerender } = render(<ProcedurePage {...input} />);
    fireEvent.change(screen.getByLabelText("手順書タイトル"), { target: { value: "自分の編集" } });
    rerender(
      <ProcedurePage
        {...input}
        view={{
          ...view,
          procedure: {
            ...view.procedure,
            revision: 4,
            document: { ...view.procedure.document, title: "外部の編集" },
          },
        }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(input.onSave).toHaveBeenCalledWith(
        expect.objectContaining({ title: "自分の編集" }),
        3,
        expect.any(String),
      ),
    );
    expect(screen.getByLabelText("手順書タイトル")).toHaveProperty("value", "自分の編集");
    expect(
      screen
        .getAllByRole("status")
        .some((status) => status.textContent?.includes("再取得して変更内容を確認")),
    ).toBe(true);
  });

  it("完成版は編集せず指定revisionの出力を依頼する", async () => {
    const input = props();
    render(
      <ProcedurePage
        {...input}
        view={{ ...view, procedure: { ...view.procedure, status: "completed" } }}
      />,
    );
    expect(
      within(screen.getByRole("region", { name: "手順書本文" })).getByLabelText("手順書タイトル"),
    ).toHaveProperty("disabled", true);
    fireEvent.click(screen.getByRole("button", { name: "PDFを出力" }));
    await waitFor(() => expect(input.onExport).toHaveBeenCalledWith("pdf", 3, expect.any(String)));
  });

  it("出力先の選択を取り消したら成功表示と再取得をせず、次回は新しい操作IDを使う", async () => {
    const input = props();
    input.onExport = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(
      <ProcedurePage
        {...input}
        view={{ ...view, procedure: { ...view.procedure, status: "completed" } }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "PDFを出力" }));
    await waitFor(() => expect(input.onExport).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "PDFを出力" })).not.toHaveProperty(
        "disabled",
        true,
      ),
    );
    expect(input.onReload).not.toHaveBeenCalled();
    expect(screen.queryByText("PDFの出力を受け付けました。")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "PDFを出力" }));
    await waitFor(() => expect(input.onReload).toHaveBeenCalledTimes(1));
    expect(input.onExport.mock.calls[1][2]).not.toBe(input.onExport.mock.calls[0][2]);
    expect(screen.getByText("PDFの出力を受け付けました。")).toBeTruthy();
  });

  it("証跡をAIと人間に分け、選択時に詳細を遅延取得する", async () => {
    HTMLDialogElement.prototype.showModal = function () {
      this.open = true;
    };
    HTMLDialogElement.prototype.close = function () {
      this.open = false;
    };
    const input = props();
    render(<ProcedurePage {...input} />);
    expect(screen.getByRole("region", { name: "AIの証跡" }).textContent).toContain("AI実行");
    expect(screen.getByRole("region", { name: "人間の証跡" }).textContent).toContain("人間確認");
    expect(input.onLoadEvidence).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "AI実行" }));
    await waitFor(() => expect(input.onLoadEvidence).toHaveBeenCalledWith("ai-1"));
    expect(await screen.findByText("/tmp")).toBeTruthy();
  });

  it("証跡の表示名と参照有無を文書と一緒に保存する", async () => {
    const input = props();
    const editedView = {
      ...view,
      procedure: {
        ...view.procedure,
        document: {
          ...view.procedure.document,
          steps: [
            {
              ...view.procedure.document.steps[0],
              evidenceRefs: [
                { evidenceId: "ai-1", displayName: "旧名称", included: true },
                { evidenceId: "human-1", displayName: "人間確認", included: true },
              ],
            },
          ],
        },
      },
    };
    render(<ProcedurePage {...input} view={editedView} />);
    expect(screen.getByRole("textbox", { name: "証跡 ai-1 の表示名" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "証跡 human-1 の表示名" })).toBeTruthy();
    expect(screen.getByRole("checkbox", { name: "証跡 ai-1 を参照する" })).toBeTruthy();
    expect(screen.getByRole("checkbox", { name: "証跡 human-1 を参照する" })).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "証跡 ai-1 の表示名" }), {
      target: { value: "新名称" },
    });
    fireEvent.click(screen.getByRole("checkbox", { name: "証跡 ai-1 を参照する" }));
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(input.onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          steps: [
            expect.objectContaining({
              evidenceRefs: [
                { evidenceId: "ai-1", displayName: "新名称", included: false },
                { evidenceId: "human-1", displayName: "人間確認", included: true },
              ],
            }),
          ],
        }),
        3,
        expect.any(String),
      ),
    );
  });

  it("過去の会話をカーソルで取得して先頭に表示する", async () => {
    const input = props();
    const onLoadPreviousConversation = vi.fn().mockResolvedValue({
      items: [
        {
          messageId: "older",
          turnId: "turn-older",
          role: "user",
          content: [{ type: "text", text: "古い会話" }],
          status: "completed",
        },
      ],
      hasPrevious: false,
    });
    render(
      <ProcedurePage
        {...input}
        view={{
          ...view,
          conversation: { items: [], hasPrevious: true, previousCursor: 1 },
        }}
        onLoadPreviousConversation={onLoadPreviousConversation}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "過去の会話を表示" }));
    expect(await screen.findByText(/古い会話/)).toBeTruthy();
    expect(onLoadPreviousConversation).toHaveBeenCalledWith(1);
    expect(screen.queryByRole("button", { name: "過去の会話を表示" })).toBeNull();
  });
});
