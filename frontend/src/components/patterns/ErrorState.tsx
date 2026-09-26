import { Alert, AlertDescription, AlertTitle } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";

export function ErrorState({
  title = "処理を完了できませんでした",
  description,
  onRetry,
}: {
  title?: string;
  description: string;
  onRetry?: () => void;
}) {
  return (
    <Alert variant="destructive">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <p>{description}</p>
        {onRetry && (
          <Button type="button" variant="outline" onClick={onRetry}>
            再試行
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}
