import { afterAll, describe, expect, it } from 'vitest';
import { spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { openEventStream } from '../lib/background-logic';

const GO_SERVER_PATH = 'test/go/sse_auth_server.go';

async function startGoSSEServer(onSpawn: (child: ChildProcess) => void): Promise<{ baseUrl: string }> {
  const start = Date.now();
  const child = spawn('go', ['run', GO_SERVER_PATH], {
    cwd: process.cwd(),
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  onSpawn(child);

  let stdout = '';
  let stderr = '';

  child.stdout?.setEncoding('utf8');
  child.stderr?.setEncoding('utf8');
  child.stdout?.on('data', (chunk: string) => {
    stdout += chunk;
  });
  child.stderr?.on('data', (chunk: string) => {
    stderr += chunk;
  });

  const started = await new Promise<string>((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`timed out starting Go SSE server after ${Date.now() - start}ms\nstdout:\n${stdout}\nstderr:\n${stderr}`));
    }, 30_000);

    const onData = (chunk: string) => {
      stdout += chunk;
      const match = stdout.match(/READY (http:\/\/127\.0\.0\.1:\d+)/);
      if (match) {
        clearTimeout(timeout);
        child.stdout?.off('data', onData);
        console.log(`Go SSE server started in ${Date.now() - start}ms: ${match[1]}`);
        resolve(match[1]);
      }
    };

    child.stdout?.on('data', onData);
    child.once('exit', (code, signal) => {
      clearTimeout(timeout);
      reject(new Error(`Go SSE server exited before startup (code=${code}, signal=${signal})\nstdout:\n${stdout}\nstderr:\n${stderr}`));
    });
  });

  return { baseUrl: started };
}

async function stopChild(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null || child.signalCode !== null) return;
  child.kill('SIGTERM');
  await once(child, 'exit');
}

describe('background SSE integration with Go server', () => {
  let child: ChildProcess | undefined;

  afterAll(async () => {
    if (child) await stopChild(child);
  });

  it('requires bearer auth and streams encoded events', async () => {
    const started = await startGoSSEServer((c) => {
      child = c;
    });

    const noAuthResp = await openEventStream(started.baseUrl, null, new AbortController().signal);
    expect(noAuthResp.status).toBe(401);

    const authedResp = await openEventStream(started.baseUrl, 'test-token', new AbortController().signal);
    expect(authedResp.status).toBe(200);
    expect(authedResp.headers.get('content-type')).toContain('text/event-stream');

    const body = await authedResp.text();
    expect(body).toContain('event: queued');
    expect(body).toContain('"DownloadID":"queue-1"');
    expect(body).toContain('"Filename":"archive.zip"');
  }, 60_000);
});
