package api

import (
	"encoding/xml"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	cfg "github.com/slotopol/server/config"
)

// Returns all properties for pointed user at pointed club.
func ApiPropsGet(c *gin.Context) {
	var ok bool
	var arg struct {
		CID uint64 `json:"cid"`
		UID uint64 `json:"uid"`
	}
	var ret struct {
		Wallet float64 `json:"wallet"`
		Access AL      `json:"access"`
		MRTP   float64 `json:"mrtp"`
	}

	// Try JSON body first
	if err := c.ShouldBindJSON(&arg); err != nil {
		// Try query params
		arg.CID, _ = strconv.ParseUint(c.Query("cid"), 10, 64)
		arg.UID, _ = strconv.ParseUint(c.Query("uid"), 10, 64)
	}

	// Set defaults
	if arg.CID == 0 {
		arg.CID = 1
	}
	if arg.UID == 0 {
		// Return defaults for guest (access level 0 = no access)
		ret.Wallet = 0
		ret.Access = 0
		ret.MRTP = 0
		RetOk(c, ret)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		// Auto-create default club
		cd := ClubData{CID: arg.CID}
		club = MakeClub(cd)
		Clubs.Set(arg.CID, club)
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		// Auto-create user with default props
		user = &User{
			UID:    arg.UID,
			Email:  fmt.Sprintf("user_%d@auto.local", arg.UID),
			Secret: fmt.Sprintf("auto_%d", arg.UID),
			Name:   fmt.Sprintf("Player_%d", arg.UID),
		}
		user.Init()
		Users.Set(arg.UID, user)
		// Create default props
		props := &Props{
			CID:    arg.CID,
			UID:    arg.UID,
			Wallet: 1000,
			Access: ALmember,
		}
		user.props.Set(arg.CID, props)
	}

	if props, ok := user.props.Get(arg.CID); ok {
		ret.Wallet = props.Wallet
		ret.Access = props.Access
		ret.MRTP = props.MRTP
	} else {
		// Create default props if not exists
		ret.Wallet = 1000
		ret.Access = ALmember
		ret.MRTP = 0
	}

	RetOk(c, ret)
}

// Returns balance at wallet for pointed user at pointed club.
func ApiPropsWalletGet(c *gin.Context) {
	var ok bool
	var arg struct {
		CID uint64 `json:"cid"`
		UID uint64 `json:"uid"`
	}
	var ret struct {
		Wallet float64 `json:"wallet"`
	}

	// Try JSON body first
	if err := c.ShouldBindJSON(&arg); err != nil {
		// Try query params
		arg.CID, _ = strconv.ParseUint(c.Query("cid"), 10, 64)
		arg.UID, _ = strconv.ParseUint(c.Query("uid"), 10, 64)
	}

	// Set defaults
	if arg.CID == 0 {
		arg.CID = 1
	}
	if arg.UID == 0 {
		// Return 0 balance for guest
		ret.Wallet = 0
		RetOk(c, ret)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		// Auto-create default club
		cd := ClubData{CID: arg.CID}
		club = MakeClub(cd)
		Clubs.Set(arg.CID, club)
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		// Auto-create user with default wallet
		user = &User{
			UID:    arg.UID,
			Email:  fmt.Sprintf("user_%d@auto.local", arg.UID),
			Secret: fmt.Sprintf("auto_%d", arg.UID),
			Name:   fmt.Sprintf("Player_%d", arg.UID),
		}
		user.Init()
		Users.Set(arg.UID, user)
		// Create default props
		props := &Props{
			CID:    arg.CID,
			UID:    arg.UID,
			Wallet: 1000, // Starting balance
			Access: ALmember,
		}
		user.props.Set(arg.CID, props)
	}

	ret.Wallet = user.GetWallet(arg.CID)

	RetOk(c, ret)
}

