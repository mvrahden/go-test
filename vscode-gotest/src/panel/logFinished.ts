// logFinished notes how long a read of the CLI took and how it exited.
export function logFinished(
  label: string,
  startedAt: number,
  code: number | null,
  log: { info(message: string): void },
): void {
  log.info(
    `[${label}] finished in ${Date.now() - startedAt}ms, exit ${code ?? "signal"}`,
  );
}
