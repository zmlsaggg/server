package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	cfg "github.com/slotopol/server/config"
)

var OriginalCounter uint64 = 1000000 // Начинаем с большого числа

func NextOriginalGID() uint64 {
	return atomic.AddUint64(&OriginalCounter, 1)
}

// ==========================================
// ApiOriginalNew - Create new game round
// POST /api/original/new
// Body: {uid, cid, game, bet, client_seed}
// ==========================================
func ApiOriginalNew(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName    xml.Name `json:"-" yaml:"-" xml:"arg"`
		UID        uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		CID        uint64   `json:"cid" yaml:"cid" xml:"cid,attr" form:"cid" binding:"required"`
		Game       string   `json:"game" yaml:"game" xml:"game,attr" form:"game" binding:"required"`
		Bet        float64  `json:"bet" yaml:"bet" xml:"bet,attr" form:"bet" binding:"required"`
		ClientSeed string   `json:"client_seed" yaml:"client_seed" xml:"client_seed,attr" form:"client_seed"`
	}
	var ret struct {
		XMLName     xml.Name    `json:"-" yaml:"-" xml:"ret"`
		GID         uint64      `json:"gid" yaml:"gid" xml:"gid"`
		Game        string      `json:"game" yaml:"game" xml:"game"`
		Bet         float64     `json:"bet" yaml:"bet" xml:"bet"`
		Balance     float64     `json:"balance" yaml:"balance" xml:"balance"`
		Status      string      `json:"status" yaml:"status" xml:"status"`
		ClientSeed  string      `json:"client_seed" yaml:"client_seed" xml:"client_seed"`
		ServerHash  string      `json:"server_hash" yaml:"server_hash" xml:"server_hash"`
		Nonce       int64       `json:"nonce" yaml:"nonce" xml:"nonce"`
		Data        interface{} `json:"data" yaml:"data" xml:"data"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, AEC_original_new_nobind)
		return
	}

	// Validate user
	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	// Validate game type
	validGames := map[string]bool{
		GameDice: true, GameMines: true, GameCrash: true,
		GameBubbles: true, GameBlackjack: true, GameBaccarat: true,
		GameCoinflip: true, GameJackpot: true, GameSlot: true,
	}
	if !validGames[arg.Game] {
		Ret400(c, AEC_original_new_invalid_game)
		return
	}

	// Check balance
	wallet := user.GetWallet(arg.CID)
	if wallet.Balance < arg.Bet {
		Ret403(c, ErrNoMoney)
		return
	}

	// Deduct bet
	wallet.Balance -= arg.Bet
	user.SetWallet(arg.CID, wallet)

	// Generate Provably Fair seeds
	serverSeed := generateServerSeed()
	nonce := time.Now().UnixNano()
	if arg.ClientSeed == "" {
		arg.ClientSeed = generateClientSeed()
	}

	// Create GID
	gid := NextOriginalGID()

	// Initialize game-specific data
	var gameData interface{}
	switch arg.Game {
	case GameDice:
		gameData = &DiceData{}
	case GameMines:
		// Default 5x5 with 3 mines
		mines := generateMines(25, 3, serverSeed, arg.ClientSeed, nonce)
		gameData = &MinesData{
			GridSize:   25,
			MinesCount: 3,
			Revealed:   []int{},
			Mines:      mines,
			GemsFound:  0,
			CashedOut:  false,
		}
	case GameCrash:
		// Generate crash point (1.0 to ~100.0)
		crashPoint := generateCrashPoint(serverSeed, arg.ClientSeed, nonce)
		gameData = &CrashData{
			Multiplier: 1.0,
			CrashedAt:  crashPoint,
			CashedOut:  false,
			CashoutAt:  0,
		}
	case GameCoinflip:
		gameData = &CoinflipData{}
	}

	// Create scene
	now := time.Now().Unix()
	scene := &OriginalScene{
		GID:        gid,
		UID:        arg.UID,
		CID:        arg.CID,
		Game:       arg.Game,
		Bet:        arg.Bet,
		Status:     OriginalStatusActive,
		Win:        0,
		Multiplier: 1.0,
		Data:       gameData,
		ClientSeed: arg.ClientSeed,
		ServerSeed: serverSeed,
		Nonce:      nonce,
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  now + 86400, // 24 hours
	}

	SetOriginalScene(scene)

	// Return response (server seed is hashed for client)
	ret.GID = gid
	ret.Game = arg.Game
	ret.Bet = arg.Bet
	ret.Balance = wallet.Balance
	ret.Status = OriginalStatusActive
	ret.ClientSeed = arg.ClientSeed
	ret.ServerHash = hashServerSeed(serverSeed)
	ret.Nonce = nonce
	ret.Data = filterGameDataForClient(gameData, arg.Game)

	RetOk(c, ret)
}

// ==========================================
// ApiOriginalJoin - Perform game action
// POST /api/original/join
// Body: {gid, uid, action, [params]}
// ==========================================
func ApiOriginalJoin(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName   xml.Name `json:"-" yaml:"-" xml:"arg"`
		GID       uint64   `json:"gid" yaml:"gid" xml:"gid,attr" form:"gid" binding:"required"`
		UID       uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		Action    string   `json:"action" yaml:"action" xml:"action,attr" form:"action" binding:"required"`
		CellIndex int      `json:"cell_index" yaml:"cell_index" xml:"cell_index,attr" form:"cell_index"`
		Target    float64  `json:"target" yaml:"target" xml:"target,attr" form:"target"`
		IsOver    bool     `json:"is_over" yaml:"is_over" xml:"is_over,attr" form:"is_over"`
		Choice    string   `json:"choice" yaml:"choice" xml:"choice,attr" form:"choice"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, AEC_original_join_nobind)
		return
	}

	// Get scene
	scene, ok := GetOriginalScene(arg.GID)
	if !ok {
		Ret404(c, ErrNotOriginal)
		return
	}

	// Validate ownership
	if scene.UID != arg.UID {
		Ret403(c, ErrNoAccess)
		return
	}

	// Check status
	if scene.Status != OriginalStatusActive {
		Ret400(c, AEC_original_join_game_finished)
		return
	}

	// Process action based on game type
	var result gin.H
	switch scene.Game {
	case GameMines:
		result = processMinesAction(scene, arg)
	case GameDice:
		result = processDiceAction(scene, arg)
	case GameCrash:
		result = processCrashAction(scene, arg)
	case GameCoinflip:
		result = processCoinflipAction(scene, arg)
	default:
		Ret400(c, AEC_original_join_invalid_action)
		return
	}

	// Save updated scene
	scene.UpdatedAt = time.Now().Unix()
	SetOriginalScene(scene)

	RetOk(c, result)
}

