import { describe, expect, test } from "@rstest/core";
import type { ProviderEpisodeInfo } from "./scrapers/base";
import {
  isVarietyEpisodeList,
  normalizeAirDate,
  parseVarietyEpisodeIdentity,
  selectEpisodeCandidates,
  withClientEpisodeNumber,
} from "./scrapers/episode-identity";

const candidate = (
  episodeId: string,
  episodeNumber: number,
  episodeTitle: string,
  overrides: Partial<ProviderEpisodeInfo> = {},
): ProviderEpisodeInfo => ({
  provider: "fixture",
  episodeId,
  episodeNumber,
  episodeTitle,
  ...overrides,
});

describe("variety episode identity", () => {
  test("does not interpret an ISO air date as an episode number", () => {
    expect(parseVarietyEpisodeIdentity("2022-11-24").episodeNumber).toBeNull();
    expect(parseVarietyEpisodeIdentity("2023-02-23 妻子的浪漫旅行").episodeNumber).toBeNull();
    expect(normalizeAirDate("2023/2/3 12:00")).toBe("2023-02-03");
  });

  test("lets an explicit issue after an ISO air date win over the date", () => {
    expect(parseVarietyEpisodeIdentity("2026-07-18 第4期中：跨时空情书")).toEqual({
      episodeNumber: 4,
      part: "middle",
      edition: "main",
    });
  });

  test("parses explicit issues and upper/middle/lower parts", () => {
    expect(parseVarietyEpisodeIdentity("第1期上：血染糖果厂")).toEqual({
      episodeNumber: 1,
      part: "upper",
      edition: "main",
    });
    expect(parseVarietyEpisodeIdentity("第12期（中）：未完待续")).toEqual({
      episodeNumber: 12,
      part: "middle",
      edition: "main",
    });
    expect(parseVarietyEpisodeIdentity("第十二期下")).toEqual({
      episodeNumber: 12,
      part: "lower",
      edition: "main",
    });
  });

  test("keeps descriptive words ending in 版 as main content", () => {
    expect(parseVarietyEpisodeIdentity("第11期 冯建宇演绎杂技版总裁")).toEqual({
      episodeNumber: 11,
      part: "whole",
      edition: "main",
    });
  });

  test("does not classify main-episode synopsis keywords as supplements", () => {
    expect(parseVarietyEpisodeIdentity("第4期：嘉宾合唱主题曲并召开新品发布会")).toEqual({
      episodeNumber: 4,
      part: "whole",
      edition: "main",
    });
  });

  test("classifies explicit bonus issues and Youku-style leading parts", () => {
    expect(parseVarietyEpisodeIdentity("会员加更版第8期：妻子团继续旅行")).toEqual({
      episodeNumber: 8,
      part: "whole",
      edition: "bonus",
    });
    expect(parseVarietyEpisodeIdentity("上：运动会少年争冠")).toEqual({
      episodeNumber: null,
      part: "upper",
      edition: "main",
    });
    expect(parseVarietyEpisodeIdentity("精编速看版：本期高光")).toEqual({
      episodeNumber: null,
      part: "whole",
      edition: "bonus",
    });
    expect(parseVarietyEpisodeIdentity("第4期巅峰饭局")).toEqual({
      episodeNumber: 4,
      part: "whole",
      edition: "bonus",
    });
  });

  test("does not let period-numbered companion clips override an ordinary requested episode", () => {
    expect(isVarietyEpisodeList(["第1集", "第2集", "主创陪你看第1期"])).toBe(false);
    expect(isVarietyEpisodeList(["第1集", "第2集", "主创陪你看第1期", "主创陪你看第2期"])).toBe(false);
    expect(
      isVarietyEpisodeList(["第1集", "第2集", "主创陪你看第1期", "主创陪你看第2期"], {
        episodeName: "第二集",
      }),
    ).toBe(false);
    expect(isVarietyEpisodeList(["第1期上", "第2期下"])).toBe(true);
  });
});

