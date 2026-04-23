package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	cfg "github.com/slotopol/server/config"
	"github.com/slotopol/server/game"
	"github.com/slotopol/server/game/slot"
	"github.com/slotopol/server/util"
)

type gameOut struct {
	ID       int    `json:"id"`
	GameID   string `json:"gameId"`
	Title    string `json:"title"`
	Provider string `json:"provider"`
	Alias    string `json:"alias"`
	Image    string `json:"image"`
	Banner   string `json:"banner"`
}

func ApiGameAlgs(c *gin.Context) {
	RetOk(c, game.AlgList)
}

func ApiGameList(c *gin.Context) {
	if len(game.InfoMap) == 0 {
		for _, b := range game.LoadMap {
			game.MustReadChain(bytes.NewReader(b))
		}
	}

	gamelist := make([]gameOut, 0, 256)
	index := 0

	for _, gi := range game.InfoMap {
		id := gi.ID()

		gamelist = append(gamelist, gameOut{
			ID:       index,
			GameID:   id,
			Title:    gi.Name,
			Provider: gi.Prov,
			Alias:    id,
			Image:    "https://placehold.co/400x300/1a1a2e/8b5cf6?text=" + strings.ReplaceAll(gi.Name, " ", "+"),
			Banner:   "https://placehold.co/800x400/1a1a2e/8b5cf6?text=" + strings.ReplaceAll(gi.Name, " ", "+"),
		})
		index++
	}

	RetOk(c, gin.H{
		"list":   gamelist,
		"total":  len(gamelist),
		"algnum": len(game.AlgList),
		"prvnum": 1,
	})
}

var (
	SpinBuf util.SqlBuf[Spinlog]
	MultBuf util.SqlBuf[Multlog]
	BankBat = map[uint64]*SqlBank{}
	JoinBuf = SqlStory{}
)

func InitGrid(g game.Gamble) {
	g.Spin(cfg.DefMRTP)
}

func ApiGameNew(c *gin.Context) {
	var err error
	var ok bool

	var arg struct {
		CID   uint64 `json:"cid"`
		UID   uint64 `json:"uid"`
		Alias string `json:"alias"`
	}

	// Try JSON body first
	if err = c.ShouldBindJSON(&arg); err != nil {
		// Try form data
		arg.CID, _ = strconv.ParseUint(c.PostForm("cid"), 10, 64)
		arg.UID, _ = strconv.ParseUint(c.PostForm("uid"), 10, 64)
		arg.Alias = c.PostForm("alias")
		if arg.Alias == "" {
			arg.Alias = c.Query("alias")
		}
	}

	// Set defaults
	if arg.CID == 0 {
		arg.CID = 1
	}
	if arg.UID == 0 {
		Ret400(c, fmt.Errorf("uid is required"))
		return
	}
	if arg.Alias == "" {
		Ret400(c, fmt.Errorf("alias is required"))
		return
	}

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		// Fallback: try to load user from database
		session := cfg.XormStorage.NewSession()
		defer session.Close()

		user = &User{UID: arg.UID}
		if has, err := session.Get(user); err != nil || !has {
			session.Close()
			// Auto-create user with default settings
			user = &User{
				UID:    arg.UID,
				Email:  fmt.Sprintf("user_%d@auto.local", arg.UID),
				Secret: fmt.Sprintf("auto_%d_%d", arg.UID, time.Now().Unix()),
				Name:   fmt.Sprintf("Player_%d", arg.UID),
			}
			user.Init()

			// Save to database
			session2 := cfg.XormStorage.NewSession()
			if _, err := session2.Insert(user); err != nil {
				session2.Close()
				Ret500(c, err)
				return
			}
			session2.Close()

			Users.Set(user.UID, user)

			// Create default props for CID=1
			props := &Props{
				CID:    1,
				UID:    user.UID,
				Wallet: 1000, // Starting balance
				Access: ALmember,
				MRTP:   0,
			}
			if err := user.InsertPropsDB(props); err != nil {
				Ret500(c, err)
				return
			}
		} else {
			user.Init()
			Users.Set(user.UID, user) // Cache for future requests
		}
	}

	alias := util.ToID(arg.Alias)
	maker, has := game.GameFactory[alias]
	if !has {
		Ret400(c, "alias not found")
		return
	}

	anygame := maker()
	gid := StoryCounter.Inc()

	scene := &Scene{
		Story: Story{
			GID:   gid,
			CID:   arg.CID,
			UID:   arg.UID,
			Alias: alias,
		},
		Game: anygame,
	}

	InitGrid(anygame)
	Scenes.Set(scene.GID, scene)

	RetOk(c, gin.H{
		"gid": gid,
		"game": gin.H{
			"alias":  alias,
			"image":  "/assets/slots/" + alias + "/icon.png",
			"banner": "/assets/slots/" + alias + "/banner.jpg",
		},
		"wallet": user.GetWallet(arg.CID),
	})
}

