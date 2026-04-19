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
	GameCoinflip  = "coinflip"
	GameJackpot   = "jackpot"
	GameSlot      = "slot"
)

// OriginalScene represents an instant game session
type OriginalScene struct {
	GID       uint64      `json:"gid"`
	UID       uint64      `json:"uid"`
	CID       uint64      `json:"cid"`       // Currency ID
	Game      string      `json:"game"`      // dice, mines, crash, etc.
	Bet       float64     `json:"bet"`
	Status    string      `json:"status"`    // active, finished, expired
	Win       float64     `json:"win"`
	Multiplier float64    `json:"multiplier"`
	Data      interface{} `json:"data"`      // Game-specific data
	ClientSeed string     `json:"client_seed,omitempty"`
	ServerSeed string     `json:"server_seed,omitempty"`
	Nonce     int64       `json:"nonce"`
	CreatedAt int64       `json:"created_at"`
	UpdatedAt int64       `json:"updated_at"`
	ExpiresAt int64       `json:"expires_at"` // Auto-expire after 24h
}

// Game-specific data structures
type DiceData struct {
	Target     float64 `json:"target"`      // 2 to 98
	Roll       float64 `json:"roll"`        // Result 0-100
	IsOver     bool    `json:"is_over"`     // Roll over or under
	Result     string  `json:"result"`      // "win" or "loss"
}

type MinesData struct {
	GridSize   int     `json:"grid_size"`   // Usually 5x5 = 25
	MinesCount int     `json:"mines_count"` // 1-24
	Revealed   []int   `json:"revealed"`    // Revealed cell indices
	Mines      []int   `json:"mines"`       // Mine positions (hidden until finish)
	GemsFound  int     `json:"gems_found"`
	CashedOut  bool    `json:"cashed_out"`
}

type CrashData struct {
	Multiplier float64 `json:"multiplier"`  // Current multiplier
	CrashedAt  float64 `json:"crashed_at"`  // Where it crashed
	CashedOut  bool    `json:"cashed_out"`
	CashoutAt  float64 `json:"cashout_at"`  // Where player cashed out
}

type CoinflipData struct {
	Choice     string  `json:"choice"`      // "heads" or "tails"
	Result     string  `json:"result"`      // "heads" or "tails"
}

// Storage for original game scenes
var OriginalScenes = &sync.Map{}

// Helper functions for scene management
func GetOriginalScene(gid uint64) (*OriginalScene, bool) {
	if val, ok := OriginalScenes.Load(gid); ok {
		return val.(*OriginalScene), true
	}
	return nil, false
}

func SetOriginalScene(scene *OriginalScene) {
	OriginalScenes.Store(scene.GID, scene)
}

func DeleteOriginalScene(gid uint64) {
	OriginalScenes.Delete(gid)
}

// Cleanup expired scenes (run periodically)
func CleanupExpiredOriginalScenes() {
	now := time.Now().Unix()
	OriginalScenes.Range(func(key, value interface{}) bool {
		scene := value.(*OriginalScene)
		if scene.Status == OriginalStatusFinished || 
		   scene.Status == OriginalStatusExpired || 
		   now > scene.ExpiresAt {
			OriginalScenes.Delete(key)
		}
		return true
	})
}

// Request/Response types for API
type OriginalNewRequest struct {
	UID        uint64  `json:"uid" form:"uid" binding:"required"`
	CID        uint64  `json:"cid" form:"cid" binding:"required"`
	Game       string  `json:"game" form:"game" binding:"required"`
	Bet        float64 `json:"bet" form:"bet" binding:"required"`
	ClientSeed string  `json:"client_seed" form:"client_seed"`
}

type OriginalJoinRequest struct {
	GID        uint64  `json:"gid" form:"gid" binding:"required"`
	UID        uint64  `json:"uid" form:"uid" binding:"required"`
	Action     string  `json:"action" form:"action" binding:"required"` // "click", "cashout", "roll", etc.
	CellIndex  int     `json:"cell_index" form:"cell_index"`            // For mines
	Target     float64 `json:"target" form:"target"`                    // For dice (over/under)
	IsOver     bool    `json:"is_over" form:"is_over"`                  // For dice
	Choice     string  `json:"choice" form:"choice"`                    // For coinflip
}

type OriginalRtpRequest struct {
	Game string `json:"game" form:"game" binding:"required"`
}
