import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { coverUrl, coverKey } from "./cover.ts";
import { errorMessage, formatPlaytime, formatDate } from "./format.ts";
import { platformMeta, gamePlatforms, isUnmatched, primaryGameUrl } from "./platform.ts";
import {
  deriveCounts,
  filterGames,
  navKeyId,
  navTitle,
  parseNavKey,
  needsScrape,
  type NavKey,
} from "./filters.ts";
import type { game } from "../../wailsjs/go/models";

/** Builds a game record with the fields the filters read. */
function makeGame(overrides: Partial<game.GameInfo> = {}): game.GameInfo {
  return {
    schemaVersion: 2,
    id: "g1",
    title: "Game",
    type: "game",
    executables: [],
    scannedAt: "2026-01-01T00:00:00Z",
    totalPlaytime: 0,
    ...overrides,
  } as game.GameInfo;
}

describe("coverUrl", () => {
  it("builds a portrait URL by default", () => {
    assert.equal(coverUrl("steam_570"), "/covers/steam_570/cover");
  });

  it("builds a landscape URL", () => {
    assert.equal(coverUrl("steam_570", "landscape"), "/covers/steam_570/landscape");
  });

  it("appends the version so a re-scraped cover is not served from cache", () => {
    assert.equal(coverUrl("g1", "cover", 1699999999), "/covers/g1/cover?v=1699999999");
  });

  it("returns an empty string without an id", () => {
    assert.equal(coverUrl(""), "");
  });

  it("escapes ids that contain URL-significant characters", () => {
    assert.equal(coverUrl("a b/c"), "/covers/a%20b%2Fc/cover");
  });

  it("changes the cache key when the version changes", () => {
    assert.notEqual(coverKey("g1", 1), coverKey("g1", 2));
    assert.equal(coverKey("g1"), coverKey("g1", 0));
  });
});

describe("formatPlaytime", () => {
  it("returns nothing for zero or missing playtime", () => {
    assert.equal(formatPlaytime(0), "");
    assert.equal(formatPlaytime(undefined), "");
    assert.equal(formatPlaytime(-5), "");
  });

  it("formats minutes under an hour", () => {
    assert.equal(formatPlaytime(45), "45m");
  });

  it("formats whole hours", () => {
    assert.equal(formatPlaytime(120), "2h");
  });

  it("formats hours and minutes", () => {
    assert.equal(formatPlaytime(125), "2h 5m");
  });
});

describe("formatDate", () => {
  it("returns nothing for empty or invalid values", () => {
    assert.equal(formatDate(undefined), "");
    assert.equal(formatDate("not a date"), "");
  });

  it("formats a valid timestamp", () => {
    assert.notEqual(formatDate("2026-01-02T03:04:05Z"), "");
  });
});

describe("errorMessage", () => {
  it("unwraps an Error", () => {
    assert.equal(errorMessage(new Error("boom")), "boom");
  });

  it("strips a redundant Error: prefix", () => {
    assert.equal(errorMessage("Error: steam needs an API key"), "steam needs an API key");
  });

  it("handles null and undefined without producing 'undefined'", () => {
    assert.equal(errorMessage(null), "Unknown error");
    assert.equal(errorMessage(undefined), "Unknown error");
  });

  it("handles an empty message", () => {
    assert.equal(errorMessage("   "), "Unknown error");
  });
});

describe("platformMeta", () => {
  it("knows the built-in platforms", () => {
    assert.equal(platformMeta("steam").label, "Steam");
    assert.equal(platformMeta("STEAM").label, "Steam");
    assert.equal(platformMeta("vndb").label, "VNDB");
  });

  it("builds a sensible URL per platform", () => {
    assert.equal(platformMeta("steam").url("570"), "https://store.steampowered.com/app/570/");
    assert.equal(platformMeta("vndb").url("v17"), "https://vndb.org/v17");
    assert.equal(platformMeta("bangumi").url("123"), "https://bgm.tv/subject/123");
    assert.match(platformMeta("dlsite").url("RJ123456"), /RJ123456\.html$/);
  });

  it("every platform with a URL builder handles VNDB (it used to be missing)", () => {
    for (const key of ["steam", "vndb", "bangumi", "dlsite", "rawg"]) {
      assert.notEqual(platformMeta(key).url("x"), "", `${key} should have a URL builder`);
    }
  });

  it("falls back gracefully for an unknown platform", () => {
    assert.equal(platformMeta("custom").label, "Custom");
    assert.equal(platformMeta("custom").url("x"), "");
    assert.equal(platformMeta("").label, "");
  });
});

describe("gamePlatforms and isUnmatched", () => {
  it("tolerates a missing platforms list", () => {
    assert.deepEqual(gamePlatforms(makeGame()), []);
    assert.equal(isUnmatched(makeGame()), true);
  });

  it("detects a linked platform", () => {
    const linked = makeGame({ platforms: [{ platform: "steam", id: "570" }] });
    assert.equal(gamePlatforms(linked).length, 1);
    assert.equal(isUnmatched(linked), false);
  });
});

describe("primaryGameUrl", () => {
  it("prefers a platform link", () => {
    const info = makeGame({ platforms: [{ platform: "steam", id: "570" }] });
    assert.equal(primaryGameUrl(info), "https://store.steampowered.com/app/570/");
  });

  it("falls back to metadata links", () => {
    const info = makeGame({ metadata: { links: { website: "https://example.test" } } });
    assert.equal(primaryGameUrl(info), "https://example.test");
  });

  it("returns an empty string when there is nothing to link to", () => {
    assert.equal(primaryGameUrl(makeGame()), "");
  });
});

