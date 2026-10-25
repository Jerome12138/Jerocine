import Base64 from "crypto-js/enc-base64.js";
import Utf8 from "crypto-js/enc-utf8.js";
import MD5 from "crypto-js/md5.js";
import { safeJsonParseWithZod, z } from "../../runtime";
import { BaseScraper, type ProviderEpisodeInfo } from "../base";
import {
  type EpisodeMatchContext,
  normalizeAirDate,
  parseVarietyEpisodeIdentity,
  selectEpisodeCandidates,
} from "../episode-identity";
import { youkuDanmuResultSchema, youkuEpisodeInfoSchema, youkuIdSchema, youkuVideoResultSchema } from "./schema";

type YoukuEpisodeInfo = z.infer<typeof youkuEpisodeInfoSchema>;

export class YoukuScraper extends BaseScraper<typeof youkuIdSchema> {
  providerName = "youku";

  idSchema = youkuIdSchema;

  protected PROVIDER_SPECIFIC_BLACKLIST =
    "^(.*?)(抢先(版|篇)?|加更(版|篇)?|花絮|预告|特辑|彩蛋|专访|幕后(故事|花絮)?|直播|纯享|未播|衍生|会员(专属|加长)?|片花|精华|看点|速览|解读|reaction|影评|少年的挑战|同学录)(.*?)$";

  async parseProviderUrl(url: URL) {
    if (!url.hostname.includes("youku.com")) {
      return null;
    }

    let showId =
      url.searchParams.get("showid") || url.searchParams.get("showId") || url.searchParams.get("s") || undefined;
    const vid = url.searchParams.get("vid") || url.pathname.match(/\/v_show\/id_(.+)\.html$/)?.[1] || undefined;
    if (!showId && !vid) {
      return null;
    }
    if (!showId && vid) {
      showId = (await this.getVideoInfo(vid))?.show_id || undefined;
    }

    return {
      ...(showId ? { showId } : {}),
      ...(vid ? { vid } : {}),
    };
  }

  constructor() {
    super();
    this.fetch.setHeaders({
      "User-Agent":
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
    });
  }

  private get token() {
    const tokenValue = this.fetch.getCookie("_m_h5_tk")?.split("_")[0] ?? "";
    return tokenValue.substring(0, 32);
  }
  private get cna() {
    return this.fetch.getCookie("cna") ?? "";
  }

