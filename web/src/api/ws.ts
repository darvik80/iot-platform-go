type Handler = (msg: { event?: string; data?: unknown }) => void;

export class PlatformWS {
  private ws: WebSocket | null = null;
  private handlers = new Set<Handler>();
  private timer: number | null = null;
  private token = '';

  connect(token: string) {
    this.close();
    this.token = token;
    if (!token) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${location.host}/api/v1/ws?token=${encodeURIComponent(token)}`);
    this.ws = ws;
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data);
        this.handlers.forEach((h) => h(msg));
      } catch {
        /* ignore */
      }
    };
    ws.onclose = () => {
      if (!this.token) return;
      this.timer = window.setTimeout(() => this.connect(this.token), 3000);
    };
  }

  subscribe(handler: Handler) {
    this.handlers.add(handler);
    return () => {
      this.handlers.delete(handler);
    };
  }

  close() {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.token = '';
    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
      this.ws = null;
    }
  }
}

export const platformWS = new PlatformWS();
