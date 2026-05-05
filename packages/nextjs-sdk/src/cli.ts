#!/usr/bin/env node
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, relative, resolve, sep } from "node:path";
import { diffFddpContracts, generateFddpTypes, validateFddpContractSchema, type FddpContractChange, type FddpContractDiffOptions, type FddpContractSchema } from "./codegen";

type FddpConfig = {
  contract?: string;
  output?: string;
  dataTypeName?: string;
  fieldsConstName?: string;
  resourcesConstName?: string;
  commandsConstName?: string;
};

type CliOptions = FddpConfig & {
  config?: string;
  from?: string;
  to?: string;
  allowRequiredInputAdd?: boolean;
  allowInputRequiredTighten?: boolean;
  allowIdempotencyTighten?: boolean;
  allowMaxPageSizeDecrease?: boolean;
  force?: boolean;
  help?: boolean;
  template?: "fullstack" | "go" | "next";
  name?: string;
  module?: string;
  packageName?: string;
};

const packageRoot = resolveCliPackageRoot();

const defaultConfig: Required<
  Pick<FddpConfig, "contract" | "output" | "dataTypeName" | "fieldsConstName" | "resourcesConstName" | "commandsConstName">
> = {
  contract: "http://localhost:8080/contract",
  output: "src/fddp.generated.ts",
  dataTypeName: "FddpGeneratedData",
  fieldsConstName: "fields",
  resourcesConstName: "resources",
  commandsConstName: "commands"
};

async function main(argv: string[]): Promise<void> {
  const command = argv[0]?.startsWith("-") || !argv[0] ? "codegen" : argv[0];
  const args = command === "codegen" ? argv.slice(command === argv[0] ? 1 : 0) : argv.slice(1);

  switch (command) {
    case "new":
      await runNew(parseArgs(args, { allowPositionals: true }));
      return;
    case "init":
      await runInit(parseArgs(args));
      return;
    case "codegen":
      await runCodegen(parseArgs(args));
      return;
    case "check":
      await runCheck(parseArgs(args));
      return;
    case "diff":
      await runDiff(parseArgs(args));
      return;
    case "help":
    case "--help":
    case "-h":
      printHelp();
      return;
    default:
      throw new Error(`Unknown command: ${command}`);
  }
}

async function runNew(options: CliOptions): Promise<void> {
  if (options.help) {
    printNewHelp();
    return;
  }

  const projectName = options.name ?? "fddp-app";
  const template = options.template ?? "fullstack";
  const targetDir = resolve(process.cwd(), projectName);
  const backendDir = join(targetDir, "backend");
  const frontendDir = join(targetDir, "frontend");
  const backendModule = options.module ?? `${sanitizePackageName(projectName)}/backend`;
  const frontendPackage = options.packageName ?? `${sanitizePackageName(projectName)}-frontend`;
  const frontendDependency = `file:${posixPath(relative(frontendDir, packageRoot))}`;
  const goReplacePath = posixPath(relative(backendDir, resolve(packageRoot, "..", "go-fddp")));

  const files = scaffoldFiles({
    projectName,
    template,
    backendModule,
    frontendPackage,
    frontendDependency,
    goReplacePath
  });

  for (const [relativePath, content] of files) {
    await writeText(join(targetDir, relativePath), content, options.force);
  }

  console.log(`Created ${targetDir}`);
  printNextSteps(projectName, template);
}

async function runInit(options: CliOptions): Promise<void> {
  if (options.help) {
    printInitHelp();
    return;
  }

  const configPath = resolve(process.cwd(), options.config ?? "fddp.config.json");
  const config = {
    contract: options.contract ?? defaultConfig.contract,
    output: options.output ?? defaultConfig.output,
    dataTypeName: options.dataTypeName ?? defaultConfig.dataTypeName,
    fieldsConstName: options.fieldsConstName ?? defaultConfig.fieldsConstName,
    resourcesConstName: options.resourcesConstName ?? defaultConfig.resourcesConstName,
    commandsConstName: options.commandsConstName ?? defaultConfig.commandsConstName
  };

  await writeJSON(configPath, config, options.force);
  console.log(`Created ${configPath}`);
}

