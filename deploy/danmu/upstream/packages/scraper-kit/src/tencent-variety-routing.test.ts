import { describe, expect, test } from "@rstest/core";
import { initializeFetchAdapter } from "./runtime";
import { TencentScraper } from "./scrapers/tencent";

const tencentResponse = (items: Array<{ vid: string; title: string; union_title?: string }>) => ({
  data: {
    module_list_datas: [
      {
        module_datas: [
          {
            item_data_lists: {
              item_datas: items.map((item) => ({
                item_params: {
                  ...item,
                  union_title: item.union_title ?? item.title,
                  is_trailer: "0",
                },
              })),
            },
          },
        ],
      },
    ],
  },
});

describe("Tencent variety routing", () => {
  test("keeps ordinary drama routing sequential and requests only the target page", async () => {
    const requests: unknown[] = [];
    initializeFetchAdapter({
      async get<_T>() {
        throw new Error("Unexpected GET request");
      },
      async post<T>(_url: string, body: unknown) {
        requests.push(body);
        return {
          data: tencentResponse([
            { vid: "drama-31", title: "第31集" },
            { vid: "promo-1", title: "主创陪你看第1期" },
            { vid: "promo-2", title: "主创陪你看第2期" },
            { vid: "drama-32", title: "第32集" },
          ]) as T,
          statusCode: 200,
          headers: {},
        };
      },
    });

    const episodes = await new TencentScraper().getEpisodes("cid=ordinary-drama", 31);

    expect(episodes).toEqual([
      expect.objectContaining({
        episodeId: "cid=ordinary-drama&vid=drama-31",
        episodeTitle: "第31集",
        episodeNumber: 31,
        episodePart: "whole",
        episodeEdition: "main",
      }),
    ]);
    expect(requests).toHaveLength(1);
    expect(requests[0]).toEqual(
      expect.objectContaining({
        page_params: expect.objectContaining({
          cid: "ordinary-drama",
          page_context: expect.stringContaining("episode_begin=31"),
        }),
      }),
    );
  });

  test("uses the curated variety list and maps S2E8 to 第4期下", async () => {
    const requests: unknown[] = [];
    initializeFetchAdapter({
      async get<_T>() {
        throw new Error("Unexpected GET request");
      },
      async post<T>(_url: string, body: unknown) {
        requests.push(body);
        return {
          data: tencentResponse([
            { vid: "v410293xxkh", title: "第4期上：憋笑分贝挑战" },
            { vid: "n41020iq5oi", title: "第4期下：高空滑翔伞" },
            { vid: "d4102u4vmbv", title: "游戏加更第4期：智力滑铁卢" },
          ]) as T,
          statusCode: 200,
          headers: {},
        };
      },
    });

    const scraper = new TencentScraper();
    const episodes = await scraper.getEpisodes("cid=mzc002005etgzma", 8, {
      episodeName: "第四期下",
    });

    expect(episodes).toEqual([
      expect.objectContaining({
        episodeId: "cid=mzc002005etgzma&vid=n41020iq5oi",
        episodeTitle: "第4期下：高空滑翔伞",
        episodeNumber: 8,
        episodePart: "lower",
        episodeEdition: "main",
      }),
    ]);
    expect(requests).toHaveLength(1);
    expect(requests[0]).toEqual(
      expect.objectContaining({
        page_params: expect.objectContaining({
          cid: "mzc002005etgzma",
          page_size: "",
          page_context: "",
        }),
      }),
    );
  });

  test("keeps a numbered episode whose subtitle contains 特别加长版", async () => {
    initializeFetchAdapter({
      async get<_T>() {
        throw new Error("Unexpected GET request");
      },
      async post<T>() {
        return {
          data: tencentResponse([
            { vid: "ep1", title: "第01话 塞外镖客 特别加长版", union_title: "塞外镖客 特别加长版_01" },
            { vid: "ep3", title: "第03话 风起云涌", union_title: "风起云涌_03" },
          ]) as T,
          statusCode: 200,
          headers: {},
        };
      },
    });

    const episodes = await new TencentScraper().getEpisodes("cid=mzc00200ab0trmk", 1);

    expect(episodes).toEqual([
      expect.objectContaining({
        episodeId: "cid=mzc00200ab0trmk&vid=ep1",
        episodeTitle: "塞外镖客 特别加长版_01",
        episodeNumber: 1,
        episodeEdition: "main",
      }),
    ]);
  });

  test("keeps a proven exact vid without requiring it to appear in Tencent's incomplete lists", async () => {
    initializeFetchAdapter({
      async get<_T>() {
        throw new Error("Unexpected GET request");
      },
      async post<_T>() {
        throw new Error("Exact vid must not be revalidated against an incomplete list");
      },
    });

    const scraper = new TencentScraper();
    const [episode] = await scraper.getEpisodes("cid=mzc002005etgzma&vid=n41020iq5oi", 8, {
      episodeName: "第四期下",
      airDate: "2026-07-19",
    });

    expect(episode).toEqual(
      expect.objectContaining({
        episodeId: "cid=mzc002005etgzma&vid=n41020iq5oi",
        episodeTitle: "第四期下",
        episodeNumber: 8,
        episodePart: "lower",
        episodeEdition: "main",
        airDate: "2026-07-19",
      }),
    );
  });
});
