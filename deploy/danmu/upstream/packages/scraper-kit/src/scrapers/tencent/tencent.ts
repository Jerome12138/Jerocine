import parseUrl from "url-parse";
import { TTL_2_HOURS } from "../../runtime";
import { BaseScraper, type ProviderEpisodeInfo } from "../base";
import {
  type EpisodeMatchContext,
  isVarietyEpisodeList,
  parseVarietyEpisodeIdentity,
  selectEpisodeCandidates,
} from "../episode-identity";
import {
  type TencentSegmentIndex,
  tencentEpisodeResultSchema,
  tencentIdSchema,
  tencentSegmentIndexSchema,
  tencentSegmentSchema,
} from "./schema";

const pageSize = 30;
const { qs } = parseUrl;

export class TencentScraper extends BaseScraper<typeof tencentIdSchema> {
  providerName = "tencent";

  idSchema = tencentIdSchema;

  protected PROVIDER_SPECIFIC_BLACKLIST = [
    "拍摄花絮",
    "制作花絮",
    "幕后花絮",
    "未播花絮",
    "独家花絮",
    "花絮特辑",
    "预告片",
    "先导预告",
    "终极预告",
    "正式预告",
    "官方预告",
    "彩蛋片段",
    "删减片段",
    "未播片段",
    "番外彩蛋",
    "精彩片段",
    "精彩看点",
    "精彩回顾",
    "精彩集锦",
    "看点解析",
    "看点预告",
    "NG镜头",
    "NG花絮",
    "番外篇",
    "番外特辑",
    "制作特辑",
    "拍摄特辑",
    "幕后特辑",
    "导演特辑",
    "演员特辑",
    "片尾曲",
    "插曲",
    "主题曲",
    "背景音乐",
    "OST",
    "音乐MV",
    "歌曲MV",
    "前季回顾",
    "剧情回顾",
    "往期回顾",
    "内容总结",
    "剧情盘点",
    "精选合集",
    "剪辑合集",
    "混剪视频",
    "独家专访",
    "演员访谈",
    "导演访谈",
    "主创访谈",
    "媒体采访",
    "发布会采访",
    "抢先看",
    "抢先版",
    "试看版",
    "短剧",
    "vlog",
    "纯享",
    "加更",
    "reaction",
    "精编",
    "会员版",
    "Plus",
    "独家版",
    "特别版",
    "短片",
    "合唱",
  ].join("|");

  async parseProviderUrl(url: URL) {
    if (url.hostname !== "v.qq.com") {
      return null;
    }

    const withVidMatch = url.pathname.match(/^\/x\/cover\/([^/]+)\/([^/.]+)\.html$/);
    if (withVidMatch) {
      return {
        cid: withVidMatch[1],
        vid: withVidMatch[2],
      };
    }

    const cidOnlyMatch = url.pathname.match(/^\/x\/cover\/([^/.]+)\.html$/);
    if (cidOnlyMatch) {
      return { cid: cidOnlyMatch[1] };
    }

    const cid = url.searchParams.get("cid") ?? undefined;
    const vid = url.searchParams.get("vid") ?? undefined;
    if (cid) {
      return { cid, vid: vid || undefined };
    }

    return null;
  }

  constructor() {
    super();
    this.fetch.setCookie({
      pgv_pvid: "40b67e3b06027f3d",
      video_platform: "2",
      vversion_name: "8.2.95",
      video_bucketid: "4",
      video_omgid: "0a1ff6bc9407c0b1cff86ee5d359614d",
    });
    this.fetch.setHeaders({
      "User-Agent":
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
    });
  }

  async getEpisodes(idString: string, episodeNumber?: number, context?: EpisodeMatchContext) {
    const tencentId = this.parseIdString(idString);
    if (!tencentId) {
      return [];
    }
    const requestedIdentity = parseVarietyEpisodeIdentity(context?.episodeName ?? "");
    if (tencentId.vid) {
      return [
        {
          provider: this.providerName,
          episodeId: this.generateIdString(tencentId),
          episodeTitle: context?.episodeName?.trim() || tencentId.vid,
          episodeNumber: episodeNumber ?? requestedIdentity.episodeNumber ?? 1,
          episodePart: requestedIdentity.part,
          episodeEdition: requestedIdentity.edition,
          airDate: context?.airDate,
        },
      ];
    }

    // 获取指定cid的所有分集列表
    // mediaId 对于腾讯来说就是 cid
    const tencentEpisodes =
      requestedIdentity.episodeNumber === null
        ? await this.internalGetEpisodes(tencentId.cid, episodeNumber, context)
        : await this.getCuratedEpisodes(tencentId.cid);
    return selectEpisodeCandidates(tencentEpisodes, episodeNumber, context);
  }

  async getSegments(idString: string) {
    const tencentId = this.parseIdString(idString);
    if (!tencentId) {
      return [];
    }

    let segmentIndex: TencentSegmentIndex["segment_index"] = {};
    try {
      const response = await this.fetch.get(`https://dm.video.qq.com/barrage/base/${tencentId.vid}`, {
        schema: tencentSegmentIndexSchema,
        cache: {
          cacheKey: `tencent:segment:${tencentId.vid}`,
          ttl: TTL_2_HOURS,
        },
      });

      if (!response.data) {
        return [];
      }

      if (!response.data?.segment_index) {
        this.logger.info("vid：", tencentId.vid, "没有找到弹幕分段索引。");
        return [];
      }
      segmentIndex = response.data.segment_index;
    } catch (e) {
      this.logger.error("获取弹幕索引失败，vid：", tencentId.vid, "错误：", e);
      return [];
    }

    const sortedKeys = Object.keys(segmentIndex).sort((a, b) => parseInt(a, 10) - parseInt(b, 10));
    this.logger.debug("为 vid：", tencentId.vid, "找到", sortedKeys.length, "个弹幕分段");

    return sortedKeys.map((key) => {
      return {
        provider: this.providerName,
        startTime: parseInt(key, 10) / 1000.0,
        segmentId: segmentIndex[key]?.segment_name,
      };
    });
  }

