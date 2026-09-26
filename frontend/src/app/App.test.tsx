import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";

import {
  authenticateAgent,
  checkAuthentication,
  completeInitialSetup,
  getStartupState,
  listAgentCandidates,
  listProjects,
  onAppEvent,
  onSystemWake,
  type AgentCandidate,
} from "@/shared/api/wails";

import { App } from "./App";

const candidate: AgentCandidate = {
  candidateKey: "agent",
  displayName: "Agent",
  command: "agent",
  args: [],
  transport: "stdio",
  source: "path",
  warnings: [],
};

const completed: Awaited<ReturnType<typeof completeInitialSetup>> = {
  data: {
    connection: {
      connectionId: "connection-1",
      displayName: "Agent",
      command: "agent",
      transport: "stdio",
      resolvedExecutablePath: "/agent",
      args: [],
      lastVerifiedAt: "2026-09-26T00:00:00Z",
      protocolVersion: "1",
      authState: "not_required",
      schemaArtifactVersion: "1",
    },
    nextRoute: "/projects",
    changeSequence: 1,
  },
  receipt: { operationId: "operation-1", committedAt: "2026-09-26T00:00:00Z", changeSequence: 1 },
};

const appEvent = (aggregateType: string, payload: Record<string, unknown> = {}) => ({
  eventId: "event-1",
  name: "test",
  emittedAt: "2026-09-26T00:00:00Z",
  aggregateType,
  aggregateId: "one",
  changeSequence: 1,
  streamKey: `${aggregateType}:one`,
  streamRevision: 1,
  correlation: {},
  payload,
});

vi.mock("@/shared/api/wails", () => ({
  authenticateAgent: vi.fn(),
  checkAuthentication: vi.fn(),
  completeInitialSetup: vi.fn(),
  getStartupState: vi.fn(),
  listAgentCandidates: vi.fn(),
  listProjects: vi.fn(),
  onAppEvent: vi.fn(() => vi.fn()),
  onSystemWake: vi.fn(() => vi.fn()),
  parseAppError: vi.fn(() => null),
}));
vi.mock("@/pages/preparation/PreparationRoute", () => ({
  PreparationRoute: () => <h1>準備画面</h1>,
}));

afterEach(cleanup);

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listAgentCandidates).mockResolvedValue([candidate]);
  vi.mocked(completeInitialSetup).mockResolvedValue(completed);
  vi.mocked(onAppEvent).mockReturnValue(vi.fn());
  vi.mocked(onSystemWake).mockReturnValue(vi.fn());
  vi.mocked(listProjects).mockResolvedValue({
    items: [],
    total: 0,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 0,
  });
});

