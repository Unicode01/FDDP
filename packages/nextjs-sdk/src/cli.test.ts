import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const packageRoot = process.cwd();
const tsxCli = join(packageRoot, "node_modules/tsx/dist/cli.mjs");
const cliPath = join(packageRoot, "src/cli.ts");
const root = await mkdtemp(join(tmpdir(), "fddp-cli-"));

try {
  const init = run(["src/cli.ts", "init", "--contract", "contract.json", "--output", "generated/fddp.ts"], root);
  assertEqual(init.status, 0, init.stderr);

  const config = JSON.parse(await readFile(join(root, "fddp.config.json"), "utf8")) as {
    contract: string;
    output: string;
    resourcesConstName: string;
    commandsConstName: string;
  };
  assertEqual(config.contract, "contract.json");
  assertEqual(config.output, "generated/fddp.ts");
  assertEqual(config.resourcesConstName, "resources");
  assertEqual(config.commandsConstName, "commands");

  await writeContract(join(root, "contract.json"));
  const codegen = run(["src/cli.ts", "codegen"], root);
  assertEqual(codegen.status, 0, codegen.stderr);

  const generated = await readFile(join(root, "generated/fddp.ts"), "utf8");
  assertIncludes(generated, "contractVersion: contract_cli_test");
  assertIncludes(generated, '"name": "me.profile.name"');
  assertIncludes(generated, '"list": "project.list"');
  assertIncludes(generated, '"update": "user.profile.update"');

  const legacy = run([
    "src/cli.ts",
    "--input",
    "contract.json",
    "--output",
    "generated/legacy.ts"
  ], root);
  assertEqual(legacy.status, 0, legacy.stderr);
  assertIncludes(await readFile(join(root, "generated/legacy.ts"), "utf8"), "FddpGeneratedData");

  const check = run(["src/cli.ts", "check", "--contract", "contract.json"], root);
  assertEqual(check.status, 0, check.stderr);
  assertIncludes(check.stdout, "FDDP contract OK: contract_cli_test");

  await writeContract(join(root, "contract.next.json"), {
    contractVersion: "contract_cli_next",
    fields: [
      { field: "me.profile.name", type: "string" },
      { field: "me.profile.avatar", type: "string", nullable: true }
    ],
    resources: [
      {
        path: "project.list",
        types: ["collection"],
        fields: [{ field: "id", type: "string" }]
      }
    ],
    commands: [
      {
        name: "user.profile.update",
        idempotencyRequired: true,
        input: [{ field: "displayName", type: "string", required: true }]
      }
    ]
  });
  const diff = run(["src/cli.ts", "diff", "--from", "contract.json", "--to", "contract.next.json"], root);
  assertNotEqual(diff.status, 0, "expected fddp diff to fail on breaking changes");
  assertIncludes(diff.stdout, "Breaking changes:");
  assertIncludes(diff.stdout, "project.list.name: resource field removed");
  assertIncludes(diff.stdout, "user.profile.update.displayName: required command input added");

  const allowedDiff = run([
    "src/cli.ts",
    "diff",
    "--from",
    "contract.json",
    "--to",
    "contract.next.json",
    "--allow-required-input-add"
  ], root);
  assertNotEqual(allowedDiff.status, 0, "resource field removal should still fail allowed input diff");
  assertIncludes(allowedDiff.stdout, "allowed by diff policy");

  const scaffold = run(["src/cli.ts", "new", "starter", "--template", "fullstack"], root);
  assertEqual(scaffold.status, 0, scaffold.stderr);

  const backendMain = await readFile(join(root, "starter/backend/main.go"), "utf8");
  assertIncludes(backendMain, "fddplite.NewDevApp(db)");
  assertIncludes(backendMain, "fddplite.FieldGroup[Profile]");
  assertIncludes(backendMain, "fddplite.Collection[Project]");
  assertIncludes(backendMain, "fddplite.UpdateCommand[Profile, UpdateProfileInput]");

  const backendMod = await readFile(join(root, "starter/backend/go.mod"), "utf8");
  assertIncludes(backendMod, "gorm.io/driver/sqlite");
  assertIncludes(backendMod, "gorm.io/gorm");

  const generatedStarter = await readFile(join(root, "starter/frontend/src/fddp.generated.ts"), "utf8");
  assertIncludes(generatedStarter, '"description": "me.profile.description"');
  assertIncludes(generatedStarter, '"ownerId"');
  assertIncludes(generatedStarter, '"owner"');
  assertIncludes(generatedStarter, 'displayName?: string;');

  const frontendData = await readFile(join(root, "starter/frontend/src/dashboard-data.ts"), "utf8");
  assertIncludes(frontendData, "const api = createFddpApi(fddp)");
  assertIncludes(frontendData, "api.load({");
  assertIncludes(frontendData, "fields: [fields.me.profile.name");
  assertIncludes(frontendData, "filter: { status: { eq: \"active\" } }");
  assertIncludes(frontendData, "expand: { owner: [\"id\", \"name\"] }");

  const packageJson = JSON.parse(await readFile(join(root, "starter/frontend/package.json"), "utf8")) as {
    dependencies: Record<string, string>;
  };
  const dependency = packageJson.dependencies["@fddp/next-sdk"];
  if (!dependency) {
    throw new Error("Expected scaffold to include @fddp/next-sdk dependency");
  }
  assertIncludes(dependency, "packages/nextjs-sdk");

  const noOverwrite = run(["src/cli.ts", "new", "starter"], root);
  assertNotEqual(noOverwrite.status, 0, "expected fddp new to reject existing files");
} finally {
  await rm(root, { recursive: true, force: true });
}

function run(args: string[], cwd: string) {
  const [, ...cliArgs] = args;
  return spawnSync(process.execPath, [tsxCli, cliPath, ...cliArgs], {
    cwd,
    env: process.env,
    encoding: "utf8",
    shell: false
  });
}

async function writeContract(path: string, value?: unknown): Promise<void> {
  await writeFile(
    path,
    JSON.stringify(
      value ?? {
        contractVersion: "contract_cli_test",
        fields: [{ field: "me.profile.name", type: "string" }],
        resources: [
          {
            path: "project.list",
            types: ["collection"],
            fields: [
              { field: "id", type: "string" },
              { field: "name", type: "string" }
            ]
          }
        ],
        commands: [{ name: "user.profile.update", idempotencyRequired: true }]
      },
      null,
      2
    ),
    "utf8"
  );
}

function assertIncludes(value: string, expected: string): void {
  if (!value.includes(expected)) {
    throw new Error(`Expected value to include ${expected}`);
  }
}

function assertEqual<T>(actual: T, expected: T, message?: string): void {
  if (actual !== expected) {
    throw new Error(message || `Expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertNotEqual<T>(actual: T, expected: T, message?: string): void {
  if (actual === expected) {
    throw new Error(message || `Expected ${String(actual)} not to equal ${String(expected)}`);
  }
}
