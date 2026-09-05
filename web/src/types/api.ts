/**
 * 新后端契约 (/api/v1):
 *  - 成功直返 data(语义化 HTTP status), 无 {code,data,msg} 信封;
 *  - 失败统一 RFC7807 application/problem+json: { code, message, traceId }。
 * 拦截器据此: 2xx 返 resp.data; 非 2xx 抛 ApiError。
 */

/** RFC7807 错误体 */
export interface Problem {
  code: number
  message: string
  traceId?: string
  details?: unknown
}

/** 分页元信息(对齐后端 dto.PageMeta) */
export interface PageMeta {
  current: number
  size: number
  total: number
  pageCount: number
}

/** 分页响应 */
export interface Paginated<T> {
  list: T[]
  page: PageMeta
}

/** 拦截器抛出的 API 错误(携带 HTTP status) */
export class ApiError extends Error {
  public readonly status: number
  public readonly code: number
  public readonly traceId?: string
  public readonly details?: unknown
  constructor(status: number, problem?: Partial<Problem>) {
    super(problem?.message ?? '请求失败')
    this.name = 'ApiError'
    this.status = status
    this.code = problem?.code ?? status
    this.traceId = problem?.traceId
    this.details = problem?.details
  }
}

/** 向后兼容别名(旧代码引用 BizError) */
export { ApiError as BizError }
