import { useCallback, useEffect, useRef, useState } from "react";
import { Navigate, Route, Routes, useNavigate } from "react-router-dom";

import type { AgentConnectionInput } from "@/features/setup-connection/ui/SetupConnection";
import { SetupPage } from "@/pages/setup/SetupPage";
import { ProjectListPage } from "@/pages/projects/ProjectListPage";
import {
  authenticateAgent,
  checkAuthentication,
  completeInitialSetup,
  getStartupState,
  listAgentCandidates,
  onAppEvent,
  onSystemWake,
  parseAppError,
  type AgentCandidate,
  type AgentProbe,
} from "@/shared/api/wails";

type Status = "ready" | "probing" | "authenticating" | "saving";
type ViewError = { code: string; message: string; retryable: boolean };

export function App() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [candidates, setCandidates] = useState<AgentCandidate[]>([]);
  const [connection, setConnection] = useState<AgentConnectionInput>();
  const [probe, setProbe] = useState<AgentProbe>();
  const [nextRoute, setNextRoute] = useState<string>();
  const [status, setStatus] = useState<Status>("ready");
  const [error, setError] = useState<ViewError>();
  const connectionRef = useRef(connection);
  const probeRef = useRef(probe);
  const statusRef = useRef(status);
  const started = useRef(false);
  const foregroundOperation = useRef(0);
  const startupSnapshot = useRef(0);
  const completionRoute = useRef<string | undefined>(undefined);
  const retry = useRef<() => void>(() => {});

  useEffect(() => {
    connectionRef.current = connection;
    probeRef.current = probe;
    statusRef.current = status;
  }, [connection, probe, status]);

  const fail = useCallback((cause: unknown, generation: number, retryOperation: () => void) => {
    if (generation !== foregroundOperation.current) return;
    const appError = parseAppError(cause);
    retry.current = retryOperation;
    setError(
      appError ?? { code: "internal", message: "予期しないエラーが発生しました", retryable: true },
    );
    setStatus("ready");
  }, []);

  const completeConnection = useCallback(
    async (input: AgentConnectionInput, probeId: string) => {
      const generation = ++foregroundOperation.current;
      setError(undefined);
      setStatus("saving");
      try {
        const result = await completeInitialSetup(input, probeId, crypto.randomUUID());
        if (generation === foregroundOperation.current) {
          completionRoute.current = result.data.nextRoute;
          setNextRoute(result.data.nextRoute);
          setStatus("ready");
        }
      } catch (cause) {
        fail(cause, generation, () => void completeConnection(input, probeId));
      }
    },
    [fail],
  );

  const loadStartup = useCallback(
    async (showSetup = false): Promise<boolean> => {
      const generation = ++startupSnapshot.current;
      setError(undefined);
      try {
        const startup = await getStartupState();
        if (generation !== startupSnapshot.current) return false;
        if (showSetup && !completionRoute.current) {
          await navigate(
            startup.initialSetupRequired ? "/setup" : startup.nextRoute.replace(/^#/, ""),
            {
              replace: true,
            },
          );
        }
        if (generation !== startupSnapshot.current) return false;
        if (showSetup || startup.initialSetupRequired) {
          const found = await listAgentCandidates(false);
          if (generation !== startupSnapshot.current) return false;
          setCandidates(found);
        }
        setLoading(false);
        return true;
      } catch (cause) {
        if (generation !== startupSnapshot.current) return false;
        const appError = parseAppError(cause);
        retry.current = () => void loadStartup(showSetup);
        setError(
          appError ?? {
            code: "internal",
            message: "予期しないエラーが発生しました",
            retryable: true,
          },
        );
        if (statusRef.current === "ready") setStatus("ready");
        return false;
      }
    },
    [navigate],
  );

  const runProbe = useCallback(
    async (input: AgentConnectionInput) => {
      const generation = ++foregroundOperation.current;
      setError(undefined);
      setStatus("probing");
      try {
        const result = await checkAuthentication(input, crypto.randomUUID());
        if (generation !== foregroundOperation.current) return;
        setConnection(input);
        setProbe(result);
        if (result.authState === "not_required" || result.authState === "authenticated") {
          await completeConnection(input, result.probeId);
        } else {
          setStatus("ready");
        }
      } catch (cause) {
        fail(cause, generation, () => void runProbe(input));
      }
    },
    [completeConnection, fail],
  );

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    loadStartup(true).catch(() => undefined);
  }, [loadStartup]);

  useEffect(() => {
    const unsubscribeEvent = onAppEvent(
      (event) => {
        const activeProbe = probeRef.current;
        if (
          event.name === "agent.authentication.updated" &&
          event.aggregateType === "agent_job" &&
          statusRef.current === "authenticating" &&
          activeProbe &&
          activeProbe.probeId === event.payload.connectionId
        ) {
          if (event.payload.authState === "succeeded" && connectionRef.current) {
            statusRef.current = "saving";
            completeConnection(connectionRef.current, activeProbe.probeId).catch(() => undefined);
            return;
          }
          if (event.payload.authState === "failed") {
            setStatus("ready");
            setError({
              code: "authentication_failed",
              message: "認証に失敗しました。もう一度お試しください。",
              retryable: true,
            });
            retry.current = () => {
              if (connectionRef.current) runProbe(connectionRef.current).catch(() => undefined);
            };
            return;
          }
        }
        loadStartup().catch(() => undefined);
      },
      () => void loadStartup(),
    );
    const unsubscribeWake = onSystemWake(() => void loadStartup());
    return () => {
      unsubscribeEvent();
      unsubscribeWake();
    };
  }, [completeConnection, loadStartup, runProbe]);

  function refresh() {
    const generation = ++foregroundOperation.current;
    setError(undefined);
    setLoading(true);
    listAgentCandidates(true)
      .then((found) => {
        if (generation !== foregroundOperation.current) return;
        setCandidates(found);
        setLoading(false);
      })
      .catch((cause) => fail(cause, generation, refresh));
  }

  function authenticate(authMethodId: string) {
    if (!probe) return;
    const generation = ++foregroundOperation.current;
    setError(undefined);
    setStatus("authenticating");
    authenticateAgent(probe.probeId, authMethodId, crypto.randomUUID()).catch((cause) =>
      fail(cause, generation, () => authenticate(authMethodId)),
    );
  }

  return (
    <Routes>
      <Route
        path="/setup"
        element={
          <SetupPage
            key={probe?.probeId ?? "setup"}
            loading={loading}
            candidates={candidates}
            selected={connection}
            status={status}
            authState={
              probe?.authState === "unknown" && probe.authMethods.length > 0
                ? "required"
                : probe?.authState
            }
            authMethods={probe?.authMethods}
            error={error}
            connected={nextRoute !== undefined}
            onRefresh={refresh}
            onConnect={(candidate) => {
              runProbe({ ...candidate, environmentOverrides: [] }).catch(() => undefined);
            }}
            onAuthenticate={authenticate}
            onContinue={() =>
              void navigate((nextRoute ?? "/projects").replace(/^#/, ""), { replace: true })
            }
            onRetry={() => retry.current()}
          />
        }
      />
      <Route path="/projects" element={<ProjectListPage />} />
      <Route path="/projects/:projectId/*" element={<UnimplementedProjectStage />} />
      <Route path="*" element={<Navigate to="/setup" replace />} />
    </Routes>
  );
}

function UnimplementedProjectStage() {
  return (
    <main aria-label="未実装の工程">
      <h1>この工程はまだ利用できません</h1>
      <p>プロジェクトは保存されています。一覧から作業を確認できます。</p>
      <a href="#/projects">プロジェクト一覧へ戻る</a>
    </main>
  );
}