// Adds some coins to user wallet. Sum can be < 0 to remove some coins.
func ApiPropsWalletAdd(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		CID     uint64   `json:"cid" yaml:"cid" xml:"cid,attr" form:"cid" binding:"required"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		Sum     float64  `json:"sum" yaml:"sum" xml:"sum" binding:"required"`
	}
	var ret struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"ret"`
		Wallet  float64  `json:"wallet" yaml:"wallet" xml:"wallet"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}
	if arg.Sum > cfg.Cfg.AdjunctLimit || arg.Sum < -cfg.Cfg.AdjunctLimit {
		Ret400(c, ErrTooBig)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		Ret404(c, ErrNoClub)
		return
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, arg.CID)
	if al&ALbooker == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	var props *Props
	if props, ok = user.props.Get(arg.CID); !ok {
		Ret500(c, ErrNoProps)
		return
	}
	if props.Wallet+arg.Sum < 0 {
		Ret403(c, ErrNoMoney)
		return
	}

	// update wallet as transaction
	if cfg.Cfg.ClubInsertBuffer > 1 {
		go BankBat[arg.CID].Add(cfg.XormStorage, arg.UID, admin.UID, props.Wallet+arg.Sum, arg.Sum)
	} else if err = BankBat[arg.CID].Add(cfg.XormStorage, arg.UID, admin.UID, props.Wallet+arg.Sum, arg.Sum); err != nil {
		Ret500(c, err)
		return
	}

	// make changes to memory data
	props.Wallet += arg.Sum

	ret.Wallet = props.Wallet

	RetOk(c, ret)
}

// Returns personal access level for pointed user at pointed club.
func ApiPropsAlGet(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		CID     uint64   `json:"cid" yaml:"cid" xml:"cid,attr" form:"cid"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		All     bool     `json:"all" yaml:"all" xml:"all,attr" form:"all"`
	}
	var ret struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"ret"`
		Access  AL       `json:"access" yaml:"access" xml:"access"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok && !arg.All {
		Ret404(c, ErrNoClub)
		return
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, arg.CID)
	if admin != user && al&(ALbooker+ALadmin) == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	ret.Access = user.GetAL(arg.CID)
	if arg.All {
		ret.Access |= user.GAL
	}

	RetOk(c, ret)
}

// Set personal access level for given user at given club.
func ApiPropsAlSet(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		CID     uint64   `json:"cid" yaml:"cid" xml:"cid,attr" form:"cid" binding:"required"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid" binding:"required"`
		Access  AL       `json:"access" yaml:"access" xml:"access"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		Ret404(c, ErrNoClub)
		return
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, arg.CID)
	if al&ALadmin == 0 {
		Ret403(c, ErrNoAccess)
		return
	}
	_ = admin

	var props *Props
	if props, ok = user.props.Get(arg.CID); !ok {
		Ret500(c, ErrNoProps)
		return
	}
	if al&arg.Access != arg.Access {
		Ret403(c, ErrNoLevel)
		return
	}

	// update access level as transaction
	if cfg.Cfg.ClubInsertBuffer > 1 {
		go BankBat[arg.CID].Access(cfg.XormStorage, arg.UID, arg.Access)
	} else if err = BankBat[arg.CID].Access(cfg.XormStorage, arg.UID, arg.Access); err != nil {
		Ret500(c, err)
		return
	}

	// make changes to memory data
	props.Access = arg.Access

	Ret204(c)
}

// Returns master RTP for pointed user at pointed club.
// This RTP if it set have more priority then club RTP.
func ApiPropsRtpGet(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		CID     uint64   `json:"cid" yaml:"cid" xml:"cid,attr" form:"cid" binding:"required"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid"`
		All     bool     `json:"all" yaml:"all" xml:"all,attr" form:"all"`
	}
	var ret struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"ret"`
		MRTP    float64  `json:"mrtp" yaml:"mrtp" xml:"mrtp"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		Ret404(c, ErrNoClub)
		return
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok && !arg.All {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, arg.CID)
	if admin != user && al&ALbooker == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	if arg.All {
		ret.MRTP = GetRTP(user, club)
	} else {
		ret.MRTP = user.GetRTP(arg.CID)
	}

	RetOk(c, ret)
}

// Set personal master RTP for given user at given club.
func ApiPropsRtpSet(c *gin.Context) {
	var err error
	var ok bool
	var arg struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"arg"`
		CID     uint64   `json:"cid" yaml:"cid" xml:"cid,attr" form:"cid"`
		UID     uint64   `json:"uid" yaml:"uid" xml:"uid,attr" form:"uid"`
		MRTP    float64  `json:"mrtp" yaml:"mrtp" xml:"mrtp"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	var club *Club
	if club, ok = Clubs.Get(arg.CID); !ok {
		Ret404(c, ErrNoClub)
		return
	}
	_ = club

	var user *User
	if user, ok = Users.Get(arg.UID); !ok {
		Ret404(c, ErrNoUser)
		return
	}

	var admin, al = MustAdmin(c, arg.CID)
	if al&ALbooker == 0 {
		Ret403(c, ErrNoAccess)
		return
	}
	_ = admin

	var props *Props
	if props, ok = user.props.Get(arg.CID); !ok {
		Ret500(c, ErrNoProps)
		return
	}

	// update master RTP as transaction
	if cfg.Cfg.ClubInsertBuffer > 1 {
		go BankBat[arg.CID].MRTP(cfg.XormStorage, arg.UID, arg.MRTP)
	} else if err = BankBat[arg.CID].MRTP(cfg.XormStorage, arg.UID, arg.MRTP); err != nil {
		Ret500(c, err)
		return
	}

	// make changes to memory data
	props.MRTP = arg.MRTP

	Ret204(c)
}
