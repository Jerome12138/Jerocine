import { describe, expect, test } from "@rstest/core";
import { IqiyiScraper } from "./scrapers/iqiyi";
import { MgTVScraper } from "./scrapers/mgtv";
import { RenRenScraper } from "./scrapers/renren";
import { TencentScraper } from "./scrapers/tencent";

describe("exact provider episode coordinates", () => {
  test("uses episode one when an exact Tencent or iQiyi coordinate has no client episode context", async () => {
    await expect(new TencentScraper().getEpisodes("cid=exact-show&vid=exact-video")).resolves.toEqual([
      expect.objectContaining({
        episodeId: "cid=exact-show&vid=exact-video",
        episodeNumber: 1,
      }),
    ]);
    await expect(new IqiyiScraper().getEpisodes("entityId=exact-video&episodeId=exact-video")).resolves.toEqual([
      expect.objectContaining({
        episodeId: "entityId=exact-video&episodeId=exact-video",
        episodeNumber: 1,
      }),
    ]);
  });

  test("maps an exact MGTV source issue to the requested client episode", async () => {
    const scraper = new MgTVScraper();
    Reflect.set(scraper, "getEpisodeInfo", async () => [
      {
        isIntact: "1",
        isnew: "1",
        video_id: "18368257",
        t1: "第4期下",
        t2: "2026-07-18",
        t3: "嘉宾继续旅行",
        time: 3600,
      },
    ]);

    await expect(
      scraper.getEpisodes("dramaId=496452&videoId=18368257", 8, {
        episodeName: "第四期下",
      }),
    ).resolves.toEqual([
      expect.objectContaining({
        episodeId: "dramaId=496452&videoId=18368257",
        episodeNumber: 8,
        episodePart: "lower",
      }),
    ]);
  });

  test("maps an exact Renren source episode to the requested client episode", async () => {
    const scraper = new RenRenScraper();
    Reflect.set(scraper, "getDramaInfo", async () => ({
      dramaInfo: {
        dramaId: 42,
        title: "示例综艺",
        seasonNo: 1,
      },
      episodeList: [
        {
          id: 104,
          episodeNo: 4,
          title: "第四期",
        },
      ],
    }));

    await expect(scraper.getEpisodes("dramaId=42&episodeId=104", 8)).resolves.toEqual([
      expect.objectContaining({
        episodeId: "dramaId=42&episodeId=104",
        episodeNumber: 8,
      }),
    ]);
  });
});
