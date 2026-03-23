import { type Component, createSignal, For } from "solid-js";
import { CheckCircle, XCircle, Info, X } from "lucide-solid";

export type ToastVariant = "success" | "error" | "info";

interface Toast {
  id: number;
  message: string;
  variant: ToastVariant;
}

// Signal-based toast store.
const [toasts, setToasts] = createSignal<Toast[]>([]);
let nextId = 0;

export function showToast(message: string, variant: ToastVariant = "info"): void {
  const id = nextId++;
  setToasts((prev) => [...prev, { id, message, variant }]);

  // Auto-dismiss after 4 seconds.
  setTimeout(() => {
    dismissToast(id);
  }, 4000);
}

function dismissToast(id: number): void {
  setToasts((prev) => prev.filter((t) => t.id !== id));
}

const variantStyles: Record<ToastVariant, string> = {
  success: "border-emerald-500/50 bg-emerald-950/90 text-emerald-100",
  error: "border-red-500/50 bg-red-950/90 text-red-100",
  info: "border-indigo-500/50 bg-indigo-950/90 text-indigo-100",
};

const variantIcons: Record<ToastVariant, Component<{ class?: string }>> = {
  success: CheckCircle,
  error: XCircle,
  info: Info,
};

const ToastContainer: Component = () => {
  return (
    <div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      <For each={toasts()}>
        {(toast) => {
          const Icon = variantIcons[toast.variant];
          return (
            <div
              class={`flex items-center gap-3 rounded-lg border px-4 py-3 shadow-lg backdrop-blur-sm ${variantStyles[toast.variant]}`}
            >
              <Icon class="h-5 w-5 shrink-0" />
              <span class="text-sm">{toast.message}</span>
              <button
                onClick={() => dismissToast(toast.id)}
                class="ml-2 shrink-0 rounded p-0.5 opacity-70 hover:opacity-100 transition-opacity"
              >
                <X class="h-4 w-4" />
              </button>
            </div>
          );
        }}
      </For>
    </div>
  );
};

export default ToastContainer;
