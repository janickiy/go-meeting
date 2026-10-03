import { Component, type ReactNode } from "react";
import { reportClientError } from "../clientTelemetry";

/**
 * Перехватывает отказ отрисовки приложения и предлагает безопасное восстановление.
 * @params: children — дерево интерфейса; failed — состояние аварийного экрана.
 */
export class AppErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  /** @return состояние аварийного экрана без сохранения объекта ошибки. */
  static getDerivedStateFromError() {
    return { failed: true };
  }

  /**
   * Регистрирует обезличенный отказ React, не передавая componentStack.
   * @args error — пойманная ошибка только для извлечения разрешённых координат.
   */
  componentDidCatch(error: Error) {
    reportClientError("react_render_error", error);
  }

  /** @return дерево приложения либо доступное сообщение с перезагрузкой страницы. */
  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <main className="page-center">
        <div className="content-card" role="alert">
          <h1>Не удалось открыть страницу</h1>
          <p>
            Попробуйте обновить страницу. Если ошибка повторяется, обратитесь к
            администратору.
          </p>
          <button
            className="button button-primary"
            onClick={() => window.location.reload()}
          >
            Обновить страницу
          </button>
        </div>
      </main>
    );
  }
}
