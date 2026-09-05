/** 轻量 logger，生产环境 esbuild drop console，仅保留调试时用 */

const PREFIX = '[jerocine]'

export const logger = {
  debug: (...args: unknown[]): void => {
    if (import.meta.env.DEV) {
      // eslint-disable-next-line no-console
      console.debug(PREFIX, ...args)
    }
  },
  info: (...args: unknown[]): void => {
    // eslint-disable-next-line no-console
    console.info(PREFIX, ...args)
  },
  warn: (...args: unknown[]): void => {
    // eslint-disable-next-line no-console
    console.warn(PREFIX, ...args)
  },
  error: (...args: unknown[]): void => {
    // eslint-disable-next-line no-console
    console.error(PREFIX, ...args)
  }
}
