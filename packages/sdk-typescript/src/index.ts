export interface CouplerTool {
  name: string;
  description: string;
  connector: string;
  parameters?: Record<string, unknown>;
}

export interface ExecuteRequest {
  tool: string;
  arguments?: Record<string, unknown>;
  dry_run?: boolean;
  connection_id?: string;
}

export interface ExecuteResult {
  dry_run?: boolean;
  tool?: string;
  error?: string;
  message?: string;
  [key: string]: unknown;
}

export interface CouplerClientOptions {
  baseUrl: string;
  getAccessToken?: () => Promise<string | undefined>;
}

export class CouplerClient {
  constructor(private readonly opts: CouplerClientOptions) {}

  async listTools(): Promise<CouplerTool[]> {
    const res = await this.fetch("/v1/tools");
    const data = (await res.json()) as { tools: CouplerTool[] };
    return data.tools;
  }

  async searchTools(query: string): Promise<CouplerTool[]> {
    const res = await this.fetch(`/v1/tools/search?q=${encodeURIComponent(query)}`);
    const data = (await res.json()) as { tools: CouplerTool[] };
    return data.tools;
  }

  async execute(req: ExecuteRequest): Promise<ExecuteResult> {
    const res = await this.fetch("/v1/tools/execute", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ dry_run: true, ...req }),
    });
    return (await res.json()) as ExecuteResult;
  }

  private async fetch(path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    if (this.opts.getAccessToken) {
      const token = await this.opts.getAccessToken();
      if (token) headers.set("Authorization", `Bearer ${token}`);
    }
    const url = `${this.opts.baseUrl.replace(/\/$/, "")}${path}`;
    const res = await fetch(url, { ...init, headers });
    if (!res.ok) {
      throw new Error(`Coupler API ${res.status}: ${await res.text()}`);
    }
    return res;
  }
}
