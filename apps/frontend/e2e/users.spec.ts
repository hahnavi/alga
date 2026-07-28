import { expect, test, type Page } from "@playwright/test";
import { ADMIN_USER, dataEnvelope, json, makeUser, mockAuthenticated } from "./helpers";

const OPERATOR = makeUser();
const STRONG_PASSWORD = "Str0ng!Passw0rd";

async function mockUsersList(page: Page, users: unknown[] = [ADMIN_USER, OPERATOR]) {
  await page.route("**/api/v1/users**", (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill(dataEnvelope(users));
    }
    return route.fulfill(json({ status: "ok" }));
  });
}

test.describe("users: list", () => {
  test("renders users with role badges", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    await page.goto("/users");
    await expect(page.getByText("admin@alga.test")).toBeVisible();
    await expect(page.getByText("operator@alga.test")).toBeVisible();
    await expect(page.getByText("admin", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("operator", { exact: true }).first()).toBeVisible();
  });

  test("search filters the visible user list", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    await page.goto("/users");
    await expect(page.getByText("operator@alga.test")).toBeVisible();

    await page.getByRole("button", { name: "Search" }).click();
    await page.locator("[data-page-header-search]").fill("operator");
    await expect(page.getByText("operator@alga.test")).toBeVisible();
    await expect(page.getByText("admin@alga.test")).toBeHidden();
  });

  test("does not offer delete for the signed-in user", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    await page.goto("/users");
    await expect(page.getByRole("button", { name: "Delete" })).toHaveCount(1);
  });
});

test.describe("users: create", () => {
  test("create dialog POSTs the new user and reloads the list", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    let createBody: Record<string, unknown> | null = null;
    await page.route("**/api/v1/users", (route) => {
      if (route.request().method() === "POST") {
        createBody = JSON.parse(route.request().postData() ?? "{}");
        return route.fulfill(dataEnvelope(makeUser({ id: "user-new-1", email: "new@alga.test" })));
      }
      return route.fulfill(dataEnvelope([ADMIN_USER, OPERATOR]));
    });

    await page.goto("/users");
    await page.getByRole("button", { name: "Add user" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();

    await page.fill("#new-email", "new@alga.test");
    await page.fill("#new-full-name", "New User");
    await page.fill("#new-password", STRONG_PASSWORD);
    await page.selectOption("#new-role", "operator");
    await page.getByRole("button", { name: "Create" }).click();

    await expect.poll(() => createBody).not.toBeNull();
    expect(createBody).toMatchObject({
      email: "new@alga.test",
      full_name: "New User",
      role: "operator",
      password: STRONG_PASSWORD,
    });
  });

  test("weak password blocks creation without a POST", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    let postCalled = false;
    await page.route("**/api/v1/users", (route) => {
      if (route.request().method() === "POST") {
        postCalled = true;
        return route.fulfill(dataEnvelope(makeUser()));
      }
      return route.fulfill(dataEnvelope([ADMIN_USER, OPERATOR]));
    });

    await page.goto("/users");
    await page.getByRole("button", { name: "Add user" }).click();
    await page.fill("#new-email", "new@alga.test");
    await page.fill("#new-password", "weakpass");
    await page.getByRole("button", { name: "Create" }).click();

    await expect(page.locator("#new-password")).toHaveAttribute("aria-invalid", "true");
    expect(postCalled).toBe(false);
  });

  test("duplicate email surfaces the API error", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    await page.route("**/api/v1/users", (route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 409, ...json({ error: "User already exists" }) });
      }
      return route.fulfill(dataEnvelope([ADMIN_USER, OPERATOR]));
    });

    await page.goto("/users");
    await page.getByRole("button", { name: "Add user" }).click();
    await page.fill("#new-email", "operator@alga.test");
    await page.fill("#new-password", STRONG_PASSWORD);
    await page.getByRole("button", { name: "Create" }).click();

    await expect(page.getByText(/user already exists/i)).toBeVisible();
  });
});

test.describe("users: edit and delete", () => {
  test("editing a user sends PUT with the new role", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    let updateBody: Record<string, unknown> | null = null;
    await page.route("**/api/v1/users/user-op-1", (route) => {
      if (route.request().method() === "PUT") {
        updateBody = JSON.parse(route.request().postData() ?? "{}");
        return route.fulfill(json({ status: "ok" }));
      }
      return route.fulfill(dataEnvelope([ADMIN_USER, OPERATOR]));
    });

    await page.goto("/users");
    await page.getByRole("button", { name: "Edit" }).nth(1).click();
    await expect(page.getByRole("dialog")).toBeVisible();

    await page.selectOption("#edit-role", "admin");
    await page.getByRole("button", { name: "Save" }).click();

    await expect.poll(() => updateBody).not.toBeNull();
    expect(updateBody).toMatchObject({ role: "admin", email: "operator@alga.test" });
  });

  test("deleting a user requires confirmation and sends DELETE", async ({ page }) => {
    await mockAuthenticated(page);
    await mockUsersList(page);

    let deleteCalled = false;
    await page.route("**/api/v1/users/user-op-1", (route) => {
      if (route.request().method() === "DELETE") {
        deleteCalled = true;
        return route.fulfill(json({ status: "ok" }));
      }
      return route.fulfill(dataEnvelope([ADMIN_USER, OPERATOR]));
    });

    await page.goto("/users");
    await page.getByRole("button", { name: "Delete" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(/are you sure/i)).toBeVisible();
    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect.poll(() => deleteCalled).toBe(true);
  });
});