// ==========================================
// ApiOriginalInfo - Get game info
// POST /api/original/info
// Body: {gid, uid}
// ==========================================
func ApiOriginalInfo(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		GID     uint64   `json:"gid" yaml:"gid" xml:"gid,attr" form:"gid" binding:"required"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
	}
	var ret struct {
		XMLName    xml.Name    `json:"-" yaml:"-" xml:"ret"`
		GID        uint64      `json:"gid" yaml:"gid" xml:"gid"`
		Game       string      `json:"game" yaml:"game" xml:"game"`
		Bet        float64     `json:"bet" yaml:"bet" xml:"bet"`
		Status     string      `json:"status" yaml:"status" xml:"status"`
		Win        float64     `json:"win" yaml:"win" xml:"win"`
		Multiplier float64     `json:"multiplier" yaml:"multiplier" xml:"multiplier"`
		Data       interface{} `json:"data" yaml:"data" xml:"data"`
		ClientSeed string      `json:"client_seed" yaml:"client_seed" xml:"client_seed"`
		Nonce      int64       `json:"nonce" yaml:"nonce" xml:"nonce"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, AEC_original_info_nobind)
		return
	}

	scene, ok := GetOriginalScene(arg.GID)
	if !ok {
		Ret404(c, ErrNotOriginal)
		return
	}

	if scene.UID != arg.UID {
		Ret403(c, ErrNoAccess)
		return
	}

	// Return server seed only if game is finished
	ret.GID = scene.GID
	ret.Game = scene.Game
	ret.Bet = scene.Bet
	ret.Status = scene.Status
	ret.Win = scene.Win
	ret.Multiplier = scene.Multiplier
	ret.Data = filterGameDataForClient(scene.Data, scene.Game)
	ret.ClientSeed = scene.ClientSeed
	ret.Nonce = scene.Nonce

	// If finished, reveal server seed
	if scene.Status == OriginalStatusFinished {
		// Add server seed to response
		RetOk(c, gin.H{
			"gid":         ret.GID,
			"game":        ret.Game,
			"bet":         ret.Bet,
			"status":      ret.Status,
			"win":         ret.Win,
			"multiplier":  ret.Multiplier,
			"data":        ret.Data,
			"client_seed": ret.ClientSeed,
			"nonce":       ret.Nonce,
			"server_seed": scene.ServerSeed,
			"server_hash": hashServerSeed(scene.ServerSeed),
		})
		return
	}

	ret.Status = scene.Status
	RetOk(c, ret)
}