async function runCodegen(options: CliOptions): Promise<void> {
  if (options.help) {
    printCodegenHelp();
    return;
  }

  const fileConfig = await readConfig(options.config);
  const merged = {
    ...defaultConfig,
    ...fileConfig,
    ...withoutUndefined({
      contract: options.contract,
      output: options.output,
      dataTypeName: options.dataTypeName,
      fieldsConstName: options.fieldsConstName,
      resourcesConstName: options.resourcesConstName,
      commandsConstName: options.commandsConstName
    })
  };

  const outputPath = resolve(process.cwd(), merged.output);
  const schema = JSON.parse(await readInput(merged.contract)) as FddpContractSchema;
  validateFddpContractSchema(schema);

  const source = generateFddpTypes(schema, {
    dataTypeName: merged.dataTypeName,
    fieldsConstName: merged.fieldsConstName,
    resourcesConstName: merged.resourcesConstName,
    commandsConstName: merged.commandsConstName
  });
  await mkdir(dirname(outputPath), { recursive: true });
  await writeFile(outputPath, source, "utf8");
  console.log(`Generated ${outputPath}`);
}

async function runCheck(options: CliOptions): Promise<void> {
  if (options.help) {
    printCheckHelp();
    return;
  }

  const fileConfig = await readConfig(options.config);
  const contract = options.contract ?? fileConfig.contract ?? defaultConfig.contract;
  const schema = JSON.parse(await readInput(contract)) as FddpContractSchema;
  validateFddpContractSchema(schema);
  console.log(`FDDP contract OK: ${schema.contractVersion ?? "unversioned"}`);
}

async function runDiff(options: CliOptions): Promise<void> {
  if (options.help) {
    printDiffHelp();
    return;
  }
  if (!options.from || !options.to) {
    throw new Error("Missing --from or --to for fddp diff.");
  }

  const previous = JSON.parse(await readInput(options.from)) as FddpContractSchema;
  const next = JSON.parse(await readInput(options.to)) as FddpContractSchema;
  const diff = diffFddpContracts(previous, next, diffOptions(options));

  printChanges("Breaking changes", diff.breaking);
  printChanges("Non-breaking changes", diff.nonBreaking);

  if (diff.breaking.length > 0) {
    process.exitCode = 1;
  }
}

function parseArgs(argv: string[], optionsConfig: { allowPositionals?: boolean } = {}): CliOptions {
  const options: CliOptions = {};
  const positionals: string[] = [];

  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === undefined) {
      continue;
    }
    switch (arg) {
      case "--config":
      case "-c":
        options.config = readOptionValue(argv, ++index, arg);
        break;
      case "--contract":
      case "--input":
      case "-i":
        options.contract = readOptionValue(argv, ++index, arg);
        break;
      case "--from":
        options.from = readOptionValue(argv, ++index, arg);
        break;
      case "--to":
        options.to = readOptionValue(argv, ++index, arg);
        break;
      case "--allow-required-input-add":
        options.allowRequiredInputAdd = true;
        break;
      case "--allow-input-required-tighten":
        options.allowInputRequiredTighten = true;
        break;
      case "--allow-idempotency-tighten":
        options.allowIdempotencyTighten = true;
        break;
      case "--allow-max-page-size-decrease":
        options.allowMaxPageSizeDecrease = true;
        break;
      case "--output":
      case "-o":
        options.output = readOptionValue(argv, ++index, arg);
        break;
      case "--data-type":
        options.dataTypeName = readOptionValue(argv, ++index, arg);
        break;
      case "--fields-const":
        options.fieldsConstName = readOptionValue(argv, ++index, arg);
        break;
      case "--resources-const":
        options.resourcesConstName = readOptionValue(argv, ++index, arg);
        break;
      case "--commands-const":
        options.commandsConstName = readOptionValue(argv, ++index, arg);
        break;
      case "--template": {
        const value = readOptionValue(argv, ++index, arg);
        if (value !== "fullstack" && value !== "go" && value !== "next") {
          throw new Error(`Invalid template: ${value}. Use fullstack, go, or next.`);
        }
        options.template = value;
        break;
      }
      case "--name":
        options.name = readOptionValue(argv, ++index, arg);
        break;
      case "--module":
        options.module = readOptionValue(argv, ++index, arg);
        break;
      case "--package-name":
        options.packageName = readOptionValue(argv, ++index, arg);
        break;
      case "--force":
        options.force = true;
        break;
      case "--help":
      case "-h":
        options.help = true;
        break;
      default:
        if (optionsConfig.allowPositionals && !arg.startsWith("-")) {
          positionals.push(arg);
          break;
        }
        throw new Error(`Unknown argument: ${arg}`);
    }
  }

  if (positionals.length > 1) {
    throw new Error(`Unexpected argument: ${positionals[1]}`);
  }
  if (positionals.length === 1 && !options.name) {
    options.name = positionals[0];
  }

  return options;
}

