import { describe, expect, test } from "@rstest/core";
import { initializeFetchAdapter } from "./runtime";
import { IqiyiScraper } from "./scrapers/iqiyi";

describe("iQiyi exact episode routing", () => {
  test("keeps a proven exact tvId and maps it to the requested sequential client episode", async () => {
    initializeFetchAdapter({
      async get<_T>() {
        throw new Error("Exact iQiyi tvId must not be revalidated against an album list");
      },
      async post<_T>() {
        throw new Error("Unexpected POST request");
      },
    });

    const scraper = new IqiyiScraper();
    const [episode] = await scraper.getEpisodes("entityId=8837160335043100&episodeId=8837160335043100", 11, {
      episodeName: "第4期中 Rapeter替爷爷写下跨时空情书",
      airDate: "2026-07-18",
    });

    expect(episode).toEqual({
      provider: "iqiyi",
      episodeId: "entityId=8837160335043100&episodeId=8837160335043100",
      episodeTitle: "第4期中 Rapeter替爷爷写下跨时空情书",
      episodeNumber: 11,
      episodePart: "middle",
      episodeEdition: "main",
      airDate: "2026-07-18",
    });
  });
});
