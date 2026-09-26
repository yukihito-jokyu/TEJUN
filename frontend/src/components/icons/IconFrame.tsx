import type { ReactNode } from "react";
export type IconProps = { className?: string; label?: string; size?: number };

export function IconFrame({
  size = 48,
  label = "",
  className,
  children,
}: IconProps & { children: ReactNode }) {
  if (!Number.isFinite(size) || size < 12 || size > 512)
    throw new Error("Icon size must be 12–512");
  return (
    <span className={["ds-art-icon", className].filter(Boolean).join(" ")}>
      <svg
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 96 96"
        width={size}
        height={size}
        focusable="false"
        role={label ? "img" : undefined}
        aria-label={label || undefined}
        aria-hidden={label ? undefined : true}
      >
        {label ? <title>{label}</title> : null}
        {children}
      </svg>
    </span>
  );
}
