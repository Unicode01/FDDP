import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import net from "node:net";

const demoRoot = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(demoRoot, "..", "..");
const backendDir = join(demoRoot, "backend");
const frontendDir = join(demoRoot, "frontend");
const sdkCli = join(repoRoot, "packages", "nextjs-sdk", "dist", "cli.js");
const tscCli = join(frontendDir, "node_modules", "typescript", "bin", "tsc");

if (!existsSync(sdkCli)) {
  throw new Error("Missing packages/nextjs-sdk/dist/cli.js. Run `cd packages/nextjs-sdk && npm install && npm run build` first.");
}
if (!existsSync(tscCli)) {
  throw new Error("Missing demo TypeScript dependency. Run `cd examples/demo/frontend && npm install` first.");
}

const tempDir = await mkdtemp(join(tmpdir(), "fddp-demo-smoke-"));
const backendExe = join(tempDir, process.platform === "win32" ? "fddp-demo-backend.exe" : "fddp-demo-backend");
const port = await findFreePort();
const baseUrl = `http://localhost:${port}`;
let backend;

try {
  await run("go", ["build", "-o", backendExe, "."], { cwd: backendDir });

  backend = spawn(backendExe, [], {
    cwd: backendDir,
    env: { ...process.env, PORT: String(port) },
    stdio: ["ignore", "pipe", "pipe"]
  });

  const stderr = [];
  backend.stderr.on("data", (chunk) => stderr.push(String(chunk)));
  backend.stdout.on("data", () => {});

  await waitForContract(baseUrl, stderr);
  await run(process.execPath, [sdkCli, "--input", `${baseUrl}/contract`, "--output", "src/fddp.generated.ts"], {
    cwd: frontendDir
  });
  await run(process.execPath, [tscCli, "--noEmit"], { cwd: frontendDir });

  await assertQuery(baseUrl);
  await assertCommand(baseUrl);
  await assertUnsafeQueryRejected(baseUrl);

  console.log(`FDDP demo smoke test passed on ${baseUrl}`);
} finally {
  if (backend && !backend.killed) {
    backend.kill();
    await waitForExit(backend);
  }
  await rmRetry(tempDir);
}

async function assertQuery(baseUrl) {
  const body = await postJson(`${baseUrl}/data/query`, {
    query: {
      me: {
        profile: ["id", "name", "description"]
      },
      global: {
        config: ["appName"]
      },
      project: {
        list: {
          $type: "collection",
          args: {
            first: 2,
            filter: { status: { eq: "active" } },
            orderBy: [{ field: "updatedAt", direction: "desc" }]
          },
          selection: {
            fields: ["id", "name", "updatedAt"],
            expand: { owner: ["id", "name"] }
          }
        }
      }
    },
    trace: true
  });

  assertNoErrors(body, "query");
  assert(body.data?.me?.profile?.name === "Tom", "expected me.profile.name to be Tom");
  assert(body.data?.global?.config?.appName === "FDDP Demo", "expected global.config.appName");
  const items = body.data?.project?.list?.items;
  assert(Array.isArray(items) && items.length > 0, "expected project.list items");
  assert(typeof items[0]?.owner?.name === "string", "expected project owner expand");
}

async function assertCommand(baseUrl) {
  const idempotencyKey = `smoke_${Date.now()}`;
  const body = await postJson(`${baseUrl}/command/execute`, {
    command: "user.profile.update",
    input: { displayName: "Smoke Tom" },
    idempotencyKey
  });

  assertNoErrors(body, "command");
  assert(body.data?.status === "completed", "expected command status completed");

  const after = await postJson(`${baseUrl}/data/query`, {
    query: { me: { profile: ["name"] } }
  });
  assertNoErrors(after, "query after command");
  assert(after.data?.me?.profile?.name === "Smoke Tom", "expected command to update profile name");
}

async function assertUnsafeQueryRejected(baseUrl) {
  const body = await postJson(`${baseUrl}/data/query`, {
    query: {
      project: {
        list: {
          $type: "collection",
          args: {
            filter: {
              name: { raw: "name = name; drop table users" }
            }
          },
          selection: {
            fields: ["id", "name"]
          }
        }
      }
    }
  });

  const codes = (body.errors ?? []).map((error) => error.code);
  assert(codes.includes("UNSUPPORTED_FILTER"), `expected UNSUPPORTED_FILTER, got ${codes.join(",") || "none"}`);
}

async function postJson(url, body) {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-DDP-Subject": "user_123",
      "X-DDP-Tenant": "tenant_abc",
      "X-DDP-Permission-Version": "perm_v17"
    },
    body: JSON.stringify(body)
  });

  const text = await response.text();
  assert(response.ok, `HTTP ${response.status}: ${text}`);
  return JSON.parse(text);
}

async function waitForContract(baseUrl, stderr) {
  for (let attempt = 0; attempt < 80; attempt++) {
    if (backend.exitCode !== null) {
      throw new Error(`Backend exited early with ${backend.exitCode}: ${stderr.join("")}`);
    }
    try {
      const response = await fetch(`${baseUrl}/contract`);
      if (response.ok) {
        const contract = await response.json();
        if (contract.contractVersion === "contract_demo_gorm_v1" && contract.resources?.[0]?.fields?.length) {
          return;
        }
      }
    } catch {
      // Backend is still starting.
    }
    await sleep(250);
  }
  throw new Error(`Backend did not publish the expected contract on ${baseUrl}/contract`);
}

async function run(command, args, options) {
  await new Promise((resolvePromise, reject) => {
    let child;
    try {
      child = spawn(command, args, {
        ...options,
        stdio: ["ignore", "pipe", "pipe"]
      });
    } catch (error) {
      reject(error);
      return;
    }
    let output = "";
    child.stdout.on("data", (chunk) => {
      output += String(chunk);
    });
    child.stderr.on("data", (chunk) => {
      output += String(chunk);
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolvePromise();
      } else {
        reject(new Error(`${command} ${args.join(" ")} failed with ${code}\n${output}`));
      }
    });
  });
}

async function findFreePort() {
  return await new Promise((resolvePromise, reject) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => resolvePromise(address.port));
    });
    server.on("error", reject);
  });
}

function assertNoErrors(body, label) {
  const errors = body.errors ?? [];
  assert(errors.length === 0, `${label} returned errors: ${JSON.stringify(errors)}`);
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sleep(ms) {
  return new Promise((resolvePromise) => setTimeout(resolvePromise, ms));
}

async function waitForExit(child) {
  if (child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  await new Promise((resolvePromise) => {
    const timeout = setTimeout(resolvePromise, 3000);
    child.once("exit", () => {
      clearTimeout(timeout);
      resolvePromise();
    });
  });
}

async function rmRetry(path) {
  let lastError;
  for (let attempt = 0; attempt < 10; attempt++) {
    try {
      await rm(path, { recursive: true, force: true });
      return;
    } catch (error) {
      lastError = error;
      await sleep(250);
    }
  }
  throw lastError;
}
