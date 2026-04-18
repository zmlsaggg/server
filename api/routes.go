package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func SetupRouter(r *gin.Engine) {
	r.Use(func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		h.Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	r.NoRoute(func(c *gin.Context) {
		Ret404(c, "not found")
	})

	rootStatus := func(c *gin.Context) {
		RetOk(c, gin.H{
			"name":   "slotopol",
			"status": "ok",
		})
	}

	r.GET("/", rootStatus)
	r.GET("/healthz", rootStatus)
	r.GET("/ping", func(c *gin.Context) {
		RetOk(c, gin.H{"ok": true})
	})
	r.GET("/online", func(c *gin.Context) {
		RetOk(c, gin.H{"online": 7})
	})

	r.GET("/game/list", ApiGameList)
	r.GET("/game/algs", ApiGameAlgs)

	api := r.Group("/api")
	{
		api.GET("/ping", func(c *gin.Context) {
			RetOk(c, gin.H{"ok": true})
		})

		api.GET("/online", func(c *gin.Context) {
			RetOk(c, gin.H{"online": 7})
		})

		api.GET("/slots/game/:alias", ApiGameInfo)
		api.GET("/slots/load", ApiGameList)
		api.POST("/slots/spin", ApiSlotSpin)

		slots := api.Group("/slots")
		{
			slots.POST("/bet/get", ApiSlotBetGet)
			slots.POST("/bet/set", ApiSlotBetSet)
			slots.POST("/sel/get", ApiSlotSelGet)
			slots.POST("/sel/set", ApiSlotSelSet)
			slots.POST("/mode/set", ApiSlotModeSet)
			slots.POST("/doubleup", ApiSlotDoubleup)
			slots.POST("/collect", ApiSlotCollect)
		}

		api.POST("/game/new", ApiGameNew)
		api.POST("/game/join", ApiGameJoin)
	}
}
