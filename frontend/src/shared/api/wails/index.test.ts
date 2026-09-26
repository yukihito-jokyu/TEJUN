import { beforeEach, describe, expect, it, vi } from "vitest";
import { Call } from "@wailsio/runtime";

import { StartupService } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";

import { checkAuthentication, listAgentCandidates, nonNull, parseAppError } from ".";

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