func ApiGameJoin(c *gin.Context) {
	var err error
	var ok bool

	var arg struct {
		CID uint64 `json:"cid" form:"cid"`
		UID uint64 `json:"uid" form:"uid"`
		GID uint64 `json:"gid" form:"gid"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var scene *Scene
	if scene, err = GetScene(arg.GID); err != nil {
		Ret404(c, err)
		return
	}

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, "user not found")
		return
	}

	RetOk(c, gin.H{
		"gid": scene.GID,
		"game": gin.H{
			"alias":  scene.Alias,
			"image":  "/assets/slots/" + scene.Alias + "/icon.png",
			"banner": "/assets/slots/" + scene.Alias + "/banner.jpg",
		},
		"wallet": user.GetWallet(arg.CID),
	})
}

func ApiGameInfo(c *gin.Context) {
	alias := strings.ToLower(c.Param("alias"))
	alias = strings.ReplaceAll(alias, "%2f", "-")
	alias = strings.ReplaceAll(alias, "/", "-")

	if alias != "" && alias != "info" {
		if gi, has := game.InfoMap[alias]; has {
			RetOk(c, gin.H{
				"gameId":   gi.ID(),
				"title":    gi.Name,
				"provider": gi.Prov,
				"alias":    gi.ID(),
				"image":    "/assets/slots/" + gi.ID() + "/icon.png",
				"banner":   "/assets/slots/" + gi.ID() + "/banner.jpg",
			})
			return
		}
	}

	Ret400(c, "invalid alias")
}

func ApiGameLaunch(c *gin.Context) {
	var arg struct {
		CID   uint64 `json:"cid" form:"cid"`
		UID   uint64 `json:"uid" form:"uid"`
		Alias string `json:"alias" form:"alias"`
	}

	if err := c.ShouldBind(&arg); err != nil {
		arg.CID, _ = strconv.ParseUint(c.Query("cid"), 10, 64)
		arg.UID, _ = strconv.ParseUint(c.Query("uid"), 10, 64)
		arg.Alias = c.Query("alias")
	}

	if arg.CID == 0 {
		arg.CID = 1
	}
	if arg.UID == 0 {
		c.String(400, "uid is required")
		return
	}
	if arg.Alias == "" {
		c.String(400, "alias is required")
		return
	}

	alias := util.ToID(arg.Alias)
	maker, has := game.GameFactory[alias]
	if !has {
		c.String(404, "alias not found")
		return
	}

	// Auto-create club if not exists
	if _, ok := Clubs.Get(arg.CID); !ok {
		cd := ClubData{CID: arg.CID}
		club := MakeClub(cd)
		Clubs.Set(arg.CID, club)
	}

	// Auto-create user if not exists
	if _, ok := Users.Get(arg.UID); !ok {
		user := &User{
			UID:  arg.UID,
			Name: fmt.Sprintf("Player%d", arg.UID),
		}
		user.Init()
		Users.Set(user.UID, user)
		// Create default props
		props := &Props{
			CID:    arg.CID,
			Wallet: 100000, // Starting balance
		}
		user.props.Set(arg.CID, props)
	}

	anygame := maker()
	gid := StoryCounter.Inc()

	scene := &Scene{
		Story: Story{
			GID:   gid,
			CID:   arg.CID,
			UID:   arg.UID,
			Alias: alias,
		},
		Game: anygame,
	}

	InitGrid(anygame)
	Scenes.Set(scene.GID, scene)

	// Get real grid from game
	var gridJSON = "null"
	if grider, ok := anygame.(slot.Grider); ok {
		sx, sy := grider.Dim()
		grid := make([][]int, sy)
		for y := slot.Pos(1); y <= sy; y++ {
			row := make([]int, sx)
			for x := slot.Pos(1); x <= sx; x++ {
				row[x-1] = int(grider.At(x, y))
			}
			grid[y-1] = row
		}
		if b, err := json.Marshal(grid); err == nil {
			gridJSON = string(b)
		}
	}

	// Return HTML page with direct game UI (no nested iframe)
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { 
			background: #1a0a2e; 
			color: white;
			font-family: Arial, sans-serif;
			display: flex;
			flex-direction: column;
			min-height: 100vh;
		}
		.game-header {
			background: rgba(0,0,0,0.5);
			padding: 15px 20px;
			display: flex;
			justify-content: space-between;
			align-items: center;
		}
		.game-container {
			flex: 1;
			display: flex;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			padding: 20px;
		}
		.reels {
			display: grid;
			grid-template-columns: repeat(5, 80px);
			grid-template-rows: repeat(3, 80px);
			gap: 5px;
			margin-bottom: 30px;
		}
		.reel {
			background: linear-gradient(135deg, #2d1b4e 0%%, #1a0a2e 100%%);
			border: 2px solid #8b5cf6;
			border-radius: 8px;
			display: flex;
			align-items: center;
			justify-content: center;
			font-size: 36px;
			transition: all 0.3s;
			min-height: 80px;
		}
		.reel.spinning {
			animation: pulse 0.5s infinite;
		}
		@keyframes pulse {
			0%% { border-color: #8b5cf6; }
			50%% { border-color: #fbbf24; }
			100%% { border-color: #8b5cf6; }
		}
		.controls {
			display: flex;
			gap: 20px;
			align-items: center;
			flex-wrap: wrap;
			justify-content: center;
			padding: 20px;
			background: rgba(0,0,0,0.3);
			border-radius: 12px;
			margin-top: 20px;
		}
		button {
			background: #8b5cf6;
			color: white;
			border: none;
			padding: 20px 50px;
			font-size: 20px;
			border-radius: 10px;
			cursor: pointer;
			transition: all 0.3s;
			position: relative;
			z-index: 100;
		}
		button:hover { background: #7c3aed; transform: scale(1.05); }
		button:active { transform: scale(0.95); }
		button:disabled { background: #4b5563; cursor: not-allowed; transform: none; }
		button.spin { 
			background: linear-gradient(135deg, #ef4444 0%%, #dc2626 100%%); 
			font-weight: bold;
			box-shadow: 0 4px 15px rgba(239, 68, 68, 0.4);
		}
		button.spin:hover { 
			background: linear-gradient(135deg, #dc2626 0%%, #b91c1c 100%%);
			box-shadow: 0 6px 20px rgba(239, 68, 68, 0.6);
		}
		.info {
			background: rgba(0,0,0,0.3);
			padding: 10px 20px;
			border-radius: 8px;
			text-align: center;
		}
		.win-message {
			color: #fbbf24;
			font-size: 24px;
			font-weight: bold;
			margin-bottom: 20px;
			min-height: 30px;
		}
	</style>
</head>
<body>
	<div class="game-header">
		<span>Game: %s</span>
		<span>GID: %d | UID: %d</span>
	</div>
	<div class="game-container">
		<div class="win-message" id="winMsg"></div>
		<div class="reels" id="reels"></div>
		<div class="controls">
			<div class="info">
				<div>Bet: <span id="bet">10</span></div>
				<div>Win: <span id="win">0</span></div>
			</div>
			<button type="button" class="spin" id="spinBtn" onclick="spin()">🎰 SPIN 🎰</button>
		</div>
	</div>
	<script>
		const gid = %d;
		const uid = %d;
		const cid = %d;
		const alias = "%s";
		const API_BASE = window.location.origin + "/api";
		
		const symbols = ["7", "🍒", "🍋", "🍊", "🍇", "💎", "⭐", "🔔", "💰", "🎰", "🍀", "🌟", "💎", "👑", "🏆"];
		const initialGrid = %s; // Real grid from game
		const rows = initialGrid ? initialGrid.length : 3;
		const cols = initialGrid && initialGrid[0] ? initialGrid[0].length : 5;
		
		function getReelSymbol(index) {
			return document.getElementById('r' + index);
		}
		
		function renderGrid(grid) {
			const reels = document.getElementById('reels');
			reels.innerHTML = '';
			reels.style.gridTemplateColumns = 'repeat(' + grid[0].length + ', 80px)';
			reels.style.gridTemplateRows = 'repeat(' + grid.length + ', 80px)';
			for(let row=0; row<grid.length; row++) {
				for(let col=0; col<grid[row].length; col++) {
					const idx = row * grid[0].length + col;
					const val = grid[row][col];
					const div = document.createElement('div');
					div.className = 'reel';
					div.id = 'r' + idx;
					div.textContent = symbols[val %% symbols.length];
					reels.appendChild(div);
				}
			}
		}
		
		async function spin() {
			console.log('SPIN clicked');
			const btn = document.getElementById('spinBtn');
			const winMsg = document.getElementById('winMsg');
			if(!btn || btn.disabled) return;
			btn.disabled = true;
			winMsg.textContent = '';
			btn.textContent = 'Spinning...';
			
			// Animation - spin all reels
			const totalReels = rows * cols;
			for(let i=0; i<totalReels; i++) {
				const el = getReelSymbol(i);
				if(el) {
					el.classList.add('spinning');
					el.textContent = symbols[Math.floor(Math.random() * symbols.length)];
				}
			}
			
			try {
				const res = await fetch(API_BASE + '/slots/spin', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ gid, uid, cid, bet: 10 })
				});
				const data = await res.json();
				
				if(data.grid) {
					// Update reels with result
					for(let row=0; row<data.grid.length; row++) {
						for(let col=0; col<data.grid[row].length; col++) {
							const idx = row*data.grid[0].length + col;
							const val = data.grid[row][col];
							const el = getReelSymbol(idx);
							if(el) {
								el.textContent = symbols[val %% symbols.length];
								el.classList.remove('spinning');
							}
						}
					}
					
					if(data.gain > 0) {
						winMsg.textContent = 'WIN: ' + data.gain + ' coins!';
						document.getElementById('win').textContent = data.gain;
					}
					
					if(data.wallet !== undefined) {
						// Update parent window with new balance
						window.parent.postMessage({ type: 'balance', value: data.wallet }, '*');
					}
				}
			} catch(e) {
				console.error('Spin error:', e);
				winMsg.textContent = 'Error: ' + e.message;
			}
			
			btn.disabled = false;
			btn.textContent = '🎰 SPIN 🎰';
		}
		
		// Initialize
		if(initialGrid) {
			renderGrid(initialGrid);
		}
		console.log('Game loaded:', alias, 'GID:', gid, 'UID:', uid, 'Grid:', initialGrid);
	</script>
</body>
</html>`, alias, alias, gid, arg.UID, gid, arg.UID, arg.CID, alias, gridJSON)

	c.Header("Content-Type", "text/html")
	c.String(200, html)
}

