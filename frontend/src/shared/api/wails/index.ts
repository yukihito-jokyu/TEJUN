import { Events } from "@wailsio/runtime";
import Ajv2020 from "ajv/dist/2020";
import addFormats from "ajv-formats";

import appEventSchema from "../../../../../internal/adapter/wails/events/app_event.schema.json";

import {
  AgentControlService,
  StartupService,
  type AgentCandidate as GeneratedAgentCandidate,
  type AgentConnectionInput as GeneratedAgentConnectionInput,
  type AgentProbeResult as GeneratedAgentProbeResult,
} from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";

export { parseAppError, type AppErrorCause } from "./errors";
export {
  archiveProject,
  createProject,
  createRevision,
  deleteProject,
  chooseExportDestination,
  chooseWorkspaceDirectory,
  duplicateProject,
  exportProcedure,
  listProjects,
  reconnectProject,
  type ProjectFilter,
  type ProjectSummary,
  type PreparedExportProcedure,
} from "./projects";

export type AgentConnectionInput = {
  displayName: string;
  command: string;
  args: string[];
  transport: "stdio";
  environmentOverrides: { name: string; value: string }[];
};

export type AgentCandidate = {
  candidateKey: string;
  displayName: string;
  command: string;
  args: string[];
  transport: "stdio";
  source: "path" | "bundled_default" | "recent";
  warnings: string[];
};

export type AuthMethod = {
  type: "agent" | "terminal";
  authMethodId: string;
  name: string;
  description?: string;
  environmentNames: string[];
};

export type AgentProbe = {
  probeId: string;
  authState: "unknown" | "not_required" | "required" | "authenticated" | "failed" | "unsupported";
  authMethods: AuthMethod[];
};

export type AppEvent = {
  eventId: string;
  name: string;
  emittedAt: string;
  aggregateType: string;
  aggregateId: string;
  changeSequence: number;
  streamKey: string;
  streamRevision: number;
  correlation: Record<string, string>;
  payload: Record<string, unknown>;
};

export async function getStartupState() {
  const state = await StartupService.GetStartupState();
  return {
    ...state,
    defaultConnection: state.defaultConnection
      ? { ...state.defaultConnection, args: nonNull(state.defaultConnection.args) }
      : undefined,
  };
}

export async function listAgentCandidates(refresh: boolean): Promise<AgentCandidate[]> {
  const result = await StartupService.ListAgentCandidates({ refresh });
  return nonNull(result.items).map(candidate);
}

export async function checkAuthentication(
  connection: AgentConnectionInput,
  operationId: string,
): Promise<AgentProbe> {
  return probe(
    await StartupService.CheckAuthentication({
      connection: generatedConnection(connection),
      operationId,
    }),
  );
}

export function authenticateAgent(probeId: string, authMethodId: string, operationId: string) {
  return AgentControlService.AuthenticateAgent({
    probeId,
    connectionId: "",
    authMethodId,
    operationId,
  });
}

export function completeInitialSetup(
  connection: AgentConnectionInput,
  probeId: string,
  operationId: string,
) {
  return StartupService.CompleteInitialSetup({
    connection: generatedConnection(connection),
    probeId,
    operationId,
  });
}

export function logoutAgent(connectionId: string, operationId: string) {
  return AgentControlService.LogoutAgent({ connectionId, operationId });
}

export function respondToElicitation(
  elicitationRequestId: string,
  action: "accept" | "decline" | "cancel",
  content: string,
  operationId: string,
) {
  return AgentControlService.RespondToElicitation({
    elicitationRequestId,
    action,
    content,
    operationId,
  });
}

export function onAppEvent(callback: (event: AppEvent) => void, onInvalid?: () => void) {
  return Events.On("app:event", (event) => {
    const value: unknown = event.data;
    if (isAppEvent(value)) callback(value);
    else {
      onInvalid?.();
      const causeId = crypto.randomUUID();
      const payload =
        value && typeof value === "object" ? (value as Record<string, unknown>).payload : null;
      if (!crypto.subtle) {
        console.warn("不正なAppEvent", { causeId, payloadDigest: "unavailable" });
        return;
      }
      void crypto.subtle
        .digest("SHA-256", new TextEncoder().encode(JSON.stringify(payload)))
        .then((digest) =>
          console.warn("不正なAppEvent", {
            causeId,
            payloadDigest: Array.from(new Uint8Array(digest), (byte) =>
              byte.toString(16).padStart(2, "0"),
            ).join(""),
          }),
        )
        .catch(() => console.warn("不正なAppEvent", { causeId, payloadDigest: "unavailable" }));
    }
  });
}

export function onSystemWake(callback: () => void) {
  return Events.On("common:SystemDidWake", callback);
}

const validateAppEvent = addFormats(new Ajv2020()).compile(appEventSchema);

function isAppEvent(value: unknown): value is AppEvent {
  return validateAppEvent(value) as boolean;
}

export function nonNull<T>(items: T[] | null | undefined): T[] {
  return items ?? [];
}

function generatedConnection(connection: AgentConnectionInput): GeneratedAgentConnectionInput {
  return connection;
}

function candidate(value: GeneratedAgentCandidate): AgentCandidate {
  if (value.transport !== "stdio") throw new Error("未対応のAgent transportです");
  if (value.source !== "path" && value.source !== "bundled_default" && value.source !== "recent") {
    throw new Error("未対応のAgent候補sourceです");
  }

  return {
    ...value,
    transport: value.transport,
    source: value.source,
    args: nonNull(value.args),
    warnings: nonNull(value.warnings),
  };
}

function probe(value: GeneratedAgentProbeResult): AgentProbe {
  const authState = value.authState;
  if (!isAuthState(authState)) throw new Error("未対応の認証状態です");

  return {
    probeId: value.probeId,
    authState,
    authMethods: nonNull(value.authMethods).map((method) => {
      if (method.type !== "agent" && method.type !== "terminal") {
        throw new Error("未対応の認証方法です");
      }
      return { ...method, type: method.type, environmentNames: nonNull(method.environmentNames) };
    }),
  };
}

function isAuthState(value: string): value is AgentProbe["authState"] {
  return ["unknown", "not_required", "required", "authenticated", "failed", "unsupported"].includes(
    value,
  );
}
