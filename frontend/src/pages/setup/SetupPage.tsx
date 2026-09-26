import { LoadingState } from "@/components/patterns/LoadingState";
import type { SetupConnectionProps } from "@/features/setup-connection/ui/SetupConnection";
import { SetupConnection } from "@/features/setup-connection/ui/SetupConnection";
import { AppHeader } from "@/widgets/app-header/AppHeader";
import "./SetupPage.css";

export type SetupPageProps = SetupConnectionProps & { loading?: boolean };

export function SetupPage({ loading = false, ...props }: SetupPageProps) {
  return (
    <main className="setup-page" aria-labelledby="setup-title">
      <AppHeader title="TEJUN" badge="初回セットアップ" />
      <div className="setup-shell">
        <div className="setup-heading">
          <p className="setup-eyebrow">Agent Client Protocol</p>
          <h1 id="setup-title">AIエージェントを接続</h1>
          <p>このアプリで使用するACP対応エージェントを選び、認証状態を確認します。</p>
        </div>
        <LoadingState loading={loading} initial label="接続候補を読み込んでいます…">
          <SetupConnection {...props} />
        </LoadingState>
      </div>
    </main>
  );
}
