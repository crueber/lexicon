import { type Component, createSignal, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { BookOpen } from "lucide-solid";
import { useAuth } from "./AuthProvider";
import { ApiError } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";

const LoginPage: Component = () => {
  const auth = useAuth();
  const navigate = useNavigate();

  const [username, setUsername] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [submitting, setSubmitting] = createSignal(false);

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      await auth.login(username(), password());
      navigate("/", { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.body) as { error?: string };
          setError(body.error ?? "Login failed");
        } catch {
          setError("Login failed");
        }
      } else {
        setError("Unable to connect to server");
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div class="flex min-h-screen items-center justify-center bg-slate-900">
      <div class="w-full max-w-sm px-6">
        <div class="flex flex-col items-center gap-6 mb-8">
          <BookOpen class="h-14 w-14 text-indigo-400" />
          <h1 class="text-3xl font-bold tracking-tight text-slate-100">
            Lexicon
          </h1>
          <p class="text-sm text-slate-400">
            Sign in to your library
          </p>
        </div>

        <form onSubmit={handleSubmit} class="flex flex-col gap-4">
          <Input
            label="Username"
            type="text"
            placeholder="Enter your username"
            value={username()}
            onInput={(e) => setUsername(e.currentTarget.value)}
            autocomplete="username"
            required
          />

          <Input
            label="Password"
            type="password"
            placeholder="Enter your password"
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
            autocomplete="current-password"
            required
          />

          <Show when={error()}>
            <p class="rounded-lg bg-red-950/50 border border-red-500/30 px-3 py-2 text-sm text-red-400">
              {error()}
            </p>
          </Show>

          <Button
            type="submit"
            variant="primary"
            size="lg"
            loading={submitting()}
            class="mt-2 w-full"
          >
            Sign in
          </Button>
        </form>
      </div>
    </div>
  );
};

export default LoginPage;
