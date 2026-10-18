import { Component, type ReactNode } from "react";
import { ErrorPanel } from "@/components/ErrorPanel";

/** Replaces a crashed page with its error (copyable) instead of a blank window. */
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: unknown }> {
  state = { error: null as unknown };

  static getDerivedStateFromError(error: unknown) {
    return { error };
  }

  render() {
    if (this.state.error !== null) {
      return (
        <div className="p-8">
          <ErrorPanel title="Highlighter hit an unexpected error" error={this.state.error}
            onRetry={() => location.reload()} retryLabel="Reload" />
        </div>
      );
    }
    return this.props.children;
  }
}