function diffOptions(options: CliOptions): FddpContractDiffOptions {
  return {
    allowRequiredCommandInputAdd: options.allowRequiredInputAdd,
    allowCommandInputRequiredTighten: options.allowInputRequiredTighten,
    allowIdempotencyTighten: options.allowIdempotencyTighten,
    allowResourceMaxPageSizeDecrease: options.allowMaxPageSizeDecrease
  };
}

function printChanges(title: string, changes: readonly FddpContractChange[]): void {
  console.log(`${title}: ${changes.length}`);
  for (const change of changes) {
    console.log(`  - ${change.path}: ${change.message}`);
  }
}

function readOptionValue(argv: string[], index: number, option: string): string {
  const value = argv[index];
  if (!value || value.startsWith("-")) {
    throw new Error(`Missing value for ${option}`);
  }
  return value;
}

async function readConfig(path?: string): Promise<FddpConfig> {
  const configPath = resolve(process.cwd(), path ?? "fddp.config.json");
  try {
    return JSON.parse(await readFile(configPath, "utf8")) as FddpConfig;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return {};
    }
    throw error;
  }
}

async function readInput(input: string): Promise<string> {
  if (/^https?:\/\//i.test(input)) {
    const response = await fetch(input);
    if (!response.ok) {
      throw new Error(`Failed to fetch FDDP contract: ${response.status} ${response.statusText}`);
    }
    return response.text();
  }

  return readFile(resolve(process.cwd(), input), "utf8");
}

async function writeJSON(path: string, value: unknown, force = false): Promise<void> {
  if (!force) {
    try {
      await readFile(path, "utf8");
      throw new Error(`Refusing to overwrite existing file: ${path}. Use --force to replace it.`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
        throw error;
      }
    }
  }

  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, `${JSON.stringify(value, null, 2)}\n`, "utf8");
}

async function writeText(path: string, value: string, force = false): Promise<void> {
  if (!force) {
    try {
      await readFile(path, "utf8");
      throw new Error(`Refusing to overwrite existing file: ${path}. Use --force to replace it.`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
        throw error;
      }
    }
  }

  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, value, "utf8");
}

function withoutUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>;
}

type ScaffoldContext = {
  projectName: string;
  template: "fullstack" | "go" | "next";
  backendModule: string;
  frontendPackage: string;
  frontendDependency: string;
  goReplacePath: string;
};

