package api

import "github.com/gin-gonic/gin"

func ApiOriginalNew(c *gin.Context) {
    // Создание раунда Dice/Mines/Crash/etc
    // Параметры: uid, game, bet
}

func ApiOriginalJoin(c *gin.Context) {
    // Действие в игре (клик в mines, остановка crash, бросок dice)
    // Параметры: gid, uid, action, [доп. данные]
}

func ApiOriginalInfo(c *gin.Context) {
    // Информация о текущем раунде
}

func ApiOriginalRtpGet(c *gin.Context) {
    // RTP настройки для игры
}

func ApiOriginalAlgs(c *gin.Context) {
    // Список алгоритмов Provably Fair
}
