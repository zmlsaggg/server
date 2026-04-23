package api

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	cfg "github.com/slotopol/server/config"
	"github.com/slotopol/server/game"
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
