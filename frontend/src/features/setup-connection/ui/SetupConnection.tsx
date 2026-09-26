import { Bot, CheckCircle2, ChevronRight, KeyRound, PlugZap } from "lucide-react";

import { ErrorState } from "@/components/patterns/ErrorState";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import type { AgentCandidate, AgentConnectionInput, AuthMethod } from "@/shared/api/wails";

import "./SetupConnection.css";

export type { AgentCandidate, AgentConnectionInput } from "@/shared/api/wails";

export type SetupConnectionProps = {
  candidates: AgentCandidate[];
  selected?: AgentConnectionInput;
  status: "ready" | "probing" | "authenticating" | "saving";
  authState?: "unknown" | "not_required" | "required" | "authenticated" | "failed" | "unsupported";
  authMethods?: AuthMethod[];
  error?: { code: string; message: string; retryable: boolean };
  connected?: boolean;
  onRefresh: () => void;
  onConnect: (candidate: AgentCandidate) => void;
  onAuthenticate: (authMethodId: string) => void;
  onContinue: () => void;
  onRetry: () => void;
};

export function SetupConnection({
  candidates,
  selected,
  status,
  authState,
  authMethods = [],
  error,
  connected,
  onRefresh,
  onConnect,
  onAuthenticate,
  onContinue,
  onRetry,
}: SetupConnectionProps) {
  const busy = status !== "ready";

  return (
    <div className="setup-form">
      {error && (
        <ErrorState
          title={errorTitle(error.code)}
          description={error.message}
          onRetry={error.retryable ? onRetry : undefined}
        />
      )}

      <section className="setup-section" aria-labelledby="agent-title">
        <div className="setup-section-heading">
          <div>
            <h2 id="agent-title">使用するエージェント</h2>
            <p>このコンピューターで利用できるエージェントです。</p>
          </div>
        </div>

        {candidates.length === 0 ? (
          <div className="setup-empty" role="status">
            <p>利用できるエージェントが見つかりませんでした。</p>
            <Button size="sm" type="button" disabled={busy} onClick={onRefresh}>
              もう一度探す
            </Button>
          </div>
        ) : (
          <div className="candidate-list">
            {candidates.map((candidate) => {
              const active = selected?.command === candidate.command;

              return (
                <article className="candidate" key={candidate.candidateKey}>
                  <span className="agent-logo" aria-hidden="true">
                    <Bot size={21} />
                  </span>
                  <span className="agent-copy">
                    <strong>{candidate.displayName}</strong>
                    <small>ローカルにインストール済み</small>
                  </span>
                  <Button
                    type="button"
                    disabled={busy}
                    loading={active && busy}
                    onClick={() => onConnect(candidate)}
                  >
                    <PlugZap size={17} /> 接続
                  </Button>
                </article>
              );
            })}
          </div>
        )}
      </section>

      {(authState === "required" || connected) && (
        <section className="setup-section" aria-labelledby="authentication-title">
          <div className="setup-section-heading">
            <div>
              <h2 id="authentication-title">認証</h2>
              <p>
                {connected
                  ? "認証と接続設定を保存しました。"
                  : "エージェントを利用するため、認証を完了してください。"}
              </p>
            </div>
          </div>
          {connected ? (
            <div className="authentication-complete">
              <Alert variant="success">
                <CheckCircle2 size={18} />
                <AlertTitle>エージェントを接続しました</AlertTitle>
                <AlertDescription>Codex ACPを使用できます。</AlertDescription>
              </Alert>
              <Button type="button" size="lg" onClick={onContinue}>
                プロジェクト一覧へ <ChevronRight size={18} />
              </Button>
            </div>
          ) : (
            <div className="auth-methods">
              {authMethods.map((method) => (
                <div className="auth-method" key={method.authMethodId}>
                  <span className="auth-icon" aria-hidden="true">
                    <KeyRound size={20} />
                  </span>
                  <div>
                    <strong>{method.name}</strong>
                    {method.description && <p>{method.description}</p>}
                    {method.type === "terminal" && (
                      <small>ターミナルで認証を完了してください。</small>
                    )}
                  </div>
                  <Button
                    type="button"
                    disabled={busy}
                    onClick={() => onAuthenticate(method.authMethodId)}
                  >
                    {method.type === "terminal" ? "ターミナルで認証" : "認証する"}
                  </Button>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      <p className="setup-status" role="status" aria-live="polite">
        {statusMessage(status)}
      </p>
    </div>
  );
}

function statusMessage(status: SetupConnectionProps["status"]) {
  if (status === "probing") return "エージェントへ接続しています。";
  if (status === "authenticating") return "認証の完了を待っています。";
  if (status === "saving") return "接続を保存しています。";
  return "";
}

function errorTitle(code: string) {
  if (code === "acp_not_compatible") return "このAgentは互換性がありません";
  if (code === "validation_failed") return "接続できません";
  return "処理を完了できませんでした";
}
