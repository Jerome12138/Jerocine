package com.jerocine.player.download;

import java.util.List;

/**
 * 下载任务存储接口 — 纯逻辑层可测(内存实现/单测), SQLite 实现见 {@link DownloadRepository}.
 *
 * <p>为什么独立成接口: {@link DownloadTask} 的状态机/幂等/查询是核心逻辑,
 * 单测跑在 JVM 上不加载 Android 框架, 因此存储语义(幂等插入/按片查询/删除)抽接口,
 * 让纯逻辑测试直接用一个内存实现覆盖, SQLite 实现只做薄薄一层映射。
 */
public interface DownloadTaskStore {

    /**
     * 幂等插入: 同 id 已存在则忽略并返回 false, 否则插入返回 true.
     * (DB 层 UNIQUE(id) 兜底, 并发下也不会出现重复任务)
     */
    boolean insertIgnore(DownloadTask t);

    /** 更新已存在任务(按 id); 不存在返回 false. */
    boolean update(DownloadTask t);

    /** 按幂等键取任务; 无返回 null. */
    DownloadTask get(String id);

    /** 某影片的全部任务(按集号升序). */
    List<DownloadTask> listByFilm(String filmId);

    /** 全部任务(按 createdAt 升序). */
    List<DownloadTask> listAll();

    /** 删除任务; 存在并删除返回 true. */
    boolean delete(String id);
}
