package api

import (
	"encoding/xml"
	"strconv"

	"github.com/gin-gonic/gin"
	cfg "github.com/slotopol/server/config"
	"github.com/slotopol/server/util"
)

const (
	sqllock = "UPDATE club SET `lock`=`lock`+?, `utime`=CURRENT_TIMESTAMP WHERE `cid`=?"
)

func ApiUserIs(c *gin.Context) {
	type item struct {
		UID   uint64 `json:"uid"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	// Try to bind as {list: [...]} first
	var argList struct {
		List []item `json:"list"`
	}

	// Try to bind as single uid query
	var argSingle struct {
		UID   uint64 `json:"uid" form:"uid"`
		Email string `json:"email" form:"email"`
	}

	var items []item

	// For GET requests, try query params first
	if c.Request.Method == "GET" {
		if err := c.ShouldBindQuery(&argSingle); err == nil && argSingle.UID != 0 {
			items = []item{{UID: argSingle.UID, Email: argSingle.Email}}
		} else {
			// Try URL param
			uidStr := c.Param("uid")
			if uidStr == "" {
				uidStr = c.Query("uid")
			}
			if uidStr != "" {
				if uid, err := strconv.ParseUint(uidStr, 10, 64); err == nil {
					items = []item{{UID: uid}}
				}
			}
		}
	} else {
		// For POST requests, try JSON body
		if err := c.ShouldBindJSON(&argList); err == nil && len(argList.List) > 0 {
			items = argList.List
		} else if err := c.ShouldBindJSON(&argSingle); err == nil {
			items = []item{{UID: argSingle.UID, Email: argSingle.Email}}
		} else if err := c.ShouldBindQuery(&argSingle); err == nil {
			items = []item{{UID: argSingle.UID, Email: argSingle.Email}}
		}
	}

	if len(items) == 0 {
		RetOk(c, gin.H{"list": []item{}})
		return
	}

	var ret struct {
		List []item `json:"list"`
	}
	ret.List = make([]item, len(items))
	for i, ai := range items {
		var ri item
		if ai.UID != 0 {
			if user, ok := Users.Get(ai.UID); ok {
				ri.UID = user.UID
				ri.Email = user.Email
				ri.Name = user.Name
			}
		} else if ai.Email != "" {
			var email = util.ToLower(ai.Email)
			for _, user := range Users.Items() {
				if user.Email == email {
					ri.UID = user.UID
					ri.Email = user.Email
					ri.Name = user.Name
					break
				}
			}
		}
		ret.List[i] = ri
	}

	RetOk(c, ret)
}

// ApiUserGet returns user data for a single UID (GET endpoint for frontend)
func ApiUserGet(c *gin.Context) {
	uidStr := c.Param("uid")
	if uidStr == "" {
		uidStr = c.Query("uid")
	}

	uid, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil {
		Ret400(c, err)
		return
	}

	var user *User
	var ok bool
	if user, ok = Users.Get(uid); !ok {
		// Try to load from database
		session := cfg.XormStorage.NewSession()
		defer session.Close()

		user = &User{UID: uid}
		if has, err := session.Get(user); err != nil || !has {
			Ret404(c, ErrNoUser)
			return
		}
		// Add to cache
		Users.Set(uid, user)
	}

	RetOk(c, gin.H{
		"uid":     user.UID,
		"email":   user.Email,
		"name":    user.Name,
		"balance": user.GetWallet(1), // Default CID=1
	})
}

// Changes 'Name' of given user.
func ApiUserRename(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		Name    string   `json:"name" yaml:"name" xml:"name" form:"name" binding:"required"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, 0)
	if admin != user && al&ALbooker == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	if _, err = cfg.XormStorage.ID(user.UID).Cols("name").Update(&User{Name: arg.Name}); err != nil {
		Ret500(c, err)
		return
	}
	user.Name = arg.Name

	Ret204(c)
}

