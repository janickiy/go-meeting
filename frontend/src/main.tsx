import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@fontsource/inter/latin-400.css";
import "@fontsource/inter/latin-500.css";
import "@fontsource/inter/latin-600.css";
import "@fontsource/inter/latin-700.css";
import "@fontsource/inter/cyrillic-400.css";
import "@fontsource/inter/cyrillic-500.css";
import "@fontsource/inter/cyrillic-600.css";
import "@fontsource/inter/cyrillic-700.css";
import "./styles.css";
import "./accessibility.css";
import "./tokens.css";
import "./workspace.css";
import { PRODUCT_NAME } from "./brand";
import { ApiError } from "./api";
import { AuthProvider } from "./auth";
import { App } from "./App";
import { AppErrorBoundary } from "./components/AppErrorBoundary";
import { installClientTelemetry } from "./clientTelemetry";

const stopTelemetry = installClientTelemetry();
document.title = `${PRODUCT_NAME} — встречи без границ`;
if (import.meta.hot) import.meta.hot.dispose(stopTelemetry);

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10000,
      /**
       * retry решает, допустим ли повтор запроса с учётом ошибки и числа отказов.
       *
       * @args
       *   - count — число уже выполненных попыток.
       *   - error — пойманная ошибка API или сети.
       *
       * @returns вычисленное значение: !( error instanceof ApiError && error.status >= 400 && error.status < 500 ) && count < 1.
       */
      retry: (count, error) =>
        !(
          error instanceof ApiError &&
          error.status >= 400 &&
          error.status < 500
        ) && count < 1,
    },
    mutations: { retry: false },
  },
});
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AppErrorBoundary>
      <BrowserRouter>
        <QueryClientProvider client={queryClient}>
          <AuthProvider>
            <App />
          </AuthProvider>
        </QueryClientProvider>
      </BrowserRouter>
    </AppErrorBoundary>
  </StrictMode>,
);
