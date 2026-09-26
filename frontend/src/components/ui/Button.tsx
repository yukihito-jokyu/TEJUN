import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/shared/lib/utils";
import { Slot } from "radix-ui";

const buttonVariants = cva("ds-button", {
  variants: {
    variant: {
      default: "bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-active",
      destructive: "bg-destructive text-destructive-foreground",
      outline: "bg-surface text-foreground hover:bg-accent",
      secondary: "bg-secondary text-secondary-foreground hover:bg-secondary-hover",
      ghost: "text-foreground hover:bg-accent",
      link: "text-foreground underline underline-offset-4",
    },
    size: {
      default: "ds-size-md",
      sm: "ds-size-sm",
      lg: "ds-size-lg",
      icon: "ds-size-md ds-icon-button",
      "icon-sm": "ds-size-sm ds-icon-button",
      "icon-lg": "ds-size-lg ds-icon-button",
    },
  },
  defaultVariants: {
    variant: "default",
    size: "default",
  },
});

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  loading = false,
  disabled,
  onClick,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    loading?: boolean;
    asChild?: boolean;
  }) {
  const Comp = asChild ? Slot.Root : "button";

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
      disabled={asChild ? undefined : disabled || loading}
      aria-disabled={disabled || loading || undefined}
      aria-busy={loading || undefined}
      onClick={(event) => {
        if (disabled || loading) {
          event.preventDefault();
          event.stopPropagation();
          return;
        }
        onClick?.(event);
      }}
    />
  );
}

export { Button, buttonVariants };