func ApiUserSecret(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName   xml.Name `json:"-" yaml:"-" xml:"arg"`
		UID       uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		OldSecret string   `json:"oldsecret" yaml:"oldsecret" xml:"oldsecret" form:"oldsecret" binding:"required"`
		NewSecret string   `json:"newsecret" yaml:"newsecret" xml:"newsecret" form:"newsecret" binding:"required"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}
	if len(arg.NewSecret) < 6 {
		Ret400(c, ErrSmallKey)
		return
	}

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, 0)
	if admin != user && al&(ALbooker+ALadmin) == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	if arg.OldSecret != user.Secret && al&ALadmin == 0 {
		Ret403(c, ErrNotConf)
		return
	}

	if _, err = cfg.XormStorage.ID(user.UID).Cols("secret").Update(&User{Secret: arg.NewSecret}); err != nil {
		Ret500(c, err)
		return
	}
	user.Secret = arg.NewSecret

	Ret204(c)
}

// Deletes registration, drops user and all linked records from database,
// and moves all remained coins at wallets to clubs deposits.
func ApiUserDelete(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		Secret  string   `json:"secret" yaml:"secret" xml:"secret" form:"secret"`
	}
	var ret struct {
		XMLName xml.Name           `json:"-" yaml:"-" xml:"ret"`
		Wallets map[uint64]float64 `json:"wallets" yaml:"wallets" xml:"wallets"`
	}
	ret.Wallets = map[uint64]float64{}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, 0)
	if admin != user && al&ALbooker == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	if arg.Secret != user.Secret && al&ALadmin == 0 {
		Ret403(c, ErrNotConf)
		return
	}

	// write gain and total bet as transaction
	if err = SafeTransaction(cfg.XormStorage, func(session *Session) (err error) {
		if _, err = session.ID(arg.UID).Delete(user); err != nil {
			Ret500(c, err)
			return
		}

		for cid, props := range user.props.Items() {
			if props.Wallet != 0 {
				if _, err = session.Exec(sqllock, props.Wallet, cid); err != nil {
					Ret500(c, err)
					return
				}
			}
		}

		if _, err = session.Where("uid=?", arg.UID).Delete(&Props{}); err != nil {
			Ret500(c, err)
			return
		}

		if _, err = session.Where("uid=?", arg.UID).Delete(&Scene{}); err != nil {
			Ret500(c, err)
			return
		}

		return
	}); err != nil {
		return
	}

	Users.Delete(arg.UID)
	for cid, props := range user.props.Items() {
		ret.Wallets[cid] = props.Wallet
		if club, ok := Clubs.Get(cid); ok && props.Wallet != 0 {
			club.AddDeposit(props.Wallet)
			ret.Wallets[cid] = props.Wallet
		}
	}
	for gid, scene := range Scenes.Items() {
		if scene.UID == arg.UID {
			Scenes.Delete(gid)
		}
	}

	RetOk(c, ret)
}

// ApiUpdateCurrency handles currency switching from frontend.
// Currency is managed on the frontend; backend just acknowledges.
func ApiUpdateCurrency(c *gin.Context) {
	var arg struct {
		ID              string  `json:"id" form:"id" binding:"required"`
		Currency        string  `json:"currency" form:"currency" binding:"required"`
		ExpectedBalance float64 `json:"expectedBalance" form:"expectedBalance"`
	}

	if err := c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	RetOk(c, gin.H{
		"success":  true,
		"currency": arg.Currency,
	})
}

// ApiUserSettings returns user settings for the authenticated user
func ApiUserSettings(c *gin.Context) {
	user, err := extractAuth(c)
	if err != nil {
		Ret401(c, err)
		return
	}

	RetOk(c, gin.H{
		"uid":   user.UID,
		"email": user.Email,
		"name":  user.Name,
		"settings": gin.H{
			"language": "ru",
			"currency": "USD",
			"theme":    "dark",
		},
	})
}

// ApiAddBalance adds balance to user wallet, creates props if not exists
func ApiAddBalance(c *gin.Context) {
	var arg struct {
		UID    uint64  `json:"uid" form:"uid" binding:"required"`
		CID    uint64  `json:"cid" form:"cid" binding:"required"`
		Amount float64 `json:"amount" form:"amount" binding:"required"`
	}

	if err := c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var user *User
	var ok bool
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		Ret404(c, ErrNoClub)
		return
	}
	_ = club

	// Get or create props
	var props *Props
	if props, ok = user.props.Get(arg.CID); !ok {
		// Create new props for this club
		props = &Props{
			CID:    arg.CID,
			UID:    arg.UID,
			Wallet: arg.Amount,
		}
		if err := user.InsertPropsDB(props); err != nil {
			Ret500(c, err)
			return
		}
	} else {
		// Update existing props
		newBalance := props.Wallet + arg.Amount
		if cfg.Cfg.ClubInsertBuffer > 1 {
			go BankBat[arg.CID].Add(cfg.XormStorage, arg.UID, arg.UID, newBalance, arg.Amount)
		} else if err := BankBat[arg.CID].Add(cfg.XormStorage, arg.UID, arg.UID, newBalance, arg.Amount); err != nil {
			Ret500(c, err)
			return
		}
		props.Wallet = newBalance
	}

	RetOk(c, gin.H{
		"success": true,
		"uid":     arg.UID,
		"cid":     arg.CID,
		"wallet":  props.Wallet,
		"added":   arg.Amount,
	})
}
