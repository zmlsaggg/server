package api

import (
	"bytes"
	"encoding/xml"
	"strings"

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

	tests := []string{"buffalo", "lucky-88", "queen-of-the-nile"}
	for _, t := range tests {
		gamelist = append(gamelist, gameOut{
			ID:       index,
			GameID:   t,
			Title:    strings.Title(strings.ReplaceAll(t, "-", " ")),
			Provider: "Aristocrat",
			Alias:    t,
			Image:    "/assets/slots/" + t + "/icon.png",
			Banner:   "/assets/slots/" + t + "/banner.jpg",
		})
		index++
	}

	for _, gi := range game.InfoMap {
		id := gi.ID()

		gamelist = append(gamelist, gameOut{
			ID:       index,
			GameID:   id,
			Title:    gi.Name,
			Provider: gi.Prov,
			Alias:    id,
			Image:    "/assets/slots/" + id + "/icon.png",
			Banner:   "/assets/slots/" + id + "/banner.jpg",
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
		Ret404(c, "user not found")
		return
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
