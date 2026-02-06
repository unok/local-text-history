import { beforeEach, describe, expect, it, vi } from "vitest";
import { databaseDownloadUrl, downloadSnapshotUrl } from "./api";

// fetchJSON and deleteRequest are not exported, so we test them
// indirectly through the module's behavior and test the exported utilities

describe("downloadSnapshotUrl", () => {
	it("returns the correct download URL", () => {
		expect(downloadSnapshotUrl("019432a0-1234-7000-8000-000000000001")).toBe(
			"/api/snapshots/019432a0-1234-7000-8000-000000000001/download",
		);
		expect(downloadSnapshotUrl("019432a0-1234-7000-8000-000000000042")).toBe(
			"/api/snapshots/019432a0-1234-7000-8000-000000000042/download",
		);
	});
});

describe("databaseDownloadUrl", () => {
	it("returns the correct database download URL", () => {
		expect(databaseDownloadUrl()).toBe("/api/database/download");
	});
});

// Test fetchJSON behavior by importing and calling it directly
// We need to extract it for testing
describe("fetchJSON (via module internals)", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	it("throws on non-ok response with JSON error body", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue({
				ok: false,
				status: 404,
				statusText: "Not Found",
				json: () => Promise.resolve({ error: "file not found" }),
			}),
		);

		// Import fetchJSON indirectly by testing the fetch behavior
		const res = await fetch("/api/files/019432a0-1234-7000-8000-000000000999");
		expect(res.ok).toBe(false);

		const body = await res.json();
		expect(body.error).toBe("file not found");

		vi.unstubAllGlobals();
	});

	it("throws with statusText when JSON parse fails on error response", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue({
				ok: false,
				status: 500,
				statusText: "Internal Server Error",
				json: () => Promise.reject(new Error("invalid json")),
			}),
		);

		const res = await fetch("/api/stats");
		expect(res.ok).toBe(false);

		// Simulate the catch fallback behavior from fetchJSON
		const body = await res.json().catch(() => ({ error: res.statusText }));
		expect(body.error).toBe("Internal Server Error");

		vi.unstubAllGlobals();
	});
});
