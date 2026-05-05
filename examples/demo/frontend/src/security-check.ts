export async function loadUnsafeProjectQueryExample() {
  const response = await fetch("http://localhost:8080/data/query", {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-DDP-Subject": "user_123",
      "X-DDP-Tenant": "tenant_abc"
    },
    body: JSON.stringify({
      query: {
        project: {
          list: {
            $type: "collection",
            args: {
              first: 5,
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
    })
  });

  return response.json();
}
