import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ExecutionRoute } from "./ExecutionRoute";

const api = vi.hoisted(() => ({
  getExecution: vi.fn(),
  attachHumanEvidence: vi.fn(),
  generateProcedureDraft: vi.fn(),
}));
vi.mock("@/shared/api/wails", () => ({
  onAppEvent: () => () => undefined,
  onSystemWake: () => () => undefined,
  parseAppError: (error: unknown) => ({ message: String(error) }),
}));
vi.mock("@/shared/api/wails/execution", () => ({
  ...api,
  runPendingChecks: vi.fn(),
  sendExecutionMessage: vi.fn(),
  setHumanCheck: vi.fn(),
  respondToExecutionPermission: vi.fn(),
}));
const snapshot = (name: string, hasPrevious = false, cursor: number | null = null) => ({
  project: { name },
  execution: { executionId: name, revision: 1 },
  session: { sessionId: name },
  changeSequence: 1,
  activeRun: null,
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
  conversation: { items: [], hasPrevious, previousCursor: cursor },
  pendingPermissions: [],
  readiness: { canGenerateProcedure: true, blockingReasons: [] },
});
const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
function mount() {
  return render(
    <MemoryRouter initialEntries={["/projects/a/check"]}>
      <Routes>
        <Route path="/projects/:projectId/check" element={<ExecutionRoute />} />
        <Route path="/projects/:projectId/procedure" element={<p>手順書画面</p>} />
        <Route path="/projects" element={<p>一覧</p>} />
      </Routes>
    </MemoryRouter>,
  );
}
afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  api.getExecution.mockResolvedValue(snapshot("a"));
});
describe("ExecutionRoute", () => {
  it("操作失敗後の再取得でエラーを消さない", async () => {
    api.attachHumanEvidence.mockRejectedValue("保存拒否");
    mount();
    await screen.findByText("a");
    fireEvent.click(screen.getByRole("button", { name: "証跡を追加" }));
    fireEvent.change(screen.getByLabelText("テキスト証跡"), { target: { value: "証跡" } });
    fireEvent.click(screen.getByRole("button", { name: "テキストを添付" }));
    expect(await screen.findByText("保存拒否")).toBeTruthy();
    expect(api.getExecution.mock.calls.length).toBeGreaterThanOrEqual(2);
  });
  it("50件を超える会話をcursorで読み込む", async () => {
    api.getExecution.mockResolvedValueOnce(snapshot("a", true, 51)).mockResolvedValueOnce({
      ...snapshot("a"),
      conversation: {
        items: [{ messageId: "old", role: "user", content: [{ text: "古い会話" }] }],
        hasPrevious: false,
        previousCursor: null,
      },
    });
    mount();
    await screen.findByText("a");
    fireEvent.click(screen.getByRole("button", { name: "以前の会話を読み込む" }));
    expect(await screen.findByText("古い会話")).toBeTruthy();
    expect(api.getExecution).toHaveBeenCalledWith("a", 51);
  });
  it("生成成功時だけnextRouteに進む", async () => {
    api.generateProcedureDraft.mockResolvedValue({ data: { nextRoute: "#/projects/a/procedure" } });
    mount();
    await screen.findByText("a");
    fireEvent.click(screen.getByRole("button", { name: "手順書の下書きを生成" }));
    expect(await screen.findByText("手順書画面")).toBeTruthy();
  });
  it("生成失敗時は画面に留まりエラーを表示する", async () => {
    api.generateProcedureDraft.mockRejectedValue("生成拒否");
    mount();
    await screen.findByText("a");
    fireEvent.click(screen.getByRole("button", { name: "手順書の下書きを生成" }));
    expect(await screen.findByText("生成拒否")).toBeTruthy();
    expect(screen.getByRole("main", { name: "動作チェック" })).toBeTruthy();
    expect(screen.queryByText("手順書画面")).toBeNull();
  });
  it("project切替後に旧応答を描画しない", async () => {
    const old = deferred<ReturnType<typeof snapshot>>();
    api.getExecution.mockReset().mockReturnValueOnce(old.promise).mockResolvedValue(snapshot("b"));
    function Switch() {
      const navigate = useNavigate();
      return (
        <button
          onClick={() => {
            Promise.resolve(navigate("/projects/b/check")).catch(() => undefined);
          }}
        >
          切替
        </button>
      );
    }
    render(
      <MemoryRouter initialEntries={["/projects/a/check"]}>
        <Switch />
        <Routes>
          <Route path="/projects/:projectId/check" element={<ExecutionRoute />} />
        </Routes>
      </MemoryRouter>,
    );
    await waitFor(() => expect(api.getExecution).toHaveBeenCalledWith("a"));
    fireEvent.click(screen.getByRole("button", { name: "切替" }));
    await screen.findByText("b");
    await act(async () => old.resolve(snapshot("a")));
    expect(screen.queryByText("a")).toBeNull();
  });
});
