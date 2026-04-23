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
		h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, x-user-id")
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
	api.Use(Auth(false))
	{
		// Auth routes (ДОБАВЛЕНО)
		api.POST("/signin", ApiSignin)
		api.POST("/signup", ApiSignup)
		api.POST("/refresh", Auth(true), ApiRefresh)
		api.GET("/signis", Auth(false), ApiSignis)

		api.GET("/ping", func(c *gin.Context) {
			RetOk(c, gin.H{"ok": true})
		})

		api.GET("/online", func(c *gin.Context) {
			RetOk(c, gin.H{"online": 7})
		})

		// User endpoints (needed by frontend)
		api.POST("/user", ApiUserIs)
		api.GET("/user/:uid", ApiUserGet)
		api.POST("/update-currency", ApiUpdateCurrency)
		api.POST("/user/promo-activate", func(c *gin.Context) {
			RetOk(c, gin.H{"success": true, "message": "Promo activated"})
		})
		api.GET("/user/settings", ApiUserSettings)
		api.POST("/user/settings", ApiUserSettingsSave)

		api.GET("/slots/game/:alias", ApiGameInfo)
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

		// Props endpoints
		api.POST("/prop/get", ApiPropsGet)
		api.POST("/prop/wallet/get", ApiPropsWalletGet)
		api.POST("/prop/wallet/add", ApiPropsWalletAdd)
		api.POST("/prop/al/get", ApiPropsAlGet)
		api.POST("/prop/al/set", ApiPropsAlSet)
		api.POST("/prop/rtp/get", ApiPropsRtpGet)
		api.POST("/prop/rtp/set", ApiPropsRtpSet)

		// Slot game sessions
		api.POST("/game/new", ApiGameNew)
		api.POST("/game/join", ApiGameJoin)

		// Original games (Dice, Mines, Crash, etc.)
		api.POST("/original/new", ApiOriginalNew)
		api.POST("/original/join", ApiOriginalJoin)
		api.POST("/original/info", ApiOriginalInfo)
		api.POST("/original/rtp/get", ApiOriginalRtpGet)
		api.GET("/original/algs", ApiOriginalAlgs)

		// Giveaway stub (not implemented yet)
		api.GET("/giveaway/active", func(c *gin.Context) {
			RetOk(c, gin.H{
				"active": false,
				"list":   []interface{}{},
			})
		})

		// Recent winners endpoint
		api.GET("/winners/recent", ApiRecentWinners)

		// RTP endpoint for all games
		api.GET("/game/rtp/:alias", ApiGameRtpGet)

		// Admin endpoints (stubs for frontend compatibility)
		api.POST("/admin/login", func(c *gin.Context) {
			var arg struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := c.ShouldBindJSON(&arg); err != nil {
				Ret400(c, err)
				return
			}
			// Stub - always success for demo
			RetOk(c, gin.H{
				"success":   true,
				"userId":    "1",
				"isAdmin":   true,
				"adminRole": "superadmin",
				"token":     "demo_admin_token",
			})
		})
		api.POST("/admin/user-list", func(c *gin.Context) {
			RetOk(c, gin.H{"list": []interface{}{}})
		})
		api.POST("/admin/add-balance", func(c *gin.Context) {
			RetOk(c, gin.H{"success": true})
		})
		api.POST("/admin/ban", func(c *gin.Context) {
			RetOk(c, gin.H{"success": true})
		})
		api.POST("/admin/unban", func(c *gin.Context) {
			RetOk(c, gin.H{"success": true})
		})
	}
}
