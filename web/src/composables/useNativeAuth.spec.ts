import { describe, it, expect, vi, beforeEach } from 'vitest'

/**
 * useNativeAuth —— 原生抽屉「账号」行的数据源(window.__jcAuth 钩子)。
 *
 * 锁三件事:
 *  1. 快照解析的兜底(任何异常报文都归一成"未登录", 别把 Java 端炸出一个裸 JSON 异常);
 *  2. 钩子只在原生壳里安装(浏览器不挂全局);
 *  3. login/logout 语义: login → 跳 /login?redirect=当前页; logout → store.logout()。
 */

const mocks = vi.hoisted(() => ({
  native: { value: true },
  logout: vi.fn(() => Promise.resolve()),
  push: vi.fn(() => Promise.resolve())
}))

vi.mock('@/utils/jerocineNative', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/jerocineNative')>()
  return {
    ...actual,
    isNative: () => mocks.native.value
  }
})

const authState = vi.hoisted(() => ({ token: '', name: '' }))
vi.mock('@/stores/user', () => ({
  useUserStore: () => ({
    get isLoggedIn(): boolean {
      return authState.token.length > 0
    },
    get displayName(): string {
      return authState.name
    },
    logout: mocks.logout
  })
}))

// 整体 mock '@/router'(不要 mock vue-router —— 真实 router/index.ts 会在模块顶层
// createRouter, 被 vue-router mock 掉后模块求值直接炸)
vi.mock('@/router', () => ({
  default: {
    push: mocks.push,
    currentRoute: { value: { fullPath: '/index' } }
  }
}))

import {
  authSnapshot,
  normalizeAuthSnapshot,
  installNativeAuthHook,
  openWebLogin,
  webLogout
} from './useNativeAuth'

describe('normalizeAuthSnapshot (报文兜底)', () => {
  it('正常报文原样解析', () => {
    expect(normalizeAuthSnapshot('{"loggedIn":true,"name":"老王"}')).toEqual({
      loggedIn: true,
      name: '老王'
    })
  })

  it('null / "null" / 空串 / 坏 JSON / 非对象 → 一律未登录', () => {
    for (const junk of [null, undefined, 'null', '', '   ', 'oops', '["array"]', '"str"', '123']) {
      expect(normalizeAuthSnapshot(junk as string)).toEqual({ loggedIn: false, name: '' })
    }
  })

  it('字段类型不对( loggedIn 非 boolean / name 非字符串 )按兜底处理', () => {
    expect(normalizeAuthSnapshot('{"loggedIn":"yes","name":123}')).toEqual({
      loggedIn: false,
      name: ''
    })
  })
})

describe('authSnapshot (读 store)', () => {
  beforeEach(() => {
    authState.token = ''
    authState.name = ''
  })

  it('未登录: loggedIn=false, name 为空', () => {
    expect(authSnapshot()).toEqual({ loggedIn: false, name: '' })
  })

  it('已登录: 带昵称', () => {
    authState.token = 'tok'
    authState.name = '小明'
    expect(authSnapshot()).toEqual({ loggedIn: true, name: '小明' })
  })
})

describe('installNativeAuthHook (window.__jcAuth)', () => {
  type AuthWin = {
    __jcAuth?: { user: () => string; login: () => boolean; logout: () => boolean }
  }

  beforeEach(() => {
    mocks.native.value = true
    mocks.logout.mockClear()
    mocks.push.mockClear()
    authState.token = ''
    authState.name = ''
    delete (window as unknown as AuthWin).__jcAuth
  })

  it('原生壳: 挂钩子; user() 返回 JSON 串', () => {
    installNativeAuthHook()
    const hook = (window as unknown as AuthWin).__jcAuth
    expect(hook).toBeTruthy()
    authState.token = 'tok'
    authState.name = '小明'
    expect(JSON.parse(hook!.user())).toEqual({ loggedIn: true, name: '小明' })
  })

  it('浏览器(无桥): 不挂钩子', () => {
    mocks.native.value = false
    installNativeAuthHook()
    expect((window as unknown as AuthWin).__jcAuth).toBeUndefined()
  })

  it('login() → 跳 /login 且带 redirect 回当前页', () => {
    installNativeAuthHook()
    const ret = (window as unknown as AuthWin).__jcAuth!.login()
    expect(ret).toBe(true)
    expect(mocks.push).toHaveBeenCalledWith({
      path: '/login',
      query: { redirect: '/index' }
    })
    expect(mocks.logout).not.toHaveBeenCalled()
  })

  it('logout() → store.logout(), 且不跳路由', () => {
    authState.token = 'tok'
    installNativeAuthHook()
    const ret = (window as unknown as AuthWin).__jcAuth!.logout()
    expect(ret).toBe(true)
    expect(mocks.logout).toHaveBeenCalledTimes(1)
    expect(mocks.push).not.toHaveBeenCalled()
  })
})

describe('openWebLogin / webLogout (直调语义与钩子一致)', () => {
  beforeEach(() => {
    mocks.logout.mockClear()
    mocks.push.mockClear()
  })

  it('openWebLogin 带 redirect; webLogout 调 store.logout() 吞掉异常', async () => {
    openWebLogin()
    expect(mocks.push).toHaveBeenCalledWith({
      path: '/login',
      query: { redirect: '/index' }
    })

    mocks.logout.mockRejectedValueOnce(new Error('network'))
    expect(() => webLogout()).not.toThrow()
    await new Promise((r) => setTimeout(r, 0))
    expect(mocks.logout).toHaveBeenCalledTimes(1)
  })
})
