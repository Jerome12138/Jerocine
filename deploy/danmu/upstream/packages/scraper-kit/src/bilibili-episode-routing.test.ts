import { describe, expect, test } from "@rstest/core";
import { initializeFetchAdapter } from "./runtime";
import { BilibiliScraper } from "./scrapers/bilibili";

describe("Bilibili episode routing", () => {
  test("keeps ordinary series sequential after skipping previews", async () => {
    initializeFetchAdapter({
      async get<T>(url: string) {
        expect(url).toContain("season_id=ordinary-series");
        return {
          data: {
            code: 0,
            result: {
              episodes: [
                {
                  aid: 101,
                  cid: 201,
                  badge: "",
                  duration: 1_200_000,
                  title: "1",
                  show_title: "第1集",
                  long_title: "第一集",
                },
                {
                  aid: 102,
                  cid: 202,
                  badge: "预告",
                  duration: 60_000,
                  title: "预告",
                  show_title: "预告",
                  long_title: "预告",
                },
                {
                  aid: 104,
                  cid: 204,
                  badge: "",
                  duration: 60_000,
                  title: "花絮第1期",
                  show_title: "幕后花絮第1期",
                  long_title: "拍摄花絮",
                },
                {
                  aid: 103,
                  cid: 203,
                  badge: "",
                  duration: 1_200_000,
                  title: "2",
                  show_title: "第2集",
                  long_title: "第二集",
                },
              ],
            },
          } as T,
          statusCode: 200,
          headers: {},
        };
      },
      async post<_T>() {
        throw new Error("Unexpected POST request");
      },
    });

    const episodes = await new BilibiliScraper().getEpisodes("seasonId=ordinary-series", 2, {
      episodeName: "第二集",
    });

    expect(episodes).toEqual([
      expect.objectContaining({
        episodeId: "seasonId=ordinary-series&aid=103&cid=203",
        episodeTitle: "第2集",
        episodeNumber: 2,
        episodePart: "whole",
        episodeEdition: "main",
      }),
    ]);
  });

  test("maps a proven exact source issue to the requested client episode", async () => {
    initializeFetchAdapter({
      async get<T>(url: string) {
        expect(url).toContain("season_id=variety-series");
        return {
          data: {
            code: 0,
            result: {
              episodes: [
                {
                  aid: 104,
                  cid: 204,
                  badge: "",
                  duration: 1_200_000,
                  title: "4",
                  show_title: "第4期下",
                  long_title: "高空滑翔伞",
                },
              ],
            },
          } as T,
          statusCode: 200,
          headers: {},
        };
      },
      async post<_T>() {
        throw new Error("Unexpected POST request");
      },
    });

    const episodes = await new BilibiliScraper().getEpisodes("seasonId=variety-series&aid=104&cid=204", 8, {
      episodeName: "第四期下",
    });

    expect(episodes).toEqual([
      expect.objectContaining({
        episodeId: "seasonId=variety-series&aid=104&cid=204",
        episodeNumber: 8,
        episodePart: "lower",
      }),
    ]);
  });
});
