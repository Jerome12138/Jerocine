import type { ProviderEpisodeInfo } from "./base";
import { parseEpNumber } from "./parse-ep-number";

export type EpisodePart = "whole" | "upper" | "middle" | "lower";
export type EpisodeEdition = "main" | "bonus" | "preview" | "special";

export interface EpisodeMatchContext {
  episodeName?: string;
  airDate?: string;
}

export interface ParsedEpisodeIdentity {
  episodeNumber: number | null;
  part: EpisodePart;
  edition: EpisodeEdition;
}

const PART_ALIASES: Record<string, EpisodePart> = {
  上: "upper",
  中: "middle",
  下: "lower",
};

const PREVIEW_PATTERN = /先导|预告|抢先|超前|试看|预热|发布会/i;
const BONUS_PATTERN =
  /加更|番外|花絮|纯享|未播|彩蛋|删减|精彩(?:片段|看点|回顾|集锦)|看点|NG|PD\s*VLOG|VLOG|精编|会员(?:专享|专属|加长|版)|衍生|售后|幕后|饭局|跑图|猫咖|有猫腻|茶话会|专访|访谈|采访|片尾曲|插曲|主题曲|背景音乐|OST|MV|前季回顾|剧情回顾|往期回顾|内容总结|剧情盘点|合集|混剪|REACTION|短片|合唱|解锁中|开推吧[！!]?X/i;
const SPECIAL_PATTERN = /特别(?:篇|版|企划)|特辑|SP(?:\b|$)/i;

const normalizeText = (value: string) =>
  value
    .trim()
    .replace(/[０-９]/g, (char) => String.fromCharCode(char.charCodeAt(0) - 0xff10 + 0x30))
    .replace(/\u3000/g, " ");

const parseIssueNumber = (value: string) => {
  if (/^\d+$/.test(value)) {
    const number = Number.parseInt(value, 10);
    return Number.isFinite(number) ? number : null;
  }
  return parseEpNumber(`第${value}期`);
};

const parsePart = (title: string, issueMatch?: RegExpExecArray | null): EpisodePart => {
  const issuePart = issueMatch?.[2];
  if (issuePart && PART_ALIASES[issuePart]) return PART_ALIASES[issuePart];

  const leadingPart = /^(?:正片\s*)?[（(【[]?\s*([上中下])\s*[）)】\]]?\s*[:：]/.exec(title)?.[1];
  if (leadingPart && PART_ALIASES[leadingPart]) return PART_ALIASES[leadingPart];
  return "whole";
};

const parseEdition = (title: string): EpisodeEdition => {
  const label = title.split(/[：:]/, 1)[0] ?? title;
  if (PREVIEW_PATTERN.test(label)) return "preview";
  if (BONUS_PATTERN.test(label)) return "bonus";
  if (SPECIAL_PATTERN.test(label)) return "special";
  return "main";
};

export const normalizeAirDate = (value?: string) => {
  if (!value) return undefined;
  const match = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})/.exec(value.trim());
  if (!match) return undefined;
  return [match[1], match[2].padStart(2, "0"), match[3].padStart(2, "0")].join("-");
};

export const parseVarietyEpisodeIdentity = (rawTitle: string): ParsedEpisodeIdentity => {
  const title = normalizeText(rawTitle);
  if (!title) {
    return { episodeNumber: null, part: "whole", edition: parseEdition(title) };
  }

  const issueMatch =
    /第\s*([0-9]+|[零〇一二两三四五六七八九十百千万萬]+)\s*期(?:\s*[（(【[]?\s*([上中下])\s*[）)】\]]?)?/.exec(title);
  return {
    episodeNumber: issueMatch?.[1] ? parseIssueNumber(issueMatch[1]) : null,
    part: parsePart(title, issueMatch),
    edition: parseEdition(title),
  };
};

const EDITION_ORDER: Record<EpisodeEdition, number> = {
  main: 0,
  bonus: 1,
  special: 2,
  preview: 3,
};

const PART_ORDER: Record<EpisodePart, number> = {
  whole: 0,
  upper: 1,
  middle: 2,
  lower: 3,
};

