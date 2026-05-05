import { fddp } from "./fddp-client";
import { createFddpApi } from "./fddp.generated";

const api = createFddpApi(fddp);

export async function updateProfile(displayName: string) {
  return api.command.user.profile.update(
    { displayName },
    {
      idempotencyKey: crypto.randomUUID()
    }
  );
}