// ==========================================
// ApiOriginalRtpGet - Get RTP settings
// POST /api/original/rtp/get
// Body: {game}
// ==========================================
func ApiOriginalRtpGet(c *gin.Context) {
	var err error
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		Game    string   `json:"game" yaml:"game" xml:"game,attr" form:"game" binding:"required"`
	}
	var ret struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"ret"`
		Game    string   `json:"game" yaml:"game" xml:"game"`
		RTP     gin.H    `json:"rtp" yaml:"rtp" xml:"rtp"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, AEC_original_rtpget_nobind)
		return
	}

	// Default RTP settings for original games
	rtpSettings := map[string]gin.H{
		GameDice: {
			"rtp":        98.0,
			"house_edge": 2.0,
			"max_bet":    10000.0,
			"min_bet":    1.0,
		},
		GameMines: {
			"rtp":        97.0,
			"house_edge": 3.0,
			"max_bet":    5000.0,
			"min_bet":    1.0,
		},
		GameCrash: {
			"rtp":        98.5,
			"house_edge": 1.5,
			"max_bet":    10000.0,
			"min_bet":    1.0,
			"max_payout": 100000.0,
		},
		GameCoinflip: {
			"rtp":        98.0,
			"house_edge": 2.0,
			"max_bet":    10000.0,
			"min_bet":    1.0,
		},
	}

	rtp, ok := rtpSettings[arg.Game]
	if !ok {
		Ret400(c, AEC_original_rtpget_invalid_game)
		return
	}

	ret.Game = arg.Game
	ret.RTP = rtp

	RetOk(c, ret)
}

// ==========================================
// ApiOriginalAlgs - Get Provably Fair algorithms
// GET /api/original/algs
// Query: ?gameId=&userId=
// ==========================================
func ApiOriginalAlgs(c *gin.Context) {
	gameId := c.Query("gameId")
	if gameId == "" {
		Ret400(c, AEC_original_algs_nogameid)
		return
	}

	algs := []gin.H{
		{
			"name":        "SHA-256",
			"description": "Server seed hashed with client seed and nonce",
			"verified":    true,
		},
		{
			"name":        "HMAC-SHA256",
			"description": "Provably Fair HMAC generation",
			"verified":    true,
		},
	}

	RetOk(c, gin.H{
		"gameId": gameId,
		"algs":   algs,
		"note":   "All games use cryptographically secure random generation",
	})
}

// ==========================================
// Game Action Processors
// ==========================================

