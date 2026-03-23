import { type Component, type JSX, splitProps, Show } from "solid-js";

export interface InputProps extends JSX.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

const Input: Component<InputProps> = (rawProps) => {
  const [local, rest] = splitProps(rawProps, ["label", "error", "class", "id"]);

  // Generate a stable id for label association if not provided.
  const inputId = local.id ?? `input-${Math.random().toString(36).slice(2, 9)}`;

  return (
    <div class="flex flex-col gap-1.5">
      <Show when={local.label}>
        <label
          for={inputId}
          class="text-sm font-medium text-slate-300"
        >
          {local.label}
        </label>
      </Show>
      <input
        {...rest}
        id={inputId}
        class={`rounded-lg border bg-slate-800 px-3 py-2 text-sm text-slate-100 placeholder-slate-500 transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-slate-900 ${
          local.error
            ? "border-red-500 focus:ring-red-500"
            : "border-slate-700 focus:ring-indigo-500"
        } ${local.class ?? ""}`}
      />
      <Show when={local.error}>
        <p class="text-sm text-red-400">{local.error}</p>
      </Show>
    </div>
  );
};

export default Input;
