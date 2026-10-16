package com.jerocine.tv.data

fun isPlayerHistoryEvent(name: String): Boolean =
    name == "playerProgress" || name == "playerClosed" || name == "playerEpisodeChange"