describe("App routing", () => {
  it("一覧から準備画面へ進む", async () => {
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: false,
      nextRoute: "/projects",
      defaultConnection: undefined,
      changeSequence: 1,
    });
    vi.mocked(listProjects).mockResolvedValue({
      items: [
        {
          projectId: "p1",
          name: "準備中",
          description: "",
          workspacePath: "/tmp/project",
          status: "preparing",
          currentStage: "preparation",
          progress: { completed: 0, total: 3 },
          attentionRank: 0,
          attentionReason: null,
          updatedAt: "2026-09-26T00:00:00Z",
          completedAt: null,
          resumeRoute: "#/projects/p1/prepare",
          currentProcedureId: null,
          connectionState: "disconnected",
          errorSummary: null,
          revision: 1,
        },
      ],
      total: 1,
      nextCursor: null,
      generatedAt: "",
      changeSequence: 1,
    });
    render(
      <MemoryRouter initialEntries={["/projects"]}>
        <App />
      </MemoryRouter>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "開く" }));
    expect(await screen.findByRole("heading", { name: "準備画面" })).toBeTruthy();
  });

  it("project ID付き準備URLをセットアップへ戻さない", async () => {
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: false,
      nextRoute: "/projects",
      defaultConnection: undefined,
      changeSequence: 1,
    });
    render(
      <MemoryRouter initialEntries={["/projects/p1/prepare"]}>
        <App />
      </MemoryRouter>,
    );
    expect(await screen.findByRole("heading", { name: "準備画面" })).toBeTruthy();
  });
  it.each([
    { initialSetupRequired: true, nextRoute: "/setup", heading: "AIエージェントを接続" },
    { initialSetupRequired: false, nextRoute: "/projects", heading: "作業を続ける" },
  ])("起動時は設定状態に応じた画面を表示する", async (state) => {
    vi.mocked(getStartupState).mockResolvedValue({
      ...state,
      defaultConnection: undefined,
      changeSequence: 0,
    });

    render(<App />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: state.heading })).toBeTruthy();
  });

  it("app:event後にstartup snapshotを再取得する", async () => {
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: false,
      nextRoute: "/projects",
      defaultConnection: undefined,
      changeSequence: 1,
    });
    let eventHandler = (_event: ReturnType<typeof appEvent>) => {};
    const unsubscribe = vi.fn();
    vi.mocked(onAppEvent).mockImplementation((callback) => {
      eventHandler = callback;
      return unsubscribe;
    });

    const view = render(<App />, { wrapper: MemoryRouter });
    await screen.findByRole("heading", { name: "作業を続ける" });
    const callsBeforeEvent = vi.mocked(getStartupState).mock.calls.length;
    eventHandler(appEvent("other"));

    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(callsBeforeEvent + 1));
    view.unmount();
    expect(unsubscribe).toHaveBeenCalledTimes(vi.mocked(onAppEvent).mock.calls.length);
  });

  it("不正Eventとsystem wakeでstartup snapshotを再取得し、購読を解除する", async () => {
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: false,
      nextRoute: "/projects",
      defaultConnection: undefined,
      changeSequence: 1,
    });
    let invalidHandler = () => {};
    let wakeHandler = () => {};
    const unsubscribeEvent = vi.fn();
    const unsubscribeWake = vi.fn();
    vi.mocked(onAppEvent).mockImplementation((_callback, onInvalid) => {
      invalidHandler = onInvalid ?? (() => {});
      return unsubscribeEvent;
    });
    vi.mocked(onSystemWake).mockImplementation((callback) => {
      wakeHandler = callback;
      return unsubscribeWake;
    });

    const view = render(<App />, { wrapper: MemoryRouter });
    await screen.findByRole("heading", { name: "作業を続ける" });
    expect(vi.mocked(onAppEvent).mock.calls[0]?.[1]).toBeTypeOf("function");
    const initialCalls = vi.mocked(getStartupState).mock.calls.length;
    act(() => invalidHandler());
    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(initialCalls + 1));
    act(() => wakeHandler());
    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(initialCalls + 2));

    view.unmount();
    expect(unsubscribeEvent).toHaveBeenCalledTimes(vi.mocked(onAppEvent).mock.calls.length);
    expect(unsubscribeWake).toHaveBeenCalledTimes(vi.mocked(onSystemWake).mock.calls.length);
  });

  it("古い不正Event応答で新しいwake状態を上書きしない", async () => {
    let resolveOld!: (value: {
      initialSetupRequired: boolean;
      nextRoute: string;
      defaultConnection: undefined;
      changeSequence: number;
    }) => void;
    const oldRequest = new Promise<Parameters<typeof resolveOld>[0]>((resolve) => {
      resolveOld = resolve;
    });
    vi.mocked(getStartupState)
      .mockResolvedValueOnce({
        initialSetupRequired: false,
        nextRoute: "/projects",
        defaultConnection: undefined,
        changeSequence: 1,
      })
      .mockReturnValueOnce(oldRequest)
      .mockResolvedValueOnce({
        initialSetupRequired: false,
        nextRoute: "/projects",
        defaultConnection: undefined,
        changeSequence: 3,
      });
    let invalidHandler = () => {};
    let wakeHandler = () => {};
    vi.mocked(onAppEvent).mockImplementation((_callback, onInvalid) => {
      invalidHandler = onInvalid ?? (() => {});
      return vi.fn();
    });
    vi.mocked(onSystemWake).mockImplementation((callback) => {
      wakeHandler = callback;
      return vi.fn();
    });

    render(<App />, { wrapper: MemoryRouter });
    await screen.findByRole("heading", { name: "作業を続ける" });
    act(() => {
      invalidHandler();
      wakeHandler();
    });
    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(3));
    await act(async () => {
      resolveOld({
        initialSetupRequired: true,
        nextRoute: "/setup",
        defaultConnection: undefined,
        changeSequence: 2,
      });
      await oldRequest;
    });

    expect(screen.getByRole("heading", { name: "作業を続ける" })).toBeTruthy();
    expect(listAgentCandidates).toHaveBeenCalledOnce();
  });

  it("probe失敗の再試行は候補取得ではなくprobeを再実行する", async () => {
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: true,
      nextRoute: "/setup",
      defaultConnection: undefined,
      changeSequence: 0,
    });
    vi.mocked(checkAuthentication)
      .mockRejectedValueOnce(new Error("probe failed"))
      .mockResolvedValueOnce({ probeId: "probe-1", authState: "not_required", authMethods: [] });

    render(<App />, { wrapper: MemoryRouter });
    fireEvent.click(await screen.findByRole("button", { name: "接続" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "再試行" }));

    expect(screen.queryByRole("alert")).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: /プロジェクト一覧へ/ }));
    await screen.findByRole("heading", { name: "作業を続ける" });
    expect(checkAuthentication).toHaveBeenCalledTimes(2);
    expect(listAgentCandidates).toHaveBeenCalledOnce();
  });

  it("probe中のEvent再取得はprobe結果を失効させない", async () => {
    let resolveProbe!: (value: {
      probeId: string;
      authState: "not_required";
      authMethods: [];
    }) => void;
    const pendingProbe = new Promise<Parameters<typeof resolveProbe>[0]>((resolve) => {
      resolveProbe = resolve;
    });
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: true,
      nextRoute: "/setup",
      defaultConnection: undefined,
      changeSequence: 0,
    });
    vi.mocked(checkAuthentication).mockReturnValue(pendingProbe);
    let eventHandler = (_event: ReturnType<typeof appEvent>) => {};
    vi.mocked(onAppEvent).mockImplementation((callback) => {
      eventHandler = callback;
      return vi.fn();
    });

    render(<App />, { wrapper: MemoryRouter });
    fireEvent.click(await screen.findByRole("button", { name: "接続" }));
    act(() => eventHandler(appEvent("other")));
    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(2));
    await act(async () =>
      resolveProbe({ probeId: "probe-1", authState: "not_required", authMethods: [] }),
    );

    fireEvent.click(await screen.findByRole("button", { name: /プロジェクト一覧へ/ }));
    await screen.findByRole("heading", { name: "作業を続ける" });
  });

  it("認証Job完了後に元のprobeを保存する", async () => {
    vi.mocked(getStartupState)
      .mockResolvedValueOnce({
        initialSetupRequired: true,
        nextRoute: "/setup",
        defaultConnection: undefined,
        changeSequence: 0,
      })
      .mockImplementation(() => new Promise(() => {}));
    vi.mocked(checkAuthentication).mockResolvedValue({
      probeId: "probe-auth",
      authState: "unknown",
      authMethods: [
        {
          type: "agent",
          authMethodId: "chat-gpt",
          name: "ChatGPT",
          description: "Use ChatGPT to authenticate",
          environmentNames: [],
        },
      ],
    });
    vi.mocked(authenticateAgent).mockResolvedValue({
      data: {
        jobId: "job-1",
        targetId: "probe-auth",
        state: "pending",
        acceptedAt: "2026-09-26T00:00:00Z",
      },
      receipt: {
        operationId: "operation-1",
        committedAt: "2026-09-26T00:00:00Z",
        changeSequence: 1,
      },
    });
    let eventHandler = (_event: ReturnType<typeof appEvent>) => {};
    vi.mocked(onAppEvent).mockImplementation((callback) => {
      eventHandler = callback;
      return vi.fn();
    });

    render(<App />, { wrapper: MemoryRouter });
    fireEvent.click(await screen.findByRole("button", { name: "接続" }));
    fireEvent.click(await screen.findByRole("button", { name: "認証する" }));
    await act(async () => {
      eventHandler({
        ...appEvent("agent_job", { connectionId: "probe-auth", authState: "succeeded" }),
        name: "agent.authentication.updated",
      });
      eventHandler(
        appEvent("agent_connection", { connectionId: "probe-auth", authState: "pending" }),
      );
    });

    fireEvent.click(await screen.findByRole("button", { name: /プロジェクト一覧へ/ }));
    await screen.findByRole("heading", { name: "作業を続ける" });
    expect(completeInitialSetup).toHaveBeenCalledWith(
      expect.objectContaining({ command: "agent" }),
      "probe-auth",
      expect.any(String),
    );
  });

  it("認証Job失敗後は入力を保って再試行できる", async () => {
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: true,
      nextRoute: "/setup",
      defaultConnection: undefined,
      changeSequence: 0,
    });
    vi.mocked(checkAuthentication).mockResolvedValue({
      probeId: "probe-auth",
      authState: "unknown",
      authMethods: [
        {
          type: "agent",
          authMethodId: "chat-gpt",
          name: "ChatGPT",
          environmentNames: [],
        },
      ],
    });
    vi.mocked(authenticateAgent).mockResolvedValue({
      data: {
        jobId: "job-1",
        targetId: "probe-auth",
        state: "pending",
        acceptedAt: "2026-09-26T00:00:00Z",
      },
      receipt: {
        operationId: "operation-1",
        committedAt: "2026-09-26T00:00:00Z",
        changeSequence: 1,
      },
    });
    let eventHandler = (_event: ReturnType<typeof appEvent>) => {};
    vi.mocked(onAppEvent).mockImplementation((callback) => {
      eventHandler = callback;
      return vi.fn();
    });

    render(<App />, { wrapper: MemoryRouter });
    fireEvent.click(await screen.findByRole("button", { name: "接続" }));
    fireEvent.click(await screen.findByRole("button", { name: "認証する" }));
    act(() =>
      eventHandler({
        ...appEvent("agent_job", { connectionId: "probe-auth", authState: "failed" }),
        name: "agent.authentication.updated",
      }),
    );

    expect(await screen.findByText("認証に失敗しました。もう一度お試しください。")).toBeTruthy();
    expect(completeInitialSetup).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "再試行" }));
    await waitFor(() => expect(checkAuthentication).toHaveBeenCalledTimes(2));
  });

  it("保存後にBackend指定routeへ遷移する", async () => {
    vi.mocked(getStartupState)
      .mockResolvedValueOnce({
        initialSetupRequired: true,
        nextRoute: "/setup",
        defaultConnection: undefined,
        changeSequence: 0,
      })
      .mockResolvedValueOnce({
        initialSetupRequired: false,
        nextRoute: "/projects",
        defaultConnection: undefined,
        changeSequence: 1,
      });
    vi.mocked(completeInitialSetup).mockResolvedValue({
      data: {
        connection: {
          connectionId: "connection-1",
          displayName: "Agent",
          command: "agent",
          transport: "stdio",
          resolvedExecutablePath: "/agent",
          args: [],
          lastVerifiedAt: "2026-09-26T00:00:00Z",
          protocolVersion: "1",
          authState: "not_required",
          schemaArtifactVersion: "1",
        },
        nextRoute: "/projects",
        changeSequence: 1,
      },
      receipt: {
        operationId: "operation-1",
        committedAt: "2026-09-26T00:00:00Z",
        changeSequence: 1,
      },
    });
    vi.mocked(checkAuthentication).mockResolvedValue({
      probeId: "probe-1",
      authState: "not_required",
      authMethods: [],
    });
    let eventHandler = (_event: ReturnType<typeof appEvent>) => {};
    vi.mocked(onAppEvent).mockImplementation((callback) => {
      eventHandler = callback;
      return vi.fn();
    });

    render(<App />, { wrapper: MemoryRouter });

    fireEvent.click(await screen.findByRole("button", { name: "接続" }));
    await screen.findByText("エージェントを接続しました");
    await act(async () => eventHandler(appEvent("other")));
    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("button", { name: /プロジェクト一覧へ/ })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /プロジェクト一覧へ/ }));
    expect(screen.getByRole("heading", { name: "作業を続ける" })).toBeTruthy();
    expect(completeInitialSetup).toHaveBeenCalledTimes(1);
  });

  it("保存中にEventが先着しても保存応答で遷移する", async () => {
    let resolveSave!: (value: Awaited<ReturnType<typeof completeInitialSetup>>) => void;
    const pendingSave = new Promise<Awaited<ReturnType<typeof completeInitialSetup>>>((resolve) => {
      resolveSave = resolve;
    });
    vi.mocked(getStartupState).mockResolvedValue({
      initialSetupRequired: true,
      nextRoute: "/setup",
      defaultConnection: undefined,
      changeSequence: 0,
    });
    vi.mocked(checkAuthentication).mockResolvedValue({
      probeId: "probe-1",
      authState: "not_required",
      authMethods: [],
    });
    vi.mocked(completeInitialSetup).mockReturnValue(
      pendingSave as ReturnType<typeof completeInitialSetup>,
    );
    let eventHandler = (_event: ReturnType<typeof appEvent>) => {};
    vi.mocked(onAppEvent).mockImplementation((callback) => {
      eventHandler = callback;
      return vi.fn();
    });

    render(<App />, { wrapper: MemoryRouter });
    fireEvent.click(await screen.findByRole("button", { name: "接続" }));
    await act(async () => eventHandler(appEvent("other")));
    await waitFor(() => expect(getStartupState).toHaveBeenCalledTimes(2));
    await act(async () =>
      resolveSave({
        data: {
          connection: {
            connectionId: "connection-1",
            displayName: "Agent",
            command: "agent",
            transport: "stdio",
            resolvedExecutablePath: "/agent",
            args: [],
            lastVerifiedAt: "2026-09-26T00:00:00Z",
            protocolVersion: "1",
            authState: "not_required",
            schemaArtifactVersion: "1",
          },
          nextRoute: "/projects",
          changeSequence: 1,
        },
        receipt: {
          operationId: "operation-1",
          committedAt: "2026-09-26T00:00:00Z",
          changeSequence: 1,
        },
      }),
    );

    fireEvent.click(await screen.findByRole("button", { name: /プロジェクト一覧へ/ }));
    expect(await screen.findByRole("heading", { name: "作業を続ける" })).toBeTruthy();
  });
});
