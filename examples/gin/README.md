# Gin Integration Example

This example mounts FDDP under `/api/fddp/*` in a Gin app while keeping identity resolution backend-owned through `BearerTokenIdentityResolver`.

Run it:

```bash
cd examples/gin
go mod tidy
go run .
```

Read the contract:

```bash
curl http://localhost:8080/api/fddp/contract
```

Query page data:

```bash
curl -X POST http://localhost:8080/api/fddp/data/query \
  -H 'content-type: application/json' \
  -H 'authorization: Bearer demo-token' \
  -d '{"query":{"me":{"profile":["id","name","desc"]},"global":{"config":["appName"]}}}'
```

Execute a command:

```bash
curl -X POST http://localhost:8080/api/fddp/command/execute \
  -H 'content-type: application/json' \
  -H 'authorization: Bearer demo-token' \
  -d '{"command":"user.profile.update","idempotencyKey":"profile-update-1","input":{"displayName":"Tom"}}'
```

The demo token verifier accepts only `Bearer demo-token`. Replace it with your JWT/session verifier before using this pattern in a real service.