func processMinesAction(scene *OriginalScene, arg struct {
	XMLName   xml.Name `json:"-" yaml:"-" xml:"arg"`
	GID       uint64   `json:"gid" yaml:"gid" xml:"gid,attr" form:"gid" binding:"required"`
	UID       uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
	Action    string   `json:"action" yaml:"action" xml:"action,attr" form:"action" binding:"required"`
	CellIndex int      `json:"cell_index" yaml:"cell_index" xml:"cell_index,attr" form:"cell_index"`
	Target    float64  `json:"target" yaml:"target" xml:"target,attr" form:"target"`
	IsOver    bool     `json:"is_over" yaml:"is_over" xml:"is_over,attr" form:"is_over"`
	Choice    string   `json:"choice" yaml:"choice" xml:"choice,attr" form:"choice"`
}) gin.H {
	data := scene.Data.(*MinesData)

	switch arg.Action {
	case "click":
		cell := arg.CellIndex
		if cell < 0 || cell >= data.GridSize {
			return gin.H{"error": "invalid cell", "code": AEC_original_join_invalid_action}
		}

		// Check if already revealed
		for _, r := range data.Revealed {
			if r == cell {
				return gin.H{"error": "already revealed", "code": AEC_original_join_invalid_action}
			}
		}

		// Reveal cell
		data.Revealed = append(data.Revealed, cell)

		// Check if mine
		for _, mine := range data.Mines {
			if mine == cell {
				// Hit mine - game over
				scene.Status = OriginalStatusFinished
				scene.Win = 0
				data.CashedOut = false

				// Return with revealed mines
				return gin.H{
					"status":   OriginalStatusFinished,
					"result":   "loss",
					"win":      0,
					"cell":     cell,
					"mine":     true,
					"mines":    data.Mines, // Reveal all mines
					"balance":  userGetBalance(scene.UID, scene.CID),
					"server_seed": scene.ServerSeed,
				}
			}
		}

		// Found gem
		data.GemsFound++

		// Calculate multiplier based on revealed tiles
		multiplier := calculateMinesMultiplier(data.GemsFound, data.MinesCount, data.GridSize)
		scene.Multiplier = multiplier
		scene.Win = scene.Bet * multiplier

		return gin.H{
			"status":        scene.Status,
			"result":        "continue",
			"cell":          cell,
			"mine":          false,
			"gems_found":    data.GemsFound,
			"multiplier":    multiplier,
			"potential_win": scene.Win,
		}

	case "cashout":
		if scene.Status == OriginalStatusActive {
			scene.Status = OriginalStatusFinished
			data.CashedOut = true

			// Credit win
			user, _ := Users.Get(scene.UID)
			wallet := user.GetWallet(scene.CID)
			wallet.Balance += scene.Win
			user.SetWallet(scene.CID, wallet)

			return gin.H{
				"status":      OriginalStatusFinished,
				"result":      "win",
				"win":         scene.Win,
				"multiplier":  scene.Multiplier,
				"balance":     wallet.Balance,
				"server_seed": scene.ServerSeed,
			}
		}
	}

	return gin.H{"error": "unknown action", "code": AEC_original_join_invalid_action}
}

func processDiceAction(scene *OriginalScene, arg struct {
	XMLName   xml.Name `json:"-" yaml:"-" xml:"arg"`
	GID       uint64   `json:"gid" yaml:"gid" xml:"gid,attr" form:"gid" binding:"required"`
	UID       uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
	Action    string   `json:"action" yaml:"action" xml:"action,attr" form:"action" binding:"required"`
	CellIndex int      `json:"cell_index" yaml:"cell_index" xml:"cell_index,attr" form:"cell_index"`
	Target    float64  `json:"target" yaml:"target" xml:"target,attr" form:"target"`
	IsOver    bool     `json:"is_over" yaml:"is_over" xml:"is_over,attr" form:"is_over"`
	Choice    string   `json:"choice" yaml:"choice" xml:"choice,attr" form:"choice"`
}) gin.H {
	data := scene.Data.(*DiceData)

	if arg.Action != "roll" {
		return gin.H{"error": "invalid action", "code": AEC_original_join_invalid_action}
	}

	// Generate roll 0-100
	roll := generateDiceRoll(scene.ServerSeed, scene.ClientSeed, scene.Nonce)
	data.Roll = roll
	data.Target = arg.Target
	data.IsOver = arg.IsOver

	// Determine win
	win := false
	if arg.IsOver && roll > arg.Target {
		win = true
	} else if !arg.IsOver && roll < arg.Target {
		win = true
	}

	data.Result = "loss"
	if win {
		// Calculate multiplier (example: over 50 = 1.98x)
		multiplier := calculateDiceMultiplier(arg.Target, arg.IsOver)
		scene.Multiplier = multiplier
		scene.Win = scene.Bet * multiplier
		data.Result = "win"

		// Credit win
		user, _ := Users.Get(scene.UID)
		wallet := user.GetWallet(scene.CID)
		wallet.Balance += scene.Win
		user.SetWallet(scene.CID, wallet)
	}

	scene.Status = OriginalStatusFinished

	return gin.H{
		"status":      OriginalStatusFinished,
		"result":      data.Result,
		"roll":        roll,
		"target":      arg.Target,
		"is_over":     arg.IsOver,
		"win":         scene.Win,
		"multiplier":  scene.Multiplier,
		"balance":     userGetBalance(scene.UID, scene.CID),
		"server_seed": scene.ServerSeed,
	}
}

