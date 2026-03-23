import { type Component, type JSX, mergeProps, splitProps, Show } from "solid-js";
import { Loader2 } from "lucide-solid";

export interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
}

const variantClasses: Record<string, string> = {
  primary:
    "bg-indigo-600 text-white hover:bg-indigo-500 focus-visible:ring-indigo-500",
  secondary:
    "bg-slate-700 text-slate-100 hover:bg-slate-600 focus-visible:ring-slate-500",
  ghost:
    "bg-transparent text-slate-300 hover:bg-slate-800 hover:text-slate-100 focus-visible:ring-slate-500",
  danger:
    "bg-red-600 text-white hover:bg-red-500 focus-visible:ring-red-500",
};

const sizeClasses: Record<string, string> = {
  sm: "px-3 py-1.5 text-sm",
  md: "px-4 py-2 text-sm",
  lg: "px-6 py-3 text-base",
};

const Button: Component<ButtonProps> = (rawProps) => {
  const merged = mergeProps(
    { variant: "primary" as const, size: "md" as const, loading: false },
    rawProps,
  );
  const [local, rest] = splitProps(merged, [
    "variant",
    "size",
    "loading",
    "children",
    "class",
    "disabled",
  ]);

  return (
    <button
      {...rest}
      disabled={local.disabled || local.loading}
      class={`inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900 disabled:cursor-not-allowed disabled:opacity-50 ${variantClasses[local.variant]} ${sizeClasses[local.size]} ${local.class ?? ""}`}
    >
      <Show when={local.loading}>
        <Loader2 class="h-4 w-4 animate-spin" />
      </Show>
      {local.children}
    </button>
  );
};

export default Button;