  async getComments(idString: string, segmentId: string) {
    const tencentId = this.parseIdString(idString);
    if (!tencentId?.vid) {
      return [];
    }
    const response = await this.fetch.get(`https://dm.video.qq.com/barrage/segment/${tencentId.vid}/${segmentId}`, {
      schema: tencentSegmentSchema,
    });
    const comments = response.data?.barrage_list ?? [];
    this.logger.info("找到", comments.length, "条弹幕");
    return comments;
  }

  private isExcludedEpisode(item: { title: string; union_title?: string }) {
    const titles = [item.title, item.union_title].filter((title): title is string => Boolean(title));
    // 第N话/集 can contain 特别加长版 and still be the real episode.
    if (titles.some((title) => /第\s*[0-9]+\s*(?:集|话|話|回)/.test(title))) return false;
    return titles.some((title) => this.episodeBlacklistPattern.test(title));
  }

  private async requestEpisodes(
    cid: string,
    pageParams: Record<string, unknown>,
    cache: string | { cacheKey: string; ttl: number },
    forceVariety = false,
    context?: EpisodeMatchContext,
  ): Promise<ProviderEpisodeInfo[]> {
    this.fetch.setHeaders({
      Referer: `https://v.qq.com/x/cover/${cid}.html`,
    });

    const response = await this.fetch.post(
      "https://pbaccess.video.qq.com/trpc.universal_backend_service.page_server_rpc.PageServer/GetPageData",
      {
        page_params: {
          page_id: "vsite_episode_list",
          page_type: "detail_operation",
          cid,
          id_type: "1",
          req_from: "web_vsite",
          ...pageParams,
        },
        has_cache: 1,
      },
      {
        params: {
          video_appid: "3000010",
          vplatform: "2",
        },
        headers: {
          "Content-Type": "application/json",
        },
        schema: tencentEpisodeResultSchema,
        cache,
      },
    );
    if (!response.data) {
      return [];
    }
    const titles = response.data
      .filter((item) => !this.isExcludedEpisode(item))
      .map((item) => (item.union_title && item.union_title !== item.title ? item.union_title : item.title));
    const isVariety = forceVariety || isVarietyEpisodeList(titles, context);
    const sourceItems = isVariety ? response.data : response.data.filter((item) => !this.isExcludedEpisode(item));
    return sourceItems.map((item: { vid: string; title: string; union_title?: string }) => {
      const title = item.union_title && item.union_title !== item.title ? item.union_title : item.title;
      const identity = isVariety
        ? parseVarietyEpisodeIdentity(title)
        : { episodeNumber: null, part: "whole" as const, edition: "main" as const };
      return {
        provider: this.providerName,
        episodeId: this.generateIdString({ cid, vid: item.vid }),
        episodeTitle: title,
        episodeNumber: identity.episodeNumber ?? (isVariety ? 0 : (this.getEpisodeIndexFromTitle(title) ?? 0)),
        episodePart: identity.part,
        episodeEdition: identity.edition,
      };
    });
  }

  private async getCuratedEpisodes(cid: string): Promise<ProviderEpisodeInfo[]> {
    return this.requestEpisodes(
      cid,
      {
        page_size: "",
        vid: "",
        lid: "",
        page_num: "",
        page_context: "",
        detail_page_type: "1",
      },
      {
        cacheKey: `tencent:episodes:${cid}:curated`,
        ttl: TTL_2_HOURS,
      },
      true,
    );
  }

  private async getEpisodesPage(cid: string, page = 0, context?: EpisodeMatchContext): Promise<ProviderEpisodeInfo[]> {
    return this.requestEpisodes(
      cid,
      {
        page_context: qs.stringify({
          chapter_name: "",
          cid,
          detail_page_type: "1",
          episode_begin: page * pageSize + 1,
          episode_end: page * pageSize + pageSize,
          episode_step: pageSize,
          id_type: "1",
          lid: "",
          list_page_context: "",
          mvl_strategy_id: "",
          need_tab: "1",
          order: "",
          page_num: page,
          page_size: pageSize + 4,
          req_from: "web_vsite",
          req_from_second_type: "",
          req_type: "0",
          siteName: "",
          sub_chapter_name: "",
          tab_type: "1",
        }),
      },
      `tencent:episodes:${cid}:${page}`,
      false,
      context,
    );
  }

  /**
   * 获取指定cid的所有分集列表。
   * 处理了腾讯视频复杂的分页逻辑。
   */
  private async internalGetEpisodes(
    cid: string,
    episodeNumber?: number,
    context?: EpisodeMatchContext,
  ): Promise<ProviderEpisodeInfo[]> {
    if (!episodeNumber) {
      return this.getEpisodesPage(cid, 0, context);
    }

    const page = Math.floor((episodeNumber - 1) / pageSize);
    const episodes = await this.getEpisodesPage(cid, page, context);
    if (episodes.find((ep) => ep.episodeNumber === episodeNumber)) {
      return episodes;
    }
    const maxEp = Math.max(...episodes.map((ep) => ep.episodeNumber ?? 0));
    const minEp = Math.min(...episodes.map((ep) => ep.episodeNumber ?? 0));
    if (episodeNumber > maxEp) {
      return this.getEpisodesPage(cid, page + 1, context);
    }
    if (episodeNumber < minEp) {
      return this.getEpisodesPage(cid, page - 1, context);
    }
    return [];
  }
}
