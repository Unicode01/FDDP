import { diffFddpContracts, generateFddpTypes } from "./codegen";
import { flattenFddpQuery, queryFromFields } from "./utils";

const source = generateFddpTypes({
  contractVersion: "contract_test",
  fields: [
    { field: "me.profile.name", type: "string" },
    { field: "me.profile.avatar", type: "string", nullable: true },
    { field: "tenant.current.name", type: "string" }
  ],
  resources: [
    {
      path: "project.list",
      types: ["collection"],
      maxPageSize: 50,
      fields: [
        { field: "id", type: "string", filterable: true, orderable: true },
        { field: "name", type: "string", filterable: true, orderable: true },
        { field: "updatedAt", type: "string", filterable: true, orderable: true }
      ],
      relations: [
        {
          name: "owner",
          fields: [
            { field: "id", type: "string" },
            { field: "name", type: "string" }
          ]
        }
      ]
    },
    { path: "report.summary", types: ["aggregate"] }
  ],
  commands: [
    { name: "user.profile.update", idempotencyRequired: true, input: [{ field: "displayName", type: "string", required: true }] }
  ]
});

assertIncludes(source, "contractVersion: contract_test");
assertIncludes(source, '"avatar": "me.profile.avatar"');
assertIncludes(source, "avatar: string | null;");
assertIncludes(source, 'export const resources = { "project": { "list": "project.list" }, "report": { "summary": "report.summary" } } as const;');
assertIncludes(source, "export const resourceCallers =");
assertIncludes(source, "export const queryCallers =");
assertIncludes(source, "load: (client:");
assertIncludes(source, "projectList?: FddpGeneratedCollectionQuery<\"project.list\">;");
assertIncludes(source, "export function createFddpApi(client:");
assertIncludes(source, "load: (input: FddpGeneratedLoadQuery");
assertIncludes(source, "commandCallers[\"user\"][\"profile\"][\"update\"](client, input, options)");
assertIncludes(source, 'export const commands = { "user": { "profile": { "update": "user.profile.update" } } } as const;');
assertIncludes(source, "export const commandCallers =");
assertIncludes(source, 'export type FddpGeneratedResource = "project.list" | "report.summary";');
assertIncludes(source, 'export type FddpGeneratedResourceItem<TResource extends FddpGeneratedResource>');
assertIncludes(source, 'export type FddpGeneratedResourceResult<TResource extends FddpGeneratedResource>');
assertIncludes(source, 'export type FddpGeneratedQueryData = FddpGeneratedData & FddpGeneratedAllResourceData;');
assertIncludes(source, 'export type FddpGeneratedResourceField<TResource extends FddpGeneratedResource>');
assertIncludes(source, '"project.list": "id" | "name" | "updatedAt";');
assertIncludes(source, '"owner": "id" | "name";');
assertIncludes(source, '"project.list": {');
assertIncludes(source, "updatedAt?: string;");
assertIncludes(source, 'FddpGeneratedCollectionQuery<TResource extends FddpGeneratedResource>');
assertIncludes(source, 'export type FddpGeneratedCommand = "user.profile.update";');
assertIncludes(source, 'export type FddpGeneratedCommandInput<TCommand extends FddpGeneratedCommand>');
assertIncludes(source, '"user.profile.update": {');
assertIncludes(source, "displayName: string;");

const query = queryFromFields(["tenant.current.name", "me.profile.avatar", "me.profile.name"]);
const flattened = flattenFddpQuery(query);
assertEqual(flattened.join(","), "me.profile.avatar,me.profile.name,tenant.current.name");

const diff = diffFddpContracts(
  {
    fields: [{ field: "me.profile.name", type: "string" }],
    resources: [
      {
        path: "project.list",
        fields: [
          { field: "id", type: "string", filterable: true },
          { field: "name", type: "string", orderable: true }
        ],
        relations: [{ name: "owner", fields: [{ field: "name", type: "string" }] }]
      }
    ],
    commands: [{ name: "user.profile.update", input: [{ field: "displayName", type: "string" }] }]
  },
  {
    fields: [{ field: "me.profile.name", type: "number" }],
    resources: [
      {
        path: "project.list",
        fields: [{ field: "id", type: "string", filterable: false }],
        relations: [{ name: "owner", fields: [{ field: "name", type: "string", nullable: true }] }]
      }
    ],
    commands: [{ name: "user.profile.update", input: [{ field: "displayName", type: "string", required: true }] }]
  }
);
assertIncludes(diff.breaking.map((change) => `${change.path}:${change.message}`).join("\n"), "me.profile.name:type changed from string to number");
assertIncludes(diff.breaking.map((change) => `${change.path}:${change.message}`).join("\n"), "project.list.id:resource field is no longer filterable");
assertIncludes(diff.breaking.map((change) => `${change.path}:${change.message}`).join("\n"), "project.list.name:resource field removed");
assertIncludes(diff.breaking.map((change) => `${change.path}:${change.message}`).join("\n"), "user.profile.update.displayName:command input became required");
assertIncludes(diff.nonBreaking.map((change) => `${change.path}:${change.message}`).join("\n"), "project.list.owner.name:field became nullable");

const policyDiff = diffFddpContracts(
  {
    fields: [{ field: "me.profile.name", type: "string" }],
    commands: [{ name: "user.profile.update", input: [{ field: "displayName", type: "string" }] }]
  },
  {
    fields: [{ field: "me.profile.name", type: "string" }],
    commands: [
      {
        name: "user.profile.update",
        idempotencyRequired: true,
        input: [
          { field: "displayName", type: "string", required: true },
          { field: "reason", type: "string", required: true }
        ]
      }
    ]
  },
  {
    allowCommandInputRequiredTighten: true,
    allowRequiredCommandInputAdd: true,
    allowIdempotencyTighten: true
  }
);
assertEqual(String(policyDiff.breaking.length), "0");
assertIncludes(policyDiff.nonBreaking.map((change) => change.message).join("\n"), "allowed by diff policy");

function assertIncludes(value: string, expected: string): void {
  if (!value.includes(expected)) {
    throw new Error(`Expected generated source to include ${expected}`);
  }
}

function assertEqual(actual: string, expected: string): void {
  if (actual !== expected) {
    throw new Error(`Expected ${expected}, got ${actual}`);
  }
}
