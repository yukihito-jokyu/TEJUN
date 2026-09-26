import { Events } from "@wailsio/runtime";

import {
  AgentControlService,
  StartupService,
  type AgentCandidate as GeneratedAgentCandidate,
  type AgentConnectionInput as GeneratedAgentConnectionInput,
  type AgentProbeResult as GeneratedAgentProbeResult,
} from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";

export { parseAppError, type AppErrorCause } from "./errors";

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
  aggregateType: string;
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

export function onAppEvent(callback: (event: AppEvent) => void) {
  return Events.On("app:event", (event) => callback(event.data));
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
