import { beforeEach, describe, expect, it, vi } from "vitest";
import { Call, Events } from "@wailsio/runtime";

import { StartupService } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";

import {
  checkAuthentication,
  listAgentCandidates,
  nonNull,
  onAppEvent,
  onSystemWake,
  parseAppError,
} from ".";

vi.mock("@wailsio/runtime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@wailsio/runtime")>();
  return { ...actual, Events: { ...actual.Events, On: vi.fn(() => vi.fn()) } };
});

vi.mock("../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails", () => ({
  AgentControlService: {},
  StartupService: {
    CheckAuthentication: vi.fn(),
    ListAgentCandidates: vi.fn(),
  },
}));

beforeEach(() => vi.clearAllMocks());

function runtimeError(cause: unknown): Call.RuntimeError {
  const error = new Call.RuntimeError("failed");
  error.cause = cause;
  return error;
}

describe("nonNull", () => {
  it.each([
    { name: "null", value: null },
    { name: "undefined", value: undefined },
    { name: "empty", value: [] },
  ])("$nameを空配列へ正規化する", ({ value }) => expect(nonNull(value)).toEqual([]));
});

describe("parseAppError", () => {
  it.each([
    {
      name: "structured cause",
      error: runtimeError({
        code: "validation_failed",
        message: "入力を確認してください",
        retryable: false,
      }),
      want: { code: "validation_failed", message: "入力を確認してください", retryable: false },
    },
    { name: "invalid cause", error: runtimeError({}), want: null },
    { name: "non runtime error", error: new Error("failed"), want: null },
  ])("$nameを変換する", ({ error, want }) => expect(parseAppError(error)).toEqual(want));
});

describe("Wails DTO", () => {
  it("候補のnullable collectionを空配列へ変換する", async () => {
    vi.mocked(StartupService.ListAgentCandidates).mockResolvedValue({
      items: [
        {
          candidateKey: "agent",
          displayName: "Agent",
          command: "agent",
          transport: "stdio",
          source: "path",
          args: null,
          warnings: null,
        },
      ],
      scannedAt: "2026-09-26T00:00:00Z",
      warnings: null,
    });

    await expect(listAgentCandidates(false)).resolves.toEqual([
      expect.objectContaining({ args: [], warnings: [] }),
    ]);
  });

  it("probeのnullable認証method collectionを空配列へ変換する", async () => {
    vi.mocked(StartupService.CheckAuthentication).mockResolvedValue({
      probeId: "probe-1",
      configFingerprint: "fingerprint",
      compatibility: "compatible",
      authState: "not_required",
      expiresAt: "2026-09-26T00:01:00Z",
      authMethods: null,
      authObservation: {
        status: "not_required",
        source: "initialize_only",
        observedAt: "2026-09-26T00:00:00Z",
        processGeneration: 1,
      },
    });

    await expect(
      checkAuthentication(
        {
          displayName: "Agent",
          command: "agent",
          args: [],
          transport: "stdio",
          environmentOverrides: [],
        },
        "operation-1",
      ),
    ).resolves.toEqual({ probeId: "probe-1", authState: "not_required", authMethods: [] });
  });
});

it("不正Eventをreducerへ渡さず、診断とsnapshot再取得へ切り替える", async () => {
  let receive: (event: { data: unknown }) => void = () => undefined;
  vi.mocked(Events.On).mockImplementation((_name, callback) => {
    receive = callback as typeof receive;
    return vi.fn();
  });
  const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
  const valid = vi.fn();
  const invalid = vi.fn();
  onAppEvent(valid, invalid);
  receive({ data: { eventId: "bad", payload: { secret: "do not log" } } });
  expect(valid).not.toHaveBeenCalled();
  expect(invalid).toHaveBeenCalledOnce();
  await vi.waitFor(() => expect(warning).toHaveBeenCalledOnce());
  expect(JSON.stringify(warning.mock.calls)).not.toContain("do not log");
  warning.mockRestore();
});

it("schemaに合うEventだけを購読先へ渡す", () => {
  let receive: (event: { data: unknown }) => void = () => undefined;
  vi.mocked(Events.On).mockImplementation((_name, callback) => {
    receive = callback as typeof receive;
    return vi.fn();
  });
  const valid = vi.fn();
  const invalid = vi.fn();
  onAppEvent(valid, invalid);
  receive({
    data: {
      eventId: "e1",
      name: "project.changed",
      emittedAt: "2026-09-26T00:00:00Z",
      aggregateType: "project",
      aggregateId: "one",
      changeSequence: 1,
      streamKey: "project:one",
      streamRevision: 0,
      correlation: {},
      payload: {},
    },
  });
  expect(valid).toHaveBeenCalledOnce();
  expect(invalid).not.toHaveBeenCalled();
});

it.each([
  { name: "欠落", change: (event: Record<string, unknown>) => delete event.eventId },
  { name: "型違い", change: (event: Record<string, unknown>) => (event.changeSequence = "1") },
  { name: "未知field", change: (event: Record<string, unknown>) => (event.secret = "hidden") },
  {
    name: "改変",
    change: (event: Record<string, unknown>) => (event.correlation = { projectId: 1 }),
  },
])("$nameのEventを棄却する", ({ change }) => {
  let receive: (event: { data: unknown }) => void = () => undefined;
  vi.mocked(Events.On).mockImplementation((_name, callback) => {
    receive = callback as typeof receive;
    return vi.fn();
  });
  const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
  const valid = vi.fn();
  const invalid = vi.fn();
  const event: Record<string, unknown> = {
    eventId: "e1",
    name: "project.changed",
    emittedAt: "2026-09-26T00:00:00Z",
    aggregateType: "project",
    aggregateId: "one",
    changeSequence: 1,
    streamKey: "project:one",
    streamRevision: 0,
    correlation: {},
    payload: { secret: "do not log" },
  };
  change(event);
  onAppEvent(valid, invalid);
  receive({ data: event });
  expect(valid).not.toHaveBeenCalled();
  expect(invalid).toHaveBeenCalledOnce();
  warning.mockRestore();
});

it("system wakeを共通Wails Eventから購読する", () => {
  const callback = vi.fn();
  onSystemWake(callback);
  expect(Events.On).toHaveBeenCalledWith("common:SystemDidWake", callback);
});