function scaffoldFiles(ctx: ScaffoldContext): Array<[string, string]> {
  const files: Array<[string, string]> = [["README.md", scaffoldReadme(ctx)]];
  if (ctx.template === "fullstack" || ctx.template === "go") {
    files.push(
      ["backend/go.mod", scaffoldGoMod(ctx.backendModule, ctx.goReplacePath)],
      ["backend/main.go", scaffoldGoMain()],
      ["backend/.gitignore", "fddp-demo\n*.exe\n"]
    );
  }
  if (ctx.template === "fullstack" || ctx.template === "next") {
    files.push(
      ["frontend/package.json", scaffoldFrontendPackage(ctx.frontendPackage, ctx.frontendDependency)],
      ["frontend/tsconfig.json", scaffoldTsconfig()],
      ["frontend/fddp.config.json", scaffoldFddpConfig()],
      ["frontend/src/fddp.generated.ts", scaffoldGeneratedTypes()],
      ["frontend/src/fddp-client.ts", scaffoldFddpClient()],
      ["frontend/src/dashboard-data.ts", scaffoldDashboardData()],
      ["frontend/src/update-profile.ts", scaffoldUpdateProfile()]
    );
  }
  return files;
}

function scaffoldReadme(ctx: ScaffoldContext): string {
  const sections = [
    `# ${ctx.projectName}`,
    "",
    "Small FDDP starter project.",
    ""
  ];
  if (ctx.template === "fullstack" || ctx.template === "go") {
    sections.push(
      "## Backend",
      "",
      "```bash",
      "cd backend",
      "go mod tidy",
      "go run .",
      "```",
      ""
    );
  }
  if (ctx.template === "fullstack" || ctx.template === "next") {
    sections.push(
      "## Frontend SDK Loop",
      "",
      "```bash",
      "cd frontend",
      "npm install",
      "npm run codegen",
      "npm run typecheck",
      "```",
      ""
    );
  }
  return sections.join("\n");
}

function scaffoldGoMod(moduleName: string, replacePath = "../../packages/go-fddp"): string {
  return `module ${moduleName}

go 1.23

require (
	github.com/Unicode01/FDDP/packages/go-fddp v0.0.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

replace github.com/Unicode01/FDDP/packages/go-fddp => ${replacePath}
`;
}

function scaffoldGoMain(): string {
  return `package main

import (
	"log"
	"net/http"
	"os"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"github.com/Unicode01/FDDP/packages/go-fddp/fddplite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Profile struct {
	ID          string \`gorm:"primaryKey"\`
	UserID      string \`gorm:"index"\`
	Name        string
	Description string
}

type User struct {
	ID   string \`gorm:"primaryKey"\`
	Name string
}

type Project struct {
	ID        string \`gorm:"primaryKey"\`
	Name      string
	OwnerID   string
	Owner     User \`gorm:"foreignKey:OwnerID"\`
	Status    string
	TenantID  string \`gorm:"index"\`
	UpdatedAt string \`gorm:"index"\`
}

type UpdateProfileInput struct {
	DisplayName string \`json:"displayName"\`
}

func main() {
	db := openDB()
	seedDB(db)

	app := fddplite.NewDevApp(db)

	must(fddplite.FieldGroup[Profile](app, "me.profile").
		Fields("ID", "Name", "Description").
		Self("UserID").
		Register())

	must(fddplite.Collection[Project](app, "project.list").
		Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
		Tenant("TenantID").
		DescCursor("UpdatedAt").
		Relation("owner", "Owner", "ID", "Name").
		Register())

	must(fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
		Self("UserID").
		Idempotent().
		Set("Name", "DisplayName").
		Invalidates("me.profile.*").
		Register())

	must(app.Engine().RegisterStaticField("global.config.appName", "FDDP Starter", fddp.FieldPermission("public")))

	port := env("PORT", "8080")
	log.Printf("FDDP backend listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, app.Handler()))
}

func openDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file:fddp_starter?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatal(err)
	}
	must(db.AutoMigrate(&Profile{}, &User{}, &Project{}))
	return db
}

func seedDB(db *gorm.DB) {
	must(db.Create(&Profile{ID: "profile_1", UserID: "user_123", Name: "Tom", Description: "Starter user"}).Error)
	must(db.Create(&User{ID: "user_123", Name: "Tom"}).Error)
	must(db.Create(&User{ID: "user_456", Name: "Ann"}).Error)

	projects := []Project{
		{ID: "project_1", Name: "Alpha", OwnerID: "user_123", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-03T00:00:00Z"},
		{ID: "project_2", Name: "Beta", OwnerID: "user_456", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-02T00:00:00Z"},
		{ID: "project_3", Name: "Other Tenant", OwnerID: "user_123", Status: "active", TenantID: "tenant_other", UpdatedAt: "2026-01-01T00:00:00Z"},
	}
	for _, project := range projects {
		must(db.Create(&project).Error)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func env(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
`;
}