  async getEpisodes(idString: string, episodeNumber?: number, context?: EpisodeMatchContext) {
    const youkuId = this.parseIdString(idString);
    if (!youkuId) {
      return [];
    }
    let showId = youkuId.showId;
    if (youkuId.vid) {
      const videoInfo = await this.getVideoInfo(youkuId.vid);
      showId = showId || videoInfo?.show_id || "";
      if (!showId || !videoInfo) {
        return [];
      }
      const isVariety = videoInfo.category.includes("综艺");
      const identity = isVariety
        ? parseVarietyEpisodeIdentity(videoInfo.title)
        : { episodeNumber: null, part: "whole" as const, edition: "main" as const };
      const stageAirDate = videoInfo.stage?.match(/^(\d{4})(\d{2})(\d{2})$/);
      return [
        {
          provider: this.providerName,
          episodeId: this.generateIdString({ showId, vid: videoInfo.id }),
          episodeTitle: videoInfo.title,
          episodeNumber: episodeNumber ?? identity.episodeNumber ?? videoInfo.seq ?? 0,
          episodePart: identity.part,
          episodeEdition: identity.edition,
          airDate: stageAirDate
            ? normalizeAirDate(`${stageAirDate[1]}-${stageAirDate[2]}-${stageAirDate[3]}`)
            : normalizeAirDate(videoInfo.published),
        },
      ];
    }
    if (!showId) {
      return [];
    }

    const pageSize = 20;
    const targetPage = episodeNumber ? Math.max(1, Math.ceil(episodeNumber / pageSize)) : 1;

    try {
      const initialPage = await this.getEpisodesPage(showId, targetPage, pageSize);
      const initialVideos = [...((initialPage?.videos ?? []) as YoukuEpisodeInfo[])];
      const isVariety = initialVideos.some((video) => video.category.includes("综艺"));

      if (!isVariety) {
        const sourceVideos = initialVideos.filter((video) => !this.episodeBlacklistPattern.test(video.title));
        const results = sourceVideos.map<ProviderEpisodeInfo>((video, index) => ({
          provider: this.providerName,
          episodeId: this.generateIdString({ showId, vid: video.id }),
          episodeTitle: video.title,
          episodeNumber: video.seq ?? (targetPage - 1) * pageSize + index + 1,
          episodePart: "whole",
          episodeEdition: "main",
        }));
        return selectEpisodeCandidates(results, episodeNumber, context);
      }

      const total = Number(initialPage?.total ?? 0);
      const totalPages = Math.max(1, Math.ceil(total / pageSize));
      const maxPages = Math.min(totalPages, 20);
      const allVideos = [...initialVideos];
      for (let page = 1; page <= maxPages; page += 1) {
        if (page === targetPage) continue;
        const pageResult = await this.getEpisodesPage(showId, page, pageSize);
        allVideos.push(...((pageResult?.videos ?? []) as YoukuEpisodeInfo[]));
      }

      const sortedVideos = [...allVideos].sort(
        (left, right) =>
          (left.stage ?? left.published ?? "").localeCompare(right.stage ?? right.published ?? "") ||
          (left.seq ?? Number.MAX_SAFE_INTEGER) - (right.seq ?? Number.MAX_SAFE_INTEGER),
      );

      const mainStages = Array.from(
        new Set(
          sortedVideos
            .filter(
              (video) =>
                parseVarietyEpisodeIdentity(video.title).edition === "main" &&
                !this.episodeBlacklistPattern.test(video.title),
            )
            .map((video) => video.stage)
            .filter((stage): stage is string => Boolean(stage)),
        ),
      );
      const stageIssueMap = new Map(mainStages.map((stage, index) => [stage, index + 1]));

      const results = sortedVideos.map<ProviderEpisodeInfo>((video) => {
        const identity = parseVarietyEpisodeIdentity(video.title);
        const providerSupplement = identity.edition === "main" && this.episodeBlacklistPattern.test(video.title);
        const stageAirDate = video.stage?.match(/^(\d{4})(\d{2})(\d{2})$/);
        return {
          provider: this.providerName,
          episodeId: this.generateIdString({ showId, vid: video.id }),
          episodeTitle: video.title,
          episodeNumber: identity.episodeNumber ?? stageIssueMap.get(video.stage ?? "") ?? 0,
          episodePart: identity.part,
          episodeEdition: providerSupplement ? "bonus" : identity.edition,
          airDate: stageAirDate
            ? normalizeAirDate(`${stageAirDate[1]}-${stageAirDate[2]}-${stageAirDate[3]}`)
            : normalizeAirDate(video.published),
        };
      });

      return selectEpisodeCandidates(results, episodeNumber, context);
    } catch (error) {
      this.logger.error("获取分集失败，showId：", showId, "错误：", error);
      return [];
    }
  }

  async getSegments(idString: string) {
    const { vid } = this.parseIdString(idString) ?? {};
    if (!vid) {
      return [];
    }

    try {
      // 确保token和cookie已设置
      await this.ensureTokenCookie();
      const episodeInfo = await this.getVideoInfo(vid);
      if (!episodeInfo) {
        this.logger.warn("获取分集信息失败，vid：", vid);
        return [];
      }

      const totalMat = episodeInfo.totalMat;

      if (totalMat === 0) {
        this.logger.warn("视频时长为0，无法获取弹幕，vid：", vid);
        return [];
      }

      return Array.from({ length: totalMat }, (_, i) => ({
        provider: this.providerName,
        startTime: i * 60,
        segmentId: i.toString(),
      }));
    } catch (error) {
      this.logger.error("获取分段信息失败，vid：", vid, "错误：", error);
      return [];
    }
  }

  async getComments(idString: string, segmentId: string) {
    const { vid } = this.parseIdString(idString) ?? {};
    if (!vid) {
      return [];
    }
    try {
      // 确保token和cookie已设置
      await this.ensureTokenCookie();

      return this.getDanmuContentByMat(vid, parseInt(segmentId, 10));
    } catch (error) {
      this.logger.error("获取弹幕信息失败，vid：", vid, "错误：", error);
      return [];
    }
  }

  private async getVideoInfo(vid: string) {
    const response = await this.fetch.get("https://openapi.youku.com/v2/videos/show_basic.json", {
      params: {
        client_id: "53e6cc67237fc59a",
        package: "com.huawei.hwvplayer.youku",
        video_id: vid,
      },
      schema: youkuEpisodeInfoSchema,
      cache: {
        cacheKey: `youku:segments:${vid}`,
      },
    });
    return response.data;
  }

