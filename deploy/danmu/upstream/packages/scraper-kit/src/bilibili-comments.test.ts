import { describe, expect, test } from "@rstest/core";
import type { HttpAdapterRequestOptions, HttpResponse } from "./runtime";
import { initializeFetchAdapter } from "./runtime";
import { BilibiliScraper } from "./scrapers/bilibili";

describe("Bilibili comments", () => {
  test("uses a browser user agent and decodes replies with newer unknown envelope fields", async () => {
    initializeFetchAdapter({
      async get<T>(url: string, options?: HttpAdapterRequestOptions): Promise<HttpResponse<T>> {
        if (url.includes("/pgc/view/web/ep/list")) {
          return {
            data: {
              code: 0,
              result: {
                episodes: [
                  {
                    aid: 116974223032397,
                    cid: 40256408429,
                    badge: "",
                    duration: 6257960,
                    title: "第7期",
                    show_title: "第7期 深情派 vs 旷野派",
                    long_title: "深情派 vs 旷野派",
                  },
                ],
              },
            } as T,
            statusCode: 200,
            headers: {},
          };
        }
        if (url.includes("/x/v2/dm/web/seg.so")) {
          expect(options?.headers?.["User-Agent"]).toContain("Mozilla/5.0");
          expect(options?.base64Data).toBe(true);
          return {
            data: "ChUIARDoBxgBIBko////BzoCb2tiATEiAQAqAQA=" as T,
            statusCode: 200,
            headers: {
              "content-type": "application/octet-stream",
            },
          };
        }
        throw new Error(`Unexpected GET request: ${url}`);
      },
      async post<_T>() {
        throw new Error("Unexpected POST request");
      },
    });

    const comments = await new BilibiliScraper().getComments(
      "seasonId=239101&aid=116974223032397&cid=40256408429",
      "1",
    );

    expect(comments).toEqual([
      expect.objectContaining({
        id: "1",
        timestamp: 1,
        mode: 1,
        color: 16777215,
        content: "ok",
      }),
    ]);
  });
});