function scaffoldFrontendPackage(packageName: string, dependency: string): string {
  return `${JSON.stringify(
    {
      name: packageName,
      private: true,
      type: "module",
      scripts: {
        codegen: "fddp codegen",
        typecheck: "tsc --noEmit"
      },
      dependencies: {
        "@fddp/next-sdk": dependency
      },
      devDependencies: {
        "@types/node": "^25.6.0",
        typescript: "^5.4.0"
      }
    },
    null,
    2
  )}\n`;
}

function scaffoldTsconfig(): string {
  return `${JSON.stringify(
    {
      compilerOptions: {
        target: "ES2022",
        module: "ESNext",
        moduleResolution: "Bundler",
        strict: true,
        skipLibCheck: true,
        jsx: "react-jsx"
      },
      include: ["src/**/*.ts", "src/**/*.tsx"]
    },
    null,
    2
  )}\n`;
}

function scaffoldFddpConfig(): string {
  return `${JSON.stringify(
    {
      contract: "http://localhost:8080/contract",
      output: "src/fddp.generated.ts",
      dataTypeName: "FddpGeneratedData",
      fieldsConstName: "fields",
      resourcesConstName: "resources",
      commandsConstName: "commands"
    },
    null,
    2
  )}\n`;
}

function scaffoldGeneratedTypes(): string {
  return generateFddpTypes({
    protocolVersion: "v9",
    contractVersion: "starter",
    fields: [
      { field: "global.config.appName", type: "string", permission: "public" },
      { field: "me.profile.description", type: "string", permission: "self" },
      { field: "me.profile.id", type: "string", permission: "self" },
      { field: "me.profile.name", type: "string", permission: "self" }
    ],
    resources: [
      {
        path: "project.list",
        types: ["collection"],
        maxPageSize: 50,
        permission: "tenant",
        fields: [
          { field: "id", type: "string", filterable: true, orderable: true },
          { field: "name", type: "string", filterable: true, orderable: true },
          { field: "ownerId", type: "string", filterable: true, orderable: true },
          { field: "status", type: "string", filterable: true, orderable: true },
          { field: "updatedAt", type: "string", filterable: true, orderable: true }
        ],
        relations: [
          {
            name: "owner",
            fields: [
              { field: "id", type: "string", filterable: true, orderable: true },
              { field: "name", type: "string", filterable: true, orderable: true }
            ]
          }
        ]
      }
    ],
    commands: [
      {
        name: "user.profile.update",
        permission: "self",
        idempotencyRequired: true,
        input: [{ field: "displayName", type: "string", required: false }]
      }
    ]
  });
}

function scaffoldFddpClient(): string {
  return `import { createFddpClient } from "@fddp/next-sdk";

export const fddp = createFddpClient({
  baseUrl: process.env.NEXT_PUBLIC_FDDP_URL ?? "http://localhost:8080",
  headers: {
    "X-DDP-Subject": "user_123",
    "X-DDP-Tenant": "tenant_abc"
  },
  trace: process.env.NODE_ENV === "development"
});
`;
}

function scaffoldDashboardData(): string {
  return `import { fddp } from "./fddp-client";
import { createFddpApi, fields } from "./fddp.generated";

const api = createFddpApi(fddp);

export async function loadDashboard() {
  return api.load({
    fields: [fields.me.profile.name, fields.global.config.appName],
    projectList: {
      first: 20,
      filter: { status: { eq: "active" } },
      orderBy: [{ field: "updatedAt", direction: "desc" }],
      fields: ["id", "name", "updatedAt"],
      expand: { owner: ["id", "name"] }
    }
  });
}
`;
}