func ApiGameRtpGet(c *gin.Context) {
	// Try to get from URL param first (new endpoint: /game/rtp/:alias)
	alias := c.Param("alias")

	// If no alias in URL, try from query/body (old endpoint)
	if alias == "" {
		var arg struct {
			GID uint64 `json:"gid" form:"gid"`
		}
		if err := c.ShouldBind(&arg); err == nil && arg.GID > 0 {
			scene, err := GetScene(arg.GID)
			if err == nil {
				alias = scene.Alias
			}
		}
	}

	// Normalize alias
	if alias != "" {
		alias = strings.ToLower(strings.ReplaceAll(alias, "%2f", "-"))
		alias = strings.ReplaceAll(alias, "/", "-")

		// Log for debugging
		fmt.Printf("RTP request for alias: %s (normalized: %s)\n", c.Param("alias"), alias)
		fmt.Printf("Available games in InfoMap: %d\n", len(game.InfoMap))

		if gi, ok := game.InfoMap[alias]; ok {
			RetOk(c, gin.H{
				"mrtp":  cfg.DefMRTP, // Use default RTP instead of GetRTP(nil,nil)
				"rtp":   gi.FindClosest(96.0),
				"alias": alias,
			})
			return
		}

		// Fallback: return mock RTP for games not in InfoMap
		fmt.Printf("Game not found in InfoMap, returning mock RTP for: %s\n", alias)
		RetOk(c, gin.H{
			"mrtp":  cfg.DefMRTP, // Use default RTP
			"rtp":   96.0,        // Default mock RTP
			"alias": alias,
			"mock":  true,
		})
		return
	}

	Ret404(c, "game not found")
}

