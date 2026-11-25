// The gotest_audit language model tool: the active folder's audit as the CLI
// writes it in Markdown, so an agent in the editor reads the document the
// Change scope shows. It goes through the Change scope's one audit slot; an
// audit that runs tests asks the user first.
import * as vscode from "vscode";

export const AUDIT_TOOL = "gotest_audit";

export interface AuditToolInput {
  base?: string;
  run?: boolean;
}

export interface AuditToolDeps {
  // The folder the tool audits: the active editor's gotest folder, else the
  // first.
  folder: () => string | undefined;
  audit: (folder: string, input: AuditToolInput) => Promise<string>;
}

// readInput keeps what the schema allows of the model's input.
export function readInput(raw: unknown): AuditToolInput {
  const v = (raw && typeof raw === "object" ? raw : {}) as Record<
    string,
    unknown
  >;
  return {
    base:
      typeof v.base === "string" && v.base.trim() ? v.base.trim() : undefined,
    run: v.run === true,
  };
}

export class AuditTool implements vscode.LanguageModelTool<AuditToolInput> {
  constructor(private readonly deps: AuditToolDeps) {}

  prepareInvocation(
    options: vscode.LanguageModelToolInvocationPrepareOptions<AuditToolInput>,
  ): vscode.PreparedToolInvocation {
    const input = readInput(options.input);
    const against = input.base ? ` against ${input.base}` : "";
    if (!input.run)
      return { invocationMessage: `Auditing the test change${against}` };
    return {
      invocationMessage: `Auditing the test change${against}, running its tests`,
      confirmationMessages: {
        title: "Run tests for the audit?",
        message: new vscode.MarkdownString(
          `The audit runs the packages of the changed behaviors${against}, with coverage, as \`gotest audit --run\` does. Their fixtures start and tear down as in any run.`,
        ),
      },
    };
  }

  async invoke(
    options: vscode.LanguageModelToolInvocationOptions<AuditToolInput>,
    _token?: vscode.CancellationToken,
  ): Promise<vscode.LanguageModelToolResult> {
    const folder = this.deps.folder();
    const text = folder
      ? await this.deps.audit(folder, readInput(options.input))
      : "No workspace folder uses gotest, so there is nothing to audit.";
    return new vscode.LanguageModelToolResult([
      new vscode.LanguageModelTextPart(text),
    ]);
  }
}

// registerAuditTool registers the tool where the editor has the API; an
// older one goes without it.
export function registerAuditTool(
  deps: AuditToolDeps,
): vscode.Disposable | undefined {
  const lm = (vscode as { lm?: typeof vscode.lm }).lm;
  if (typeof lm?.registerTool !== "function") return undefined;
  return lm.registerTool(AUDIT_TOOL, new AuditTool(deps));
}
