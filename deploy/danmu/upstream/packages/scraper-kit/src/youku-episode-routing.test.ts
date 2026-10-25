import { beforeEach, describe, expect, test } from "@rstest/core";
import { initializeFetchAdapter } from "./runtime";
import { YoukuScraper } from "./scrapers/youku";

const calls: string[] = [];

const rawVideo = ({
  id,
  title,
  seq,
  category,
  stage,
}: {
  id: string;
  title: string;
  seq: number;
  category: string;
  stage?: string;
}) => ({
  id,
  show_id: "show-1",
  title,
  seq,
  stage,
  published: "2025-02-28 12:00:00",
  duration: "2700",
  category,
  link: `https://v.youku.com/v_show/id_${id}.html`,
});

initializeFetchAdapter({
  async get<T>(url: string) {
    calls.push(url);
    if (url.includes("/videos/show_basic.json")) {
      return {
        data: rawVideo({
          id: "exact-vid",
          title: "上：具体综艺正片",
          seq: 99,
          category: "综艺",
          stage: "20250228",
        }) as T,
        statusCode: 200,
        headers: {},
      };
    }

    const parsedUrl = new URL(url);
    const page = Number(parsedUrl.searchParams.get("page"));
    const showId = parsedUrl.searchParams.get("show_id");
    if (showId === "tv-show") {
      return {
        data: {
          total: 40,
          videos: [
            rawVideo({
              id: `tv-${page}`,
              title: `普通剧第 ${page === 2 ? 21 : 1} 集`,
              seq: page === 2 ? 21 : 1,
              category: "电视剧",
            }),
          ],
        } as T,
        statusCode: 200,
        headers: {},
      };
    }
    if (showId === "variety-show") {
      return {
        data: {
          total: 40,
          videos: [
            rawVideo({
              id: `variety-${page}`,
              title: page === 1 ? "上：第一期" : "上：第二期",
              seq: page,
              category: "综艺",
              stage: page === 1 ? "20250228" : "20250307",
            }),
          ],
        } as T,
        statusCode: 200,
        headers: {},
      };
    }
    if (showId === "busy-variety") {
      return {
        data: {
          total: 16,
          videos: [
            rawVideo({ id: "issue-1", title: "第一周正片", seq: 1, category: "综艺", stage: "20260616" }),
            rawVideo({ id: "live-1", title: "直播回看：首期唠嗑局", seq: 2, category: "综艺", stage: "20260617" }),
            rawVideo({ id: "live-2", title: "直播回看：少年线上热聊", seq: 3, category: "综艺", stage: "20260620" }),
            rawVideo({ id: "live-3", title: "直播回看：陪看分享", seq: 4, category: "综艺", stage: "20260621" }),
            rawVideo({ id: "issue-2", title: "第二周正片", seq: 5, category: "综艺", stage: "20260623" }),
            rawVideo({ id: "classmate-2", title: "同学录：幕后趣事", seq: 6, category: "综艺", stage: "20260624" }),
            rawVideo({ id: "live-4", title: "直播回看：郑佑辰直播", seq: 7, category: "综艺", stage: "20260625" }),
            rawVideo({ id: "issue-3", title: "第三周正片", seq: 8, category: "综艺", stage: "20260630" }),
            rawVideo({ id: "issue-4", title: "第四周正片", seq: 9, category: "综艺", stage: "20260707" }),
            rawVideo({ id: "issue-5", title: "第五周正片", seq: 10, category: "综艺", stage: "20260714" }),
            rawVideo({ id: "issue-6", title: "第六周正片", seq: 11, category: "综艺", stage: "20260721" }),
            rawVideo({ id: "issue-7", title: "第七周正片", seq: 12, category: "综艺", stage: "20260728" }),
            rawVideo({
              id: "challenge-7",
              title: "少年的挑战：脑力猜词",
              seq: 13,
              category: "综艺",
              stage: "20260728",
            }),
          ],
        } as T,
        statusCode: 200,
        headers: {},
      };
    }
    throw new Error(`Unexpected GET url: ${url}`);
  },
  async post<T>() {
    return { data: null as T, statusCode: 405, headers: {} };
  },
});

describe("Youku episode routing", () => {
  beforeEach(() => {
    calls.length = 0;
  });

  test("a concrete vid bypasses collection pagination and cannot be replaced by seq", async () => {
    const episodes = await new YoukuScraper().getEpisodes("showId=show-1&vid=exact-vid", 1);

    expect(episodes).toHaveLength(1);
    expect(episodes[0]?.episodeId).toContain("vid=exact-vid");
    expect(episodes[0]?.episodeNumber).toBe(1);
    expect(calls.filter((url) => url.includes("/shows/videos.json"))).toHaveLength(0);
  });

  test("a concrete variety vid keeps the client episode coordinate instead of its source issue", async () => {
    const episodes = await new YoukuScraper().getEpisodes("showId=show-1&vid=exact-vid", 8, {
      episodeName: "第四期上",
    });

    expect(episodes).toEqual([
      expect.objectContaining({
        episodeId: "showId=show-1&vid=exact-vid",
        episodeNumber: 8,
        episodePart: "upper",
      }),
    ]);
  });

  test("an ordinary long series requests only its target page", async () => {
    const episodes = await new YoukuScraper().getEpisodes("showId=tv-show", 21);

    expect(episodes.map((episode) => episode.episodeNumber)).toEqual([21]);
    expect(calls.filter((url) => url.includes("/shows/videos.json"))).toHaveLength(1);
    expect(calls[0]).toContain("page=2");
  });

  test("a variety collection expands bounded pages so stage groups can be numbered", async () => {
    const episodes = await new YoukuScraper().getEpisodes("showId=variety-show", 2);

    expect(episodes.map((episode) => episode.episodeNumber)).toEqual([2]);
    expect(calls.filter((url) => url.includes("/shows/videos.json"))).toHaveLength(2);
  });

  test("numbers only main stages so live replays and companion clips cannot become the requested issue", async () => {
    const episodes = await new YoukuScraper().getEpisodes("showId=busy-variety", 7, {
      episodeName: "第7期",
      airDate: "2026-07-28",
    });

    expect(episodes.map((episode) => episode.episodeId)).toEqual(["showId=busy-variety&vid=issue-7"]);
    expect(episodes[0]).toEqual(
      expect.objectContaining({
        episodeNumber: 7,
        episodeEdition: "main",
        airDate: "2026-07-28",
      }),
    );
  });
});