describe("variety candidate selection", () => {
  const episodes = [
    candidate("main-upper", 8, "第8期上", {
      episodePart: "upper",
      episodeEdition: "main",
      airDate: "2023-01-19",
    }),
    candidate("main-lower", 8, "第8期下", {
      episodePart: "lower",
      episodeEdition: "main",
      airDate: "2023-01-19",
    }),
    candidate("bonus", 8, "加更第8期", {
      episodeEdition: "bonus",
      airDate: "2023-01-20",
    }),
    candidate("preview", 8, "第8期预告", {
      episodeEdition: "preview",
      airDate: "2023-01-18",
    }),
  ];

  test("defaults to main content while preserving all main parts", () => {
    expect(selectEpisodeCandidates(episodes, 8).map((episode) => episode.episodeId)).toEqual([
      "main-upper",
      "main-lower",
    ]);
  });

  test("uses requested part or edition when that identity is explicit", () => {
    expect(
      selectEpisodeCandidates(episodes, 8, { episodeName: "第8期下" }).map((episode) => episode.episodeId),
    ).toEqual(["main-lower"]);
    expect(
      selectEpisodeCandidates(episodes, 8, { episodeName: "第8期加更" }).map((episode) => episode.episodeId),
    ).toEqual(["bonus"]);
  });

  test("does not fall back to a different explicit part or edition", () => {
    expect(
      selectEpisodeCandidates(
        [
          candidate("upper-only", 8, "第8期上", {
            episodePart: "upper",
            episodeEdition: "main",
          }),
        ],
        8,
        { episodeName: "第8期下" },
      ),
    ).toEqual([]);
    expect(
      selectEpisodeCandidates(
        [
          candidate("main-only", 8, "第8期", {
            episodePart: "whole",
            episodeEdition: "main",
          }),
        ],
        8,
        { episodeName: "第8期加更" },
      ),
    ).toEqual([]);
  });

  test("lets an explicit variety title map a sequential client episode to its source issue and part", () => {
    const splitIssue = [
      candidate("issue-4-upper", 4, "第4期上", {
        episodePart: "upper",
        episodeEdition: "main",
      }),
      candidate("issue-4-lower", 4, "第4期下", {
        episodePart: "lower",
        episodeEdition: "main",
      }),
    ];

    expect(selectEpisodeCandidates(splitIssue, 8, { episodeName: "第四期下" })).toEqual([
      expect.objectContaining({
        episodeId: "issue-4-lower",
        episodeNumber: 8,
      }),
    ]);
  });

  test("prefers a complete main episode over duplicate split parts", () => {
    const withWhole = [
      ...episodes,
      candidate("main-whole", 8, "第8期完整版", {
        episodePart: "whole",
        episodeEdition: "main",
      }),
    ];

    expect(selectEpisodeCandidates(withWhole, 8).map((episode) => episode.episodeId)).toEqual(["main-whole"]);
  });

  test("does not let air date split an explicitly matched issue into parts", () => {
    expect(selectEpisodeCandidates(episodes, 8, { airDate: "2023-01-20" }).map((episode) => episode.episodeId)).toEqual(
      ["main-upper", "main-lower"],
    );
    expect(selectEpisodeCandidates(episodes, 8, { airDate: "2024-01-01" })).toHaveLength(2);
  });

  test("uses air date as a fallback when no issue identity is available", () => {
    const dateOnly = [
      candidate("date-a", 0, "上：第一期", {
        episodePart: "upper",
        episodeEdition: "main",
        airDate: "2023-01-19",
      }),
      candidate("date-b", 0, "上：第二期", {
        episodePart: "upper",
        episodeEdition: "main",
        airDate: "2023-01-26",
      }),
    ];

    expect(
      selectEpisodeCandidates(dateOnly, undefined, { airDate: "2023-01-26" }).map((episode) => episode.episodeId),
    ).toEqual(["date-b"]);
    expect(
      selectEpisodeCandidates(dateOnly, 8, {
        episodeName: "本周正片",
        airDate: "2023-01-26",
      }).map((episode) => episode.episodeId),
    ).toEqual(["date-b"]);
  });

  test("keeps a proven exact source id while exposing the client episode coordinate", () => {
    expect(withClientEpisodeNumber(candidate("exact-source-4", 4, "第4期下"), 8)).toEqual(
      expect.objectContaining({
        episodeId: "exact-source-4",
        episodeNumber: 8,
      }),
    );
  });
});
