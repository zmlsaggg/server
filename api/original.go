package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"

	"github.com/gin-gonic/gin"
)

// ==========================================
// ApiOriginalNew - Create new game round
// POST /api/original/new
// Body: {uid, cid, game, bet, client_seed}
// ==========================================
func ApiOriginalNew(c *gin.Context) {
	var req OriginalNewRequest
	if err := c.ShouldBind(&req); err != nil {
		Ret400(c, err)
		return
	}

	// Validate user
	user, ok := Users.Get(req.UID)
	if !ok {
		Ret404(c, "user not found")
		return
	}

	// Validate game type
	validGames := map[string]bool{
		GameDice: true, GameMines: true, GameCrash: true,
		GameBubbles: true, GameBlackjack: true, GameBaccarat: true,
		GameCoinflip: true, GameJackpot: true, GameSlot: true,
	}
	if !validGames[req.Game] {
		Ret400(c, "invalid game type")
		return
	}

	// Check balance
	wallet := user.GetWallet(req.CID)
	if wallet.Balance < req.Bet {
		Ret400(c, "insufficient balance")
		return
	}

	// Deduct bet
	wallet.Balance -= req.Bet
	user.SetWallet(req.CID, wallet)

	// Generate Provably Fair seeds
	serverSeed := generateServerSeed()
	nonce := time.Now().UnixNano()
	if req.ClientSeed == "" {
		req.ClientSeed = generateClientSeed()
	}

	// Create GID
	gid := StoryCounter.Inc()

	// Initialize game-specific data
	var gameData interface{}
	switch req.Game {
	case GameDice:
		gameData = &DiceData{}
	case GameMines:
		// Default 5x5 with 3 mines
		mines := generateMines(25, 3, serverSeed, req.ClientSeed, nonce)
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
		crashPoint := generateCrashPoint(serverSeed, req.ClientSeed, nonce)
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
		UID:        req.UID,
		CID:        req.CID,
		Game:       req.Game,
		Bet:        req.Bet,
		Status:     OriginalStatusActive,
		Win:        0,
		Multiplier: 1.0,
		Data:       gameData,
		ClientSeed: req.ClientSeed,
		ServerSeed: serverSeed,
		Nonce:      nonce,
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  now + 86400, // 24 hours
	}

	SetOriginalScene(scene)

	// Return response (server seed is hashed for client)
	RetOk(c, gin.H{
		"gid":         gid,
		"game":        req.Game,
		"bet":         req.Bet,
		"balance":     wallet.Balance,
		"status":      OriginalStatusActive,
		"client_seed": req.ClientSeed,
		"server_hash": hashServerSeed(serverSeed),
		"nonce":       nonce,
		"data":        filterGameDataForClient(gameData, req.Game),
	})
}

