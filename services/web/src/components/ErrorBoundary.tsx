import { Component, type ErrorInfo, type ReactNode } from 'react';

interface State {
  error: Error | null;
}

/** Catches render/runtime errors so a malformed event never blanks the page. */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('MediaForge UI crashed:', error, info.componentStack);
  }

  render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return (
      <div className="boundary">
        <div className="eyebrow">Runtime fault</div>
        <h2>The control room hit an error.</h2>
        <p>{this.state.error.message}</p>
        <button className="forge-btn" onClick={() => location.reload()}>
          Reload ▸
        </button>
      </div>
    );
  }
}
