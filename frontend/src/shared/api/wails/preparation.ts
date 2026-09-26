import {
  AgentControlService,
  PreparationService,
} from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";
import { nonNull } from "./index";

export type CheckItemInput = {
  checkId?: string;
  clientKey: string;
  title: string;
  instruction: string;
  expectedResult: string;
  suggestedCommand?: string;
  aiRequired: boolean;
  humanRequired: boolean;
  humanEvidenceRequirement: string;
};
export type PreparationView = {
  project: {
    projectId: string;
    name: string;
    description: string;
    workspacePath: string;
    revision: number;
  };
  preparation: {
    purpose: string;
    completionCriteria: string[];
    intendedUsers: string;
    revision: number;
  };
  checkPlan: {
    planId: string;
    revision: number;
    items: (Omit<CheckItemInput, "clientKey"> & { checkId: string; sequence: number })[];
  };
  session?: {
    sessionId: string;
    state: string;
    permissionPolicy: { mode: "ask_every_time" | "reuse_explicit_always_choice"; revision: number };
    revision: number;
    modes?: { currentModeId: string; available: { modeId: string; name: string }[] };
    configOptions?: {
      id: string;
      name: string;
      currentValue: string | boolean;
      choices: { value: string; name: string }[];
    }[];
  };
  conversation: {
    items: {
      messageId: string;
      turnId: string;
      role: string;
      content: { text?: string; name?: string; url?: string }[];
      status: string;
    }[];
    hasPrevious: boolean;
    previousCursor?: number;
  };
  chats?: { sessionId: string; title: string; startedAt: string }[];
  elicitations: {
    elicitationRequestId: string;
    mode: string;
    message: string;
    status: string;
    url?: string;
    requestedSchema?: Record<string, unknown>;
  }[];
  readiness: {
    canStartExecution: boolean;
    blockingReasons: { code: string; message: string; checkId?: string }[];
  };
  changeSequence: number;
};

export async function getPreparation(
  projectId: string,
  conversationCursor?: number,
  chatId?: string,
): Promise<PreparationView> {
  const view = await PreparationService.GetPreparation({
    projectId,
    chatId: chatId ?? "",
    conversationCursor,
    conversationLimit: 50,
  });
  return {
    ...view,
    preparation: {
      ...view.preparation,
      completionCriteria: nonNull(view.preparation.completionCriteria),
    },
    checkPlan: { ...view.checkPlan, items: nonNull(view.checkPlan.items) },
    conversation: {
      ...view.conversation,
      previousCursor: view.conversation.previousCursor ?? undefined,
      items: nonNull(view.conversation.items).map((item) => ({
        ...item,
        content: nonNull(item.content),
      })),
    },
    chats: nonNull(view.chats),
    elicitations: nonNull(view.elicitations).map((item) => ({
      ...item,
      requestedSchema: item.requestedSchema ?? undefined,
    })),
    session: view.session
      ? {
          ...view.session,
          permissionPolicy: {
            ...view.session.permissionPolicy,
            mode:
              view.session.permissionPolicy.mode === "reuse_explicit_always_choice"
                ? ("reuse_explicit_always_choice" as const)
                : ("ask_every_time" as const),
          },
          modes: view.session.modes
            ? { ...view.session.modes, available: nonNull(view.session.modes.available) }
            : undefined,
          configOptions: nonNull(view.session.configOptions)
            .filter(
              (option) =>
                typeof option.currentValue === "string" || typeof option.currentValue === "boolean",
            )
            .map((option) => ({
              ...option,
              id: option.configId,
              currentValue: option.currentValue as string | boolean,
              choices: [
                ...nonNull(option.options?.items),
                ...nonNull(option.options?.groups).flatMap((group) => nonNull(group.options)),
              ],
            })),
        }
      : undefined,
    readiness: { ...view.readiness, blockingReasons: nonNull(view.readiness.blockingReasons) },
  };
}

export const sendPreparationMessage = (
  projectId: string,
  sessionId: string | undefined,
  text: string,
  operationId: string,
) =>
  PreparationService.SendPreparationMessage({
    projectId,
    sessionId: sessionId ?? "",
    content: [{ type: "text", text, evidenceId: "", url: "", name: "", mimeType: "" }],
    operationId,
  });
export const savePreparationBrief = (
  projectId: string,
  expectedPreparationRevision: number,
  brief: Omit<PreparationView["preparation"], "revision">,
  operationId: string,
) =>
  PreparationService.SavePreparationBrief({
    projectId,
    expectedPreparationRevision,
    brief,
    operationId,
  });
export const saveCheckPlan = (
  projectId: string,
  expectedPreparationRevision: number,
  expectedPlanRevision: number,
  items: CheckItemInput[],
  operationId: string,
) =>
  PreparationService.SaveCheckPlan({
    projectId,
    expectedPreparationRevision,
    expectedPlanRevision,
    items: items.map((item) => ({
      ...item,
      checkId: item.checkId ?? "",
      suggestedCommand: item.suggestedCommand ?? "",
    })),
    operationId,
  });
export const changeProjectWorkspace = (
  projectId: string,
  newWorkspacePath: string,
  expectedProjectRevision: number,
  operationId: string,
) =>
  PreparationService.ChangeProjectWorkspace({
    projectId,
    newWorkspacePath,
    expectedProjectRevision,
    confirmSessionReset: true,
    operationId,
  });
export const saveSessionPermissionPolicy = (
  sessionId: string,
  expectedSessionRevision: number,
  mode: "ask_every_time" | "reuse_explicit_always_choice",
  operationId: string,
) =>
  PreparationService.SaveSessionPermissionPolicy({
    sessionId,
    expectedSessionRevision,
    mode,
    operationId,
  });
export const startExecution = (
  projectId: string,
  expectedPreparationRevision: number,
  expectedPlanRevision: number,
  operationId: string,
) =>
  PreparationService.StartExecution({
    projectId,
    expectedPreparationRevision,
    expectedPlanRevision,
    operationId,
  });
export const cancelAgentOperation = (sessionId: string, turnId: string, operationId: string) =>
  AgentControlService.CancelAgentOperation({ sessionId, turnId, operationId });
export const setAgentMode = (
  sessionId: string,
  expectedRevision: number,
  modeId: string,
  operationId: string,
) =>
  AgentControlService.SetAgentSessionConfiguration({
    sessionId,
    expectedRevision,
    change: { kind: "mode", modeId },
    operationId,
  });
export const setAgentConfigOption = (
  sessionId: string,
  expectedRevision: number,
  configId: string,
  value: string | boolean,
  operationId: string,
) =>
  AgentControlService.SetAgentSessionConfiguration({
    sessionId,
    expectedRevision,
    change: { kind: "config_option", configId, value },
    operationId,
  });
export const respondToPreparationElicitation = (
  elicitationRequestId: string,
  action: "accept" | "decline" | "cancel",
  content: string,
  operationId: string,
) =>
  AgentControlService.RespondToElicitation({ elicitationRequestId, action, content, operationId });