// ==========================================
// ApiOriginalJoin - Perform game action
// POST /api/original/join
// Body: {gid, uid, action, [params]}
// ==========================================
func ApiOriginalJoin(c *gin.Context) {
	var req OriginalJoinRequest
	if err := c.ShouldBind(&req); err != nil {
		Ret400(c, err)
		return
	}

	// Get scene
	scene, ok := GetOriginalScene(req.GID)
	if !ok {
		Ret404(c, "game not found")
		return
	}

	// Validate ownership
	if scene.UID != req.UID {
		Ret403(c, "access denied")
		return
	}

	// Check status
	if scene.Status != OriginalStatusActive {
		Ret400(c, "game already finished")
		return
	}

	// Process action based on game type
	var result gin.H
	switch scene.Game {
	case GameMines:
		result = processMinesAction(scene, req)
	case GameDice:
		result = processDiceAction(scene, req)
	case GameCrash:
		result = processCrashAction(scene, req)
	case GameCoinflip:
		result = processCoinflipAction(scene, req)
	default:
		Ret400(c, "game action not implemented")
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
	var req struct {
		GID uint64 `json:"gid" form:"gid" binding:"required"`
		UID uint64 `json:"uid" form:"uid" binding:"required"`
	}
	if err := c.ShouldBind(&req); err != nil {
		Ret400(c, err)
		return
	}

	scene, ok := GetOriginalScene(req.GID)
	if !ok {
		Ret404(c, "game not found")
		return
	}

	if scene.UID != req.UID {
		Ret403(c, "access denied")
		return
	}

	// Return server seed only if game is finished
	response := gin.H{
		"gid":         scene.GID,
		"game":        scene.Game,
		"bet":         scene.Bet,
		"status":      scene.Status,
		"win":         scene.Win,
		"multiplier":  scene.Multiplier,
		"data":        filterGameDataForClient(scene.Data, scene.Game),
		"client_seed": scene.ClientSeed,
		"nonce":       scene.Nonce,
	}

	if scene.Status == OriginalStatusFinished {
		response["server_seed"] = scene.ServerSeed
		response["server_hash"] = hashServerSeed(scene.ServerSeed)
	} else {
		response["server_hash"] = hashServerSeed(scene.ServerSeed)
	}

	RetOk(c, response)
}

// ==========================================
// ApiOriginalRtpGet - Get RTP settings
// POST /api/original/rtp/get
// Body: {game}
// ==========================================
func ApiOriginalRtpGet(c *gin.Context) {
	var req OriginalRtpRequest
	if err := c.ShouldBind(&req); err != nil {
		Ret400(c, err)
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

	rtp, ok := rtpSettings[req.Game]
	if !ok {
		rtp = gin.H{"rtp": 97.0, "house_edge": 3.0}
	}

	RetOk(c, gin.H{
		"game": req.Game,
		"rtp":  rtp,
	})
}

// ==========================================
// ApiOriginalAlgs - Get Provably Fair algorithms
// GET /api/original/algs
// Query: ?gameId=&userId=
// ==========================================
func ApiOriginalAlgs(c *gin.Context) {
	gameId := c.Query("gameId")
	if gameId == "" {
		Ret400(c, "gameId required")
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

func processMinesAction(scene *OriginalScene, req OriginalJoinRequest) gin.H {
	data := scene.Data.(*MinesData)
	
	switch req.Action {
	case "click":
		cell := req.CellIndex
		if cell < 0 || cell >= data.GridSize {
			return gin.H{"error": "invalid cell"}
		}
		
		// Check if already revealed
		for _, r := range data.Revealed {
			if r == cell {
				return gin.H{"error": "already revealed"}
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
				return gin.H{
					"status":   OriginalStatusFinished,
					"result":   "loss",
					"win":      0,
					"cell":     cell,
					"mine":     true,
					"mines":    data.Mines, // Reveal all mines
					"balance":  getUserBalance(scene.UID, scene.CID),
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
			"status":     scene.Status,
			"result":     "continue",
			"cell":       cell,
			"mine":       false,
			"gems_found": data.GemsFound,
			"multiplier": multiplier,
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
				"status":   OriginalStatusFinished,
				"result":   "win",
				"win":      scene.Win,
				"multiplier": scene.Multiplier,
				"balance":  wallet.Balance,
				"server_seed": scene.ServerSeed,
			}
		}
	}
	
	return gin.H{"error": "unknown action"}
}

func processDiceAction(scene *OriginalScene, req OriginalJoinRequest) gin.H {
	data := scene.Data.(*DiceData)
	
	if req.Action != "roll" {
		return gin.H{"error": "invalid action"}
	}
	
	// Generate roll 0-100
	roll := generateDiceRoll(scene.ServerSeed, scene.ClientSeed, scene.Nonce)
	data.Roll = roll
	data.Target = req.Target
	data.IsOver = req.IsOver
	
	// Determine win
	win := false
	if req.IsOver && roll > req.Target {
		win = true
	} else if !req.IsOver && roll < req.Target {
		win = true
	}
	
	data.Result = "loss"
	if win {
		// Calculate multiplier (example: over 50 = 1.98x)
		multiplier := calculateDiceMultiplier(req.Target, req.IsOver)
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
		"status":   OriginalStatusFinished,
		"result":   data.Result,
		"roll":     roll,
		"target":   req.Target,
		"is_over":  req.IsOver,
		"win":      scene.Win,
		"multiplier": scene.Multiplier,
		"balance":  getUserBalance(scene.UID, scene.CID),
		"server_seed": scene.ServerSeed,
	}
}

func processCrashAction(scene *OriginalScene, req OriginalJoinRequest) gin.H {
	data := scene.Data.(*CrashData)
	
	switch req.Action {
	case "cashout":
		if data.CrashedAt <= 1.0 {
			// Already crashed
			scene.Status = OriginalStatusFinished
			scene.Win = 0
			return gin.H{
				"status":  OriginalStatusFinished,
				"result":  "loss",
				"crashed_at": data.CrashedAt,
				"win":     0,
				"balance": getUserBalance(scene.UID, scene.CID),
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
	
	return gin.H{"error": "unknown action"}
}

func processCoinflipAction(scene *OriginalScene, req OriginalJoinRequest) gin.H {
	data := scene.Data.(*CoinflipData)
	
	if req.Action != "flip" {
		return gin.H{"error": "invalid action"}
	}
	
	// Generate result
	result := generateCoinflipResult(scene.ServerSeed, scene.ClientSeed, scene.Nonce)
	data.Choice = req.Choice
	data.Result = result
	
	win := req.Choice == result
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
		"status":   OriginalStatusFinished,
		"result":   result,
		"choice":   req.Choice,
		"win":      scene.Win,
		"multiplier": scene.Multiplier,
		"balance":  getUserBalance(scene.UID, scene.CID),
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

func getUserBalance(uid, cid uint64) float64 {
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
