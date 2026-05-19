// Button — variants per boilerplate `.btn` rules.
// variant: default | primary | accent | ghost | danger
// size: sm | md (default) | xs
// loading state replaces leading content with a spinner.
//
// Button —— 按 boilerplate .btn 规则做变体；loading 时以 spinner 替换前置 icon。

import { forwardRef } from "react";
import { Spinner } from "./Spinner.jsx";

export const Button = forwardRef(function Button(
  { variant = "default", size = "md", loading, disabled, className = "", children, ...rest },
  ref
) {
  const cls = [
    "btn",
    variant === "primary" && "btn-primary",
    variant === "accent" && "btn-accent",
    variant === "ghost" && "btn-ghost",
    variant === "danger" && "btn-danger",
    size === "sm" && "btn-sm",
    size === "xs" && "btn-xs",
    loading && "is-loading",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button ref={ref} className={cls} disabled={disabled || loading} {...rest}>
      {loading && <Spinner size={12} />}
      {children}
    </button>
  );
});
