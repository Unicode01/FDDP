import { fddp } from "./fddp-client";
import { createFddpApi, fields } from "./fddp.generated";

const api = createFddpApi(fddp);

export async function loadDashboard() {
  return api.load({
    fields: [
      fields.me.profile.name,
      fields.global.config.appName
    ],
    projectList: {
      first: 20,
      filter: {
        status: { eq: "active" }
      },
      orderBy: [{ field: "updatedAt", direction: "desc" }],
      fields: ["id", "name", "updatedAt"],
      expand: {
        owner: ["id", "name"]
      }
    }
  });
}
