# Install FDDP

This page is the single place for install commands and release tag rules.
Other documentation should link here instead of hard-coding release versions.

## TypeScript SDK

The TypeScript SDK is not published to npm yet. Until it is published, install it from a GitHub release tag:

Current release tag: `v0.1.2-alpha`

```bash
npm install github:Unicode01/FDDP#v0.1.2-alpha
```

Release history is available at:

```text
https://github.com/Unicode01/FDDP/releases
```

Then create a starter project:

```bash
npx fddp new my-fddp-app
```

After npm publishing is available, this will become the normal install path:

```bash
npm install @fddp/next-sdk
```

## Go Runtime

Use the latest tagged Go module release:

```bash
go get github.com/Unicode01/FDDP/packages/go-fddp@v0.1.2-alpha
```

For a pinned version, use the same release tag shape as the TypeScript SDK:

```bash
go get github.com/Unicode01/FDDP/packages/go-fddp@<release-tag>
```

The Go module lives under `packages/go-fddp`, so repository tags for the Go module use this shape:

```text
packages/go-fddp/v0.1.2-alpha
```

If a regional Go proxy returns a timeout for a new release, retry later or use `GOPROXY=direct` for the install check.

## Source Development

For local source development, build the TypeScript SDK first, then run the local CLI:

```bash
git clone https://github.com/Unicode01/FDDP.git
cd FDDP/packages/nextjs-sdk
npm install
npm run build
node dist/cli.js new ../../my-fddp-app
```

## Release Maintainer Notes

When cutting a release, update package metadata and the changelog, then create both tags:

- Root release tag: `<release-tag>`
- Go submodule tag: `packages/go-fddp/<release-tag>`

Do not duplicate concrete release versions across README files. CI runs the documentation version check and fails if hard-coded release tags appear outside the allowed release files.