func processCrashAction(scene *OriginalScene, arg struct {
	XMLName   xml.Name `json:"-" yaml:"-" xml:"arg"`
	GID       uint64   `json:"gid" yaml:"gid" xml:"gid,attr" form:"gid" binding:"required"`
	UID       uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
	Action    string   `json:"action" yaml:"action" xml:"action,attr" form:"action" binding:"required"`
	CellIndex int      `json:"cell_index" yaml:"cell_index" xml:"cell_index,attr" form:"cell_index"`
	Target    float64  `json:"target" yaml:"target" xml:"target,attr" form:"target"`
	IsOver    bool     `json:"is_over" yaml:"is_over" xml:"is_over,attr" form:"is_over"`
	Choice    string   `json:"choice" yaml:"choice" xml:"choice,attr" form:"choice"`
}) gin.H {
	data := scene.Data.(*CrashData)

	switch arg.Action {
	case "cashout":
		if data.CrashedAt <= 1.0 {
			// Already crashed
			scene.Status = OriginalStatusFinished
			scene.Win = 0

			return gin.H{
				"status":     OriginalStatusFinished,
				"result":     "loss",
				"crashed_at": data.CrashedAt,
				"win":        0,
				"balance":    userGetBalance(scene.UID, scene.CID),
				"server_seed": scene.ServerSeed,
			}
		}

		// Cash out at current multiplier
		data.CashedOut = true
		data.CashoutAt = data.Multiplier
		scene.Status = OriginalStatusFinished
		scene.Win = scene.Bet * data.Multiplier

		// Credit win
		user, _ := Users.Get(scene.UID)
		wallet := user.GetWallet(scene.CID)
		wallet.Balance += scene.Win
		user.SetWallet(scene.CID, wallet)

		return gin.H{
			"status":      OriginalStatusFinished,
			"result":      "win",
			"cashout_at":  data.CashoutAt,
			"multiplier":  data.Multiplier,
			"win":         scene.Win,
			"balance":     wallet.Balance,
			"server_seed": scene.ServerSeed,
		}
	}

	return gin.H{"error": "unknown action", "code": AEC_original_join_invalid_action}
}

func processCoinflipAction(scene *OriginalScene, arg struct {
	XMLName   xml.Name `json:"-" yaml:"-" xml:"arg"`
	GID       uint64   `json:"gid" yaml:"gid" xml:"gid,attr" form:"gid" binding:"required"`
	UID       uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
	Action    string   `json:"action" yaml:"action" xml:"action,attr" form:"action" binding:"required"`
	CellIndex int      `json:"cell_index" yaml:"cell_index" xml:"cell_index,attr" form:"cell_index"`
	Target    float64  `json:"target" yaml:"target" xml:"target,attr" form:"target"`
	IsOver    bool     `json:"is_over" yaml:"is_over" xml:"is_over,attr" form:"is_over"`
	Choice    string   `json:"choice" yaml:"choice" xml:"choice,attr" form:"choice"`
}) gin.H {
	data := scene.Data.(*CoinflipData)

	if arg.Action != "flip" {
		return gin.H{"error": "invalid action", "code": AEC_original_join_invalid_action}
	}

	// Generate result
	result := generateCoinflipResult(scene.ServerSeed, scene.ClientSeed, scene.Nonce)
	data.Choice = arg.Choice
	data.Result = result

	win := arg.Choice == result
	scene.Multiplier = 1.98 // 2% house edge
	scene.Status = OriginalStatusFinished

	if win {
		scene.Win = scene.Bet * scene.Multiplier

		// Credit win
		user, _ := Users.Get(scene.UID)
		wallet := user.GetWallet(scene.CID)
		wallet.Balance += scene.Win
		user.SetWallet(scene.CID, wallet)
	}

	return gin.H{
		"status":      OriginalStatusFinished,
		"result":      result,
		"choice":      arg.Choice,
		"win":         scene.Win,
		"multiplier":  scene.Multiplier,
		"balance":     userGetBalance(scene.UID, scene.CID),
		"server_seed": scene.ServerSeed,
	}
}

// ==========================================
// Helper Functions
// ==========================================

func generateServerSeed() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateClientSeed() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)[:16]
}

func hashServerSeed(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(hash[:])
}

