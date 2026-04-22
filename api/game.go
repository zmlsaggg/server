package api

import (
	"bytes"
	"encoding/xml"
	"fmt"
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
		XMLName xml.Name `json:"-" xml:"arg"`
		CID     uint64   `json:"cid" form:"cid" binding:"required"`
		UID     uint64   `json:"uid" form:"uid" binding:"required"`
		Alias   string   `json:"alias" form:"alias" binding:"required"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
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
	var arg struct {
		GID uint64 `json:"gid" form:"gid"`
	}

	if err := c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	scene, err := GetScene(arg.GID)
	if err != nil {
		Ret404(c, err)
		return
	}

	gi, ok := game.InfoMap[scene.Alias]
	if !ok {
		Ret404(c, "game not found")
		return
	}

	RetOk(c, gin.H{
		"mrtp": GetRTP(nil, nil),
		"rtp":  gi.FindClosest(96.0),
	})
}