// ApiRecentWinners returns recent big wins
func ApiRecentWinners(c *gin.Context) {
	// Mock data - in production this should query from database
	winners := []gin.H{
		{"id": "1", "username": "Player847", "game": "Mines", "amount": 1250.50, "currency": "USD", "multiplier": 12.5, "time": "2m ago"},
		{"id": "2", "username": "LuckyOne", "game": "Crash", "amount": 3400.00, "currency": "USD", "multiplier": 34.0, "time": "5m ago"},
		{"id": "3", "username": "CryptoKing", "game": "Dice", "amount": 890.25, "currency": "USD", "multiplier": 8.9, "time": "8m ago"},
		{"id": "4", "username": "HighRoller", "game": "Blackjack", "amount": 2500.00, "currency": "USD", "multiplier": 2.5, "time": "12m ago"},
		{"id": "5", "username": "SlotMaster", "game": "Book of Ra", "amount": 5670.80, "currency": "USD", "multiplier": 56.7, "time": "15m ago"},
		{"id": "6", "username": "NightOwl", "game": "Mines", "amount": 420.00, "currency": "USD", "multiplier": 4.2, "time": "18m ago"},
	}

	RetOk(c, gin.H{
		"winners": winners,
		"total":   len(winners),
		"updated": time.Now().Unix(),
	})
}

// ApiUserSettingsSave saves user settings (theme, notifications, etc.)
func ApiUserSettingsSave(c *gin.Context) {
	var arg struct {
		UID           uint64 `json:"userId"`
		Theme         string `json:"theme"`
		Language      string `json:"language"`
		Notifications bool   `json:"notifications"`
	}

	if err := c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	// TODO: Save settings to database
	// For now just return success

	RetOk(c, gin.H{
		"success": true,
		"message": "Settings saved",
		"settings": gin.H{
			"theme":         arg.Theme,
			"language":      arg.Language,
			"notifications": arg.Notifications,
		},
	})
}