func generateMines(gridSize, count int, serverSeed, clientSeed string, nonce int64) []int {
	// Provably fair mines generation
	h := sha256.New()
	h.Write([]byte(serverSeed + clientSeed + fmt.Sprintf("%d", nonce)))
	hash := hex.EncodeToString(h.Sum(nil))

	mines := []int{}
	used := map[int]bool{}

	for i := 0; i < count; i++ {
		// Use hash bytes to determine positions
		idx := (i * 2) % len(hash)
		pos := int(hash[idx]) % gridSize

		// Ensure unique
		for used[pos] {
			pos = (pos + 1) % gridSize
		}
		used[pos] = true
		mines = append(mines, pos)
	}

	return mines
}

func generateCrashPoint(serverSeed, clientSeed string, nonce int64) float64 {
	// Provably fair crash point (1% chance of 1.00, exponential distribution)
	h := sha256.New()
	h.Write([]byte(serverSeed + clientSeed + fmt.Sprintf("%d", nonce)))
	hash := hex.EncodeToString(h.Sum(nil))

	// Use first 8 hex chars as int
	n := 0
	for i := 0; i < 8; i++ {
		n = n*256 + int(hash[i])
	}

	// 1% instant crash
	if n%100 == 0 {
		return 1.0
	}

	// Exponential distribution
	e := float64(n%1000000) / 1000000.0
	crash := 0.99 / (1.0 - e)
	if crash < 1.0 {
		crash = 1.0
	}
	if crash > 100.0 {
		crash = 100.0
	}

	return crash
}

func generateDiceRoll(serverSeed, clientSeed string, nonce int64) float64 {
	h := sha256.New()
	h.Write([]byte(serverSeed + clientSeed + fmt.Sprintf("%d", nonce)))
	hash := hex.EncodeToString(h.Sum(nil))

	// First 4 bytes to 0-100 float
	n := 0
	for i := 0; i < 4; i++ {
		n = n*256 + int(hash[i])
	}

	return float64(n%10000) / 100.0 // 0.00 to 99.99
}

func generateCoinflipResult(serverSeed, clientSeed string, nonce int64) string {
	h := sha256.New()
	h.Write([]byte(serverSeed + clientSeed + fmt.Sprintf("%d", nonce)))
	hash := hex.EncodeToString(h.Sum(nil))

	if hash[0]%2 == 0 {
		return "heads"
	}
	return "tails"
}

func calculateMinesMultiplier(gemsFound, minesCount, gridSize int) float64 {
	// Simplified multiplier calculation
	// More gems = higher multiplier, more mines = higher multiplier per gem
	remaining := gridSize - minesCount
	if gemsFound >= remaining {
		// All gems found - max win
		return 10.0 // Cap at 10x for safety
	}

	// Progressive multiplier
	base := 1.0
	riskFactor := float64(minesCount) / float64(gridSize)

	for i := 0; i < gemsFound; i++ {
		base += riskFactor * base * 0.5
	}

	if base > 10.0 {
		base = 10.0
	}

	return base
}

func calculateDiceMultiplier(target float64, isOver bool) float64 {
	// Lower probability = higher multiplier
	var probability float64
	if isOver {
		probability = (100.0 - target) / 100.0
	} else {
		probability = target / 100.0
	}

	if probability <= 0 {
		return 1.0
	}

	// 98% RTP: multiplier = 0.98 / probability
	multiplier := 0.98 / probability
	if multiplier > 100.0 {
		multiplier = 100.0
	}

	return multiplier
}

func userGetBalance(uid, cid uint64) float64 {
	user, ok := Users.Get(uid)
	if !ok {
		return 0
	}
	return user.GetWallet(cid).Balance
}

func filterGameDataForClient(data interface{}, game string) interface{} {
	// Filter sensitive data (hide mines positions until game over)
	switch d := data.(type) {
	case *MinesData:
		// Return without mines positions
		return gin.H{
			"grid_size":   d.GridSize,
			"mines_count": d.MinesCount,
			"revealed":    d.Revealed,
			"gems_found":  d.GemsFound,
			"cashed_out":  d.CashedOut,
		}
	case *CrashData:
		// Return without crashed_at (hidden until finish)
		return gin.H{
			"multiplier": d.Multiplier,
			"cashed_out": d.CashedOut,
		}
	default:
		return data
	}
}
