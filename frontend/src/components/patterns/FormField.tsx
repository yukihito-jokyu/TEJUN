import { useId, type ReactNode } from "react";
import { Label } from "@/components/ui/Label";
export type FieldBinding = {
  id: string;
  "aria-describedby"?: string;
  "aria-invalid"?: true;
  required?: boolean;
};

export function FormField({
  label,
  description,
  error,
  required,
  children,
}: {
  label: string;
  description?: string;
  error?: string;
  required?: boolean;
  children: (props: FieldBinding) => ReactNode;
}) {
  const id = useId();

  const described =
    [description ? `${id}-help` : null, error ? `${id}-error` : null].filter(Boolean).join(" ") ||
    undefined;

  return (
    <div className="field">
      <Label htmlFor={id}>
        {label}
        {required ? "（必須）" : ""}
      </Label>
      {children({
        id,
        "aria-describedby": described,
        "aria-invalid": error ? true : undefined,
        required,
      })}
      {description && (
        <p id={`${id}-help`} className="note">
          {description}
        </p>
      )}
      {error && (
        <p id={`${id}-error`} className="error">
          {error}
        </p>
      )}
    </div>
  );
}
