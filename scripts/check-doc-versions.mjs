import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";

const root = process.cwd();
const rootPackage = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
const nextPackage = JSON.parse(
  readFileSync(join(root, "packages", "nextjs-sdk", "package.json"), "utf8")
);
const expectedReleaseTag = `v${rootPackage.version}`;

const ignoredDirs = new Set([
  ".git",
  ".github",
  "node_modules",
  "dist",
  "coverage"
]);

const allowedFiles = new Set([
  "CHANGELOG.md",
  "docs/install.md"
]);

const patterns = [
  {
    name: "hard-coded GitHub npm release install",
    regex: /github:Unicode01\/FDDP#v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?/g
  },
  {
    name: "hard-coded Go release install",
    regex: /go get github\.com\/Unicode01\/FDDP\/packages\/go-fddp@v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?/g
  },
  {
    name: "hard-coded Go submodule release tag",
    regex: /packages\/go-fddp\/v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?/g
  },
  {
    name: "hard-coded current release version",
    regex: /v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?/g
  }
];

const markdownFiles = [];

function walk(dir) {
  for (const entry of readdirSync(dir)) {
    const fullPath = join(dir, entry);
    const stat = statSync(fullPath);
    if (stat.isDirectory()) {
      if (!ignoredDirs.has(entry)) {
        walk(fullPath);
      }
      continue;
    }

    if (entry.endsWith(".md")) {
      markdownFiles.push(fullPath);
    }
  }
}

walk(root);

const violations = [];

if (nextPackage.version !== rootPackage.version) {
  violations.push({
    file: "packages/nextjs-sdk/package.json",
    line: 1,
    pattern: "package version mismatch",
    match: `${nextPackage.version} does not match root ${rootPackage.version}`
  });
}

const installDoc = readFileSync(join(root, "docs", "install.md"), "utf8");
if (!installDoc.includes(`Current release tag: \`${expectedReleaseTag}\``)) {
  violations.push({
    file: "docs/install.md",
    line: 1,
    pattern: "current release tag mismatch",
    match: `expected Current release tag: \`${expectedReleaseTag}\``
  });
}

for (const requiredText of [
  `github:Unicode01/FDDP#${expectedReleaseTag}`,
  `go get github.com/Unicode01/FDDP/packages/go-fddp@${expectedReleaseTag}`,
  `packages/go-fddp/${expectedReleaseTag}`
]) {
  if (!installDoc.includes(requiredText)) {
    violations.push({
      file: "docs/install.md",
      line: 1,
      pattern: "install command mismatch",
      match: `missing ${requiredText}`
    });
  }
}

for (const file of markdownFiles) {
  const normalized = relative(root, file).split(sep).join("/");
  if (allowedFiles.has(normalized)) {
    continue;
  }

  const text = readFileSync(file, "utf8");
  const lines = text.split(/\r?\n/);

  for (const pattern of patterns) {
    for (const line of lines) {
      pattern.regex.lastIndex = 0;
    }

    lines.forEach((line, index) => {
      pattern.regex.lastIndex = 0;
      const matches = line.match(pattern.regex);
      if (!matches) {
        return;
      }
      for (const match of matches) {
        violations.push({
          file: normalized,
          line: index + 1,
          pattern: pattern.name,
          match
        });
      }
    });
  }
}

if (violations.length > 0) {
  console.error("Hard-coded release versions found outside allowed release files.");
  console.error("Move install commands and release tags to docs/install.md, or add historical notes to CHANGELOG.md.");
  for (const violation of violations) {
    console.error(
      `- ${violation.file}:${violation.line} ${violation.pattern}: ${violation.match}`
    );
  }
  process.exit(1);
}

console.log("Documentation release-version check passed.");