export const sortEpisodeCandidates = <T extends ProviderEpisodeInfo>(items: T[]) =>
  [...items].sort((left, right) => {
    const numberOrder = left.episodeNumber - right.episodeNumber;
    if (numberOrder !== 0) return numberOrder;
    const editionOrder = EDITION_ORDER[left.episodeEdition ?? "main"] - EDITION_ORDER[right.episodeEdition ?? "main"];
    if (editionOrder !== 0) return editionOrder;
    const partOrder = PART_ORDER[left.episodePart ?? "whole"] - PART_ORDER[right.episodePart ?? "whole"];
    if (partOrder !== 0) return partOrder;
    return (left.airDate ?? "").localeCompare(right.airDate ?? "");
  });

export const withClientEpisodeNumber = <T extends ProviderEpisodeInfo>(episode: T, episodeNumber?: number): T =>
  episodeNumber === undefined || episode.episodeNumber === episodeNumber ? episode : { ...episode, episodeNumber };

export const selectEpisodeCandidates = <T extends ProviderEpisodeInfo>(
  candidates: T[],
  episodeNumber?: number,
  context: EpisodeMatchContext = {},
) => {
  const requestedIdentity = parseVarietyEpisodeIdentity(context.episodeName ?? "");
  const requestedAirDate = normalizeAirDate(context.airDate);
  const hasExplicitIssue = requestedIdentity.episodeNumber !== null;
  let matchedClientEpisode = false;
  let selected = candidates.filter((candidate) => candidate.episodeEdition !== "preview");
  if (hasExplicitIssue) {
    selected = selected.filter((candidate) => candidate.episodeNumber === requestedIdentity.episodeNumber);
  } else if (episodeNumber !== undefined) {
    const sameClientEpisode = selected.filter((candidate) => candidate.episodeNumber === episodeNumber);
    if (sameClientEpisode.length) {
      selected = sameClientEpisode;
      matchedClientEpisode = true;
    } else if (!requestedAirDate) {
      return [];
    }
  }
  if (!selected.length) return [];

  if (requestedIdentity.edition !== "main") {
    const sameEdition = selected.filter((candidate) => candidate.episodeEdition === requestedIdentity.edition);
    if (!sameEdition.length) return [];
    selected = sameEdition;
  } else {
    const main = selected.filter((candidate) => (candidate.episodeEdition ?? "main") === "main");
    if (main.length) selected = main;
  }

  if (requestedIdentity.part !== "whole") {
    const samePart = selected.filter((candidate) => candidate.episodePart === requestedIdentity.part);
    if (!samePart.length) return [];
    selected = samePart;
  } else {
    const whole = selected.filter((candidate) => (candidate.episodePart ?? "whole") === "whole");
    if (whole.length) selected = whole;
  }

  if (requestedAirDate && !hasExplicitIssue) {
    const sameDate = selected.filter((candidate) => normalizeAirDate(candidate.airDate) === requestedAirDate);
    if (sameDate.length) {
      selected = sameDate;
    } else if (!matchedClientEpisode) {
      return [];
    }
  }

  return sortEpisodeCandidates(selected).map((candidate) => withClientEpisodeNumber(candidate, episodeNumber));
};

export const isVarietyEpisodeList = (titles: string[], context: EpisodeMatchContext = {}) => {
  const requestedTitle = context.episodeName?.trim();
  if (requestedTitle && parseVarietyEpisodeIdentity(requestedTitle).episodeNumber === null) {
    return false;
  }
  const dramaEpisodes = titles.flatMap((title) => {
    const match = /^(?:正片\s*)?第\s*([0-9]+|[零〇一二两三四五六七八九十百千万萬]+)\s*(?:集|话|話|回)/.exec(
      normalizeText(title),
    );
    const episodeNumber = match?.[1] ? parseIssueNumber(match[1]) : null;
    return episodeNumber === null ? [] : [episodeNumber];
  });
  if (new Set(dramaEpisodes).size >= 2) {
    return false;
  }
  const mainIssues = titles.flatMap((title) => {
    const identity = parseVarietyEpisodeIdentity(title);
    return identity.episodeNumber !== null && identity.edition === "main" ? [identity.episodeNumber] : [];
  });
  return new Set(mainIssues).size >= 2;
};
