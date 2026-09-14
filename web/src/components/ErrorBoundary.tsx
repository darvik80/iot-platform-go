import { Component, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, errorInfo: { componentStack?: string | null }) {
    console.error('=== ErrorBoundary caught ===');
    console.error('Error message:', error.message);
    console.error('Error name:', error.name);
    console.error('Component stack:', errorInfo.componentStack);
    // Try to find the actual object being rendered
    console.error('Full error:', JSON.stringify(error, Object.getOwnPropertyNames(error)));
  }

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div style={{ padding: '2rem', color: '#f07178', fontFamily: 'monospace' }}>
          <h2>React Error</h2>
          <pre>{this.state.error.message}</pre>
          <pre style={{ fontSize: '0.8rem', whiteSpace: 'pre-wrap' }}>
            {this.state.error.stack}
          </pre>
          <button
            onClick={() => {
              this.setState({ error: null });
              location.reload();
            }}
            style={{
              marginTop: '1rem',
              padding: '0.5rem 1rem',
              background: '#3d9cf0',
              color: '#fff',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            Reload
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