  private async getEpisodesPage(showId: string, page: number, pageSize: number) {
    const response = await this.fetch.get("https://openapi.youku.com/v2/shows/videos.json", {
      params: {
        client_id: "53e6cc67237fc59a",
        package: "com.huawei.hwvplayer.youku",
        ext: "show",
        show_id: showId,
        page: page.toString(),
        count: pageSize.toString(),
      },
      schema: youkuVideoResultSchema,
      cache: {
        cacheKey: `youku:episodes:${showId}:${page}:${pageSize}`,
      },
    });
    return response.data;
  }

  private async getDanmuContentByMat(vid: string, mat: number) {
    if (!this.token) {
      this.logger.error("无法获取弹幕，_m_h5_tk 缺失");
      return [];
    }

    const msg: Record<string, string | number> = {
      pid: 0,
      ctype: 10004,
      sver: "3.1.0",
      cver: "v1.0",
      ctime: Date.now(),
      guid: this.cna,
      vid: vid,
      mat: mat,
      mcount: 1,
      type: 1,
    };

    const msgOrderedStr = JSON.stringify(Object.fromEntries(Object.entries(msg).sort()));
    const msgEnc = Base64.stringify(Utf8.parse(msgOrderedStr));

    msg.msg = msgEnc;
    msg.sign = this.generateMsgSign(msgEnc);

    const appKey = "24679788";
    const dataPayload = JSON.stringify(msg);
    const t = Date.now().toString();

    try {
      const response = await this.fetch.get("https://acs.youku.com/h5/mopen.youku.danmu.list/1.0/", {
        params: {
          jsv: "2.7.0",
          appKey: appKey,
          t,
          sign: this.generateTokenSign(t, appKey, dataPayload),
          api: "mopen.youku.danmu.list",
          v: "1.0",
          type: "originaljson",
          dataType: "jsonp",
          timeout: "20000",
          jsonpIncPrefix: "utility",
          data: dataPayload,
        },
        headers: { Referer: "https://v.youku.com" },
        successStatus: [200],
        schema: z.looseObject({
          data: z.object({
            result: z
              .string()
              .transform((v) => safeJsonParseWithZod(v, youkuDanmuResultSchema))
              .optional(),
          }),
        }),
      });

      const result = response.data?.data.result?.data.result ?? [];
      this.logger.info("获取到分段", mat, "的弹幕", result.length, "条");

      return result;
    } catch (error) {
      this.logger.error("解析弹幕响应失败，vid：", vid, "mat：", mat, "错误：", error);
      return [];
    }
  }

  private generateMsgSign(msgEnc: string) {
    return MD5(`${msgEnc}MkmC9SoIw6xCkSKHhJ7b5D2r51kBiREr`).toString().toLowerCase();
  }

  private generateTokenSign(t: string, appKey: string, dataPayload: string) {
    return MD5([this.token, t, appKey, dataPayload].join("&")).toString().toLowerCase();
  }

  /**
   * 确保获取弹幕签名所需的 cna 和 _m_h5_tk cookie。
   * 此逻辑严格参考了 Python 代码，并针对网络环境进行了优化。
   */
  private async ensureTokenCookie() {
    this.fetch.cookie = {};

    // 步骤 1: 获取 'cna' cookie。它通常由优酷主站或其统计服务设置。
    try {
      await this.fetch.get("https://log.mmstat.com/eg.js", {
        headers: {
          Cookie: "",
          "If-None-Match": "",
        },
      });
    } catch (error) {
      this.logger.warn("无法连接到 youku.com 获取 'cna' cookie。错误：", error);
    }

    // 步骤 2: 获取 '_m_h5_tk' 令牌, 此请求可能依赖于 'cna' cookie 的存在。
    try {
      await this.fetch.get("https://acs.youku.com/h5/mtop.com.youku.aplatform.weakget/1.0/?jsv=2.5.1&appKey=24679788");
    } catch (error) {
      this.logger.error("无法连接到 acs.youku.com 获取令牌 cookie。弹幕获取很可能会失败。错误：", error);
    }

    if (!this.cna || !this.token) {
      this.logger.warn("未能获取到弹幕签名所需的全部 cookie。 cna：", this.cna, "token：", this.token);
    }
  }
}
