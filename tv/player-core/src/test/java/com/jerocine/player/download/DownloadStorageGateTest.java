package com.jerocine.player.download;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.io.File;
import java.io.IOException;

/** {@link DownloadStorageGate} 纯逻辑单测: 红线判定 / 分区探测退化 / 文案。 */
public class DownloadStorageGateTest {

    @Test
    public void belowFloor_exactlyAtFloor_passes() {
        assertFalse(DownloadStorageGate.belowFloor(DownloadStorageGate.MIN_FREE_BYTES));
    }

    @Test
    public void belowFloor_oneByteUnder_fails() {
        assertTrue(DownloadStorageGate.belowFloor(DownloadStorageGate.MIN_FREE_BYTES - 1));
    }

    @Test
    public void belowFloor_zeroAndNegative_fail() {
        assertTrue(DownloadStorageGate.belowFloor(0L));
        assertTrue(DownloadStorageGate.belowFloor(-1L));
    }

    @Test
    public void belowFloor_gbScale_abovePasses_belowFails() {
        assertFalse(DownloadStorageGate.belowFloor(3L * 1024 * 1024 * 1024));
        assertTrue(DownloadStorageGate.belowFloor(1024L * 1024 * 1024)); // 1GB
    }

    @Test
    public void usableBytes_existingDir_nonNegative() throws IOException {
        File dir = java.nio.file.Files.createTempDirectory("gate").toFile();
        long usable = DownloadStorageGate.usableBytes(dir);
        assertTrue(usable >= 0);
        assertTrue(dir.delete());
    }

    @Test
    public void usableBytes_missingDir_walksUpToExistingParent() throws IOException {
        // cacheRoot 理论上构造时已 mkdirs, 但探测逻辑必须容忍"目录还没建"——
        // File.getUsableSpace() 对不存在的路径返回 0, 会被误判为没空间 ⇒ 向上找父分区。
        File parent = java.nio.file.Files.createTempDirectory("gatep").toFile();
        File missing = new File(parent, "a/b/c");
        long usable = DownloadStorageGate.usableBytes(missing);
        assertTrue(usable > 0); // 拿到的是父分区的真实可用空间, 不是 0
    }

    @Test
    public void usableBytes_nullishInput_passesThrough() {
        // 探测彻底落空(理论上不会发生)必须放行, 不能把下载永久卡死
        assertTrue(DownloadStorageGate.usableBytes(null) == Long.MAX_VALUE);
    }

    @Test
    public void texts_mentionStorageAndAction() {
        String reason = DownloadStorageGate.pauseReason();
        assertTrue(reason.contains("存储空间不足"));
        assertTrue(reason.contains("继续")); // 告诉用户出口在哪
        assertTrue(DownloadStorageGate.toastText().contains("清理"));
    }
}
