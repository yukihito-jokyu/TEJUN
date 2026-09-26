import type { Meta, StoryObj } from "@storybook/react-vite";

import type { AgentCandidate } from "@/features/setup-connection/ui/SetupConnection";

import { SetupPage } from "./SetupPage";

const candidate: AgentCandidate = {
  candidateKey: "codex",
  displayName: "Codex ACP",
  command: "codex-acp",
  args: [],
  transport: "stdio",
  source: "path",
  warnings: [],
};

const noop = () => {};

const meta = {
  component: SetupPage,
  title: "Pages/Setup",
  args: {
    candidates: [candidate],
    status: "ready",
    onRefresh: noop,
    onConnect: noop,
    onAuthenticate: noop,
    onContinue: noop,
    onRetry: noop,
  },
  parameters: { viewport: { defaultViewport: "desktop" } },
} satisfies Meta<typeof SetupPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = { args: { loading: true } };
export const CandidatesEmpty: Story = { args: { candidates: [] } };
export const CandidateSelected: Story = {
  args: { selected: { ...candidate, environmentOverrides: [] } },
};
export const AuthenticationRequiredAgent: Story = {
  args: {
    selected: { ...candidate, environmentOverrides: [] },
    authState: "required",
    authMethods: [
      { type: "agent", authMethodId: "oauth", name: "ブラウザで認証", environmentNames: [] },
    ],
  },
};
export const AuthenticationRequiredTerminal: Story = {
  args: {
    selected: { ...candidate, environmentOverrides: [] },
    authState: "required",
    authMethods: [
      { type: "terminal", authMethodId: "login", name: "CLIログイン", environmentNames: [] },
    ],
  },
};
export const ErrorRetryable: Story = {
  args: {
    selected: { ...candidate, environmentOverrides: [] },
    error: { code: "process_start_failed", message: "Agentを起動できません。", retryable: true },
  },
};
export const Saving: Story = {
  args: {
    selected: { ...candidate, environmentOverrides: [] },
    authState: "authenticated",
    status: "saving",
  },
};
export const Connected: Story = { args: { connected: true } };