function scaffoldUpdateProfile(): string {
  return `import { fddp } from "./fddp-client";
import { createFddpApi } from "./fddp.generated";

const api = createFddpApi(fddp);

export async function updateProfile(displayName: string) {
  return api.command.user.profile.update({ displayName }, {
    idempotencyKey: crypto.randomUUID()
  });
}
`;
}

function sanitizePackageName(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "") || "fddp-app";
}

function resolveCliPackageRoot(): string {
  const entry = process.argv[1];
  if (!entry) {
    return process.cwd();
  }
  const entryDir = dirname(resolve(entry));
  if (entryDir.endsWith(`${sep}src`) || entryDir.endsWith(`${sep}dist`)) {
    return resolve(entryDir, "..");
  }
  return entryDir;
}

function posixPath(value: string): string {
  return value.split(sep).join("/");
}

function printNextSteps(projectName: string, template: CliOptions["template"]): void {
  const lines = [`Next steps:`, `  cd ${projectName}`];
  if (template === "fullstack" || template === "go") {
    lines.push(`  cd backend`, `  go mod tidy`, `  go run .`);
  }
  if (template === "fullstack" || template === "next") {
    lines.push(`  cd frontend`, `  npm install`, `  npm run codegen`, `  npm run typecheck`);
  }
  console.log(lines.join("\n"));
}

function printHelp(): void {
  console.log(`Usage:
  fddp new [name] [options]
  fddp init [options]
  fddp codegen [options]
  fddp check [options]
  fddp diff --from old.json --to new.json
  fddp-codegen [options]

Commands:
  new       Create a small FDDP starter project
  init      Create fddp.config.json
  codegen   Generate TypeScript fields and response types
  check     Validate a contract JSON file or URL
  diff      Compare two contracts and fail on breaking changes
`);
}

function printNewHelp(): void {
  console.log(`Usage:
  fddp new [options]

Options:
      --name          Project directory name, default fddp-app
      --template      fullstack, go, or next. Default fullstack
      --module        Go module name for backend
      --package-name  Frontend package name
      --force         Overwrite existing files
  -h, --help          Show help
`);
}

function printInitHelp(): void {
  console.log(`Usage:
  fddp init [options]

Options:
  -c, --config        Config file path, default fddp.config.json
      --contract      FDDP contract JSON file or URL
  -o, --output        Generated TypeScript output file
      --data-type     Generated data type name
      --fields-const  Generated field constants name
      --resources-const Generated resource constants name
      --commands-const  Generated command constants name
      --force         Overwrite existing config
  -h, --help          Show help
`);
}

function printCodegenHelp(): void {
  console.log(`Usage:
  fddp codegen [options]

Options:
  -c, --config        Config file path, default fddp.config.json
  -i, --input         FDDP contract JSON file or URL, alias for --contract
      --contract      FDDP contract JSON file or URL
  -o, --output        Generated TypeScript output file
      --data-type     Generated data type name
      --fields-const  Generated field constants name
      --resources-const Generated resource constants name
      --commands-const  Generated command constants name
  -h, --help          Show help
`);
}

function printCheckHelp(): void {
  console.log(`Usage:
  fddp check [options]

Options:
  -c, --config        Config file path, default fddp.config.json
  -i, --input         FDDP contract JSON file or URL, alias for --contract
      --contract      FDDP contract JSON file or URL
  -h, --help          Show help
`);
}

function printDiffHelp(): void {
  console.log(`Usage:
  fddp diff --from old.json --to new.json

Options:
      --from          Previous FDDP contract JSON file or URL
      --to            Next FDDP contract JSON file or URL
      --allow-required-input-add       Treat newly added required command inputs as allowed
      --allow-input-required-tighten   Treat optional-to-required command inputs as allowed
      --allow-idempotency-tighten      Treat newly required idempotency keys as allowed
      --allow-max-page-size-decrease   Treat resource maxPageSize decreases as allowed
  -h, --help          Show help
`);
}

main(process.argv.slice(2)).catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
