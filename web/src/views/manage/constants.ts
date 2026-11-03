import type { ManageFilmStatus } from '@/types/manage'
import type { StatusMap } from '@/components/manage/StatusTag.vue'

/**
 * 管理端跨页共享常量。
 * 两页以上相同的映射/页签提升到此处（manage-ui-spec 约定）:
 *  - FILM_STATUS_TABS / FILM_STATUS_TAG_MAP: 影片软删态（FilmListView 现状）,
 *    采集失败台账（FailureListView）用的是数值态页签, 与此互不相干、各自命名。
 */

/** 影片列表软删态页签（在架 / 回收站 / 全部） */
export const FILM_STATUS_TABS: { value: ManageFilmStatus; label: string }[] = [
  { value: 'active', label: '在架' },
  { value: 'deleted', label: '回收站' },
  { value: 'all', label: '全部' }
]

/** 影片软删态 → StatusTag 映射（deletedAt>0 即回收站） */
export const FILM_STATUS_TAG_MAP: StatusMap = {
  active: { label: '在架', variant: 'success' },
  deleted: { label: '回收站', variant: 'danger' }
}
