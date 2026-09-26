# step-form の使用例

選択条件と状態は[仕様](../../patterns/step-form.md)を参照する。

```tsx
import { StepForm } from "@/components/patterns/step-form";
<StepForm
  title="申請"
  steps={steps}
  currentStep={step}
  onNext={validateAndNext}
  onBack={goBack}
  onSubmit={submit}
  submitting={submitting}
/>;
```

`StepForm`に画面の見出し、必要なデータとcallbackを渡す。通信、認可、ルーティングは利用アプリで実装する。