describe("NavKey encoding", () => {
  const cases: NavKey[] = [
    { kind: "all" },
    { kind: "starred" },
    { kind: "unmatched" },
    { kind: "settings" },
    { kind: "platform", id: "steam" },
    { kind: "genre", id: "RPG" },
    { kind: "usertag", id: "backlog" },
    { kind: "folder", id: "Steam Library" },
  ];

  it("round-trips every variant, including ids containing a colon", () => {
    for (const nav of cases) {
      assert.deepEqual(parseNavKey(navKeyId(nav)), nav);
    }
  });

  it("round-trips an id that contains a colon", () => {
    const nav: NavKey = { kind: "genre", id: "Action: RPG" };
    assert.deepEqual(parseNavKey(navKeyId(nav)), nav);
  });

  it("treats an unknown string as the default view", () => {
    assert.deepEqual(parseNavKey("nonsense"), { kind: "all" });
    assert.deepEqual(parseNavKey("whatever:x"), { kind: "all" });
  });
});

describe("navTitle", () => {
  it("produces a readable title for every variant", () => {
    assert.equal(navTitle({ kind: "all" }), "All Games");
    assert.equal(navTitle({ kind: "starred" }), "Starred");
    assert.equal(navTitle({ kind: "unmatched" }), "Unmatched");
    assert.equal(navTitle({ kind: "settings" }), "Settings");
    assert.equal(navTitle({ kind: "platform", id: "steam" }), "Steam");
    assert.equal(navTitle({ kind: "genre", id: "RPG" }), "RPG");
    assert.equal(navTitle({ kind: "usertag", id: "backlog" }), "#backlog");
  });
});

describe("filterGames", () => {
  const games = [
    makeGame({ id: "a", title: "A", platforms: [{ platform: "steam", id: "1" }] }),
    makeGame({
      id: "b",
      title: "B",
      platforms: [{ platform: "vndb", id: "v2" }],
      metadata: { tags: ["RPG"] },
      tags: ["backlog"],
      starred: true,
    }),
    makeGame({ id: "c", title: "C" }),
  ];
  const context = { pathLabels: { a: ["Steam"], b: ["Visual Novels"] } };

  const ids = (nav: NavKey) =>
    filterGames(games, nav, context)
      .map((g) => g.id)
      .sort();

  it("excludes unmatched games from the default view", () => {
    assert.deepEqual(ids({ kind: "all" }), ["a", "b"]);
  });

  it("shows only unmatched games in the unmatched view", () => {
    assert.deepEqual(ids({ kind: "unmatched" }), ["c"]);
  });

  it("filters by star", () => {
    assert.deepEqual(ids({ kind: "starred" }), ["b"]);
  });

  it("filters by platform", () => {
    assert.deepEqual(ids({ kind: "platform", id: "steam" }), ["a"]);
    assert.deepEqual(ids({ kind: "platform", id: "vndb" }), ["b"]);
  });

  it("filters by genre tag", () => {
    assert.deepEqual(ids({ kind: "genre", id: "RPG" }), ["b"]);
  });

  it("filters by user tag", () => {
    assert.deepEqual(ids({ kind: "usertag", id: "backlog" }), ["b"]);
  });

  it("filters by directory label using the backend mapping", () => {
    assert.deepEqual(ids({ kind: "folder", id: "Steam" }), ["a"]);
    assert.deepEqual(ids({ kind: "folder", id: "Visual Novels" }), ["b"]);
    assert.deepEqual(ids({ kind: "folder", id: "Nonexistent" }), []);
  });

  it("returns nothing for the settings view", () => {
    assert.deepEqual(filterGames(games, { kind: "settings" }, context), []);
  });
});

describe("deriveCounts", () => {
  const games = [
    makeGame({ id: "a", platforms: [{ platform: "steam", id: "1" }], metadata: { tags: ["RPG"] } }),
    makeGame({
      id: "b",
      platforms: [{ platform: "steam", id: "2" }],
      metadata: { tags: ["RPG", "Action"] },
      tags: ["backlog"],
      starred: true,
    }),
    makeGame({ id: "c" }),
    makeGame({ id: "d", platforms: [{ platform: "vndb", id: "v1" }] }),
  ];
  const counts = deriveCounts(games, { pathLabels: { a: ["Steam"], b: ["Steam"] } });

  it("counts matched and unmatched games", () => {
    assert.equal(counts.all, 3);
    assert.equal(counts.unmatched, 1);
    assert.equal(counts.starred, 1);
  });

  it("counts platforms, most common first", () => {
    assert.deepEqual(counts.platforms[0], { id: "steam", count: 2 });
    assert.deepEqual(counts.platforms[1], { id: "vndb", count: 1 });
  });

  it("sorts ties alphabetically so the order is stable", () => {
    assert.deepEqual(counts.genres, [
      { id: "RPG", count: 2 },
      { id: "Action", count: 1 },
    ]);
  });

  it("counts user tags and directory labels", () => {
    assert.deepEqual(counts.userTags, [{ id: "backlog", count: 1 }]);
    assert.deepEqual(counts.folders, [{ id: "Steam", count: 2 }]);
  });

  it("handles an empty library", () => {
    const empty = deriveCounts([], { pathLabels: {} });
    assert.equal(empty.all, 0);
    assert.deepEqual(empty.platforms, []);
    assert.deepEqual(empty.folders, []);
  });
});

describe("needsScrape", () => {
  it("is true without metadata", () => {
    assert.equal(needsScrape(makeGame()), true);
  });

  it("is true when key fields are missing", () => {
    assert.equal(needsScrape(makeGame({ metadata: { description: "d" } })), true);
  });

  it("is false when the displayed fields are present", () => {
    const complete = makeGame({
      metadata: { description: "d", releaseDate: "2020", developer: "dev" },
    });
    assert.equal(needsScrape(complete), false);
  });
});
