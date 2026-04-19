package api

import (
	"sync"
	"time"

	"xorm.io/xorm"
)

type Session = xorm.Session

// ==========================================
// Original Games (Instant Games) Types
// ==========================================

// Game session status constants
const (
	OriginalStatusActive   = "active"
	OriginalStatusFinished = "finished"
	OriginalStatusExpired  = "expired"
)

// Available original games
const (
	GameDice      = "dice"
	GameMines     = "mines"
	GameCrash     = "crash"
	GameBubbles   = "bubbles"
	GameBlackjack = "blackjack"
	GameBaccarat  = "baccarat"
	GameCoinflip  = "
