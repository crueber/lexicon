import { type Component, type JSX, Show } from "solid-js";
import { Navigate } from "@solidjs/router";
import { Loader2 } from "lucide-solid";
import { useAuth } from "./AuthProvider";

const ProtectedRoute: Component<{ children: JSX.Element }> = (props) => {
  const auth = useAuth();

  return (
    <Show
      when={!auth.loading()}
      fallback={
        <div class="flex min-h-screen items-center justify-center bg-slate-900">
          <Loader2 class="h-8 w-8 animate-spin text-indigo-400" />
        </div>
      }
    >
      <Show when={auth.isAuthenticated()} fallback={<Navigate href="/login" />}>
        {props.children}
      </Show>
    </Show>
  );
};

export default ProtectedRoute;
