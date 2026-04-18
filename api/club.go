package api

import (
	"encoding/xml"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	cfg "github.com/slotopol/server/config"
)

const (
	sqlclub = "UPDATE club SET `bank`=`bank`+?, `fund`=`fund`+?, `lock`=`lock`+?, `utime`=CURRENT_TIMESTAMP WHERE `cid`=?"
)

var (
	ErrBadName   = errors.New("invalid club name")
	ErrEmptyCID  = errors.New("cid is required")
	ErrNoChanges = errors.New("no changes applied")
)

// =========================
// VALIDATION
// =========================

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	return s
}

func validateName(s string) error {
	if len(s) < 2 || len(s) > 64 {
		return ErrBadName
	}
	return nil
}

// =========================
// LIST
// =========================

func ApiClubList(c *gin.Context) {
	type item struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"club"`
		CID     uint64   `json:"cid" yaml:"cid" xml:"cid,attr"`
		Name    string   `json:"name,omitempty" yaml:"name,omitempty" xml:"name,omitempty"`
	}

	var ret struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"ret"`
		List    []item   `json:"list" yaml:"list" xml:"list>club"`
	}

	ret.List = make([]item, 0, Clubs.Len())

	for cid, club := range Clubs.Items() {
		if club == nil {
			continue
		}
		ret.List = append(ret.List, item{
			CID:  cid,
			Name: club.Name(),
		})
	}

	RetOk(c, ret)
}

// =========================
// IS CHECK
// =========================

func ApiClubIs(c *gin.Context) {
	var err error

	type item struct {
		XMLName xml.Name `json:"-" yaml:"-" xml:"user"`
		CID     uint64   `json:"cid,omitempty" yaml:"cid,omitempty" xml:"cid,attr,omitempty"`
		Name    string   `json:"name,omitempty" yaml:"name,omitempty" xml:"name,attr,omitempty"`
	}

	var arg struct {
		XMLName xml.Name `json:"-" xml:"arg"`
		List    []item   `json:"list" xml:"list>user" binding:"required"`
	}

	var ret struct {
		XMLName xml.Name `json:"-" xml:"ret"`
		List    []item   `json:"list" xml:"list>user"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	ret.List = make([]item, len(arg.List))

	for i, ai := range arg.List {
		var ri item

		if ai.CID != 0 {
			if club, ok := Clubs.Get(ai.CID); ok && club != nil {
				ri.CID = club.CID()
				ri.Name = club.Name()
			}
		} else {
			name := normalizeName(ai.Name)
			for _, club := range Clubs.Items() {
				if club != nil && club.Name() == name {
					ri.CID = club.CID()
					ri.Name = name
					break
				}
			}
		}

		ret.List[i] = ri
	}

	RetOk(c, ret)
}

// =========================
// INFO
// =========================

func ApiClubInfo(c *gin.Context) {
	var err error
	var ok bool

	var arg struct {
		XMLName xml.Name `json:"-" xml:"arg"`
		CID     uint64   `json:"cid" xml:"cid,attr" binding:"required"`
	}

	var ret struct {
		XMLName xml.Name `json:"-" xml:"ret"`
		ClubData
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	if arg.CID == 0 {
		Ret400(c, ErrEmptyCID)
		return
	}

	club, ok := Clubs.Get(arg.CID)
	if !ok || club == nil {
		Ret404(c, ErrNoClub)
		return
	}

	_, al := MustAdmin(c, arg.CID)
	if al&ALmaster == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	ret.ClubData = club.Get()

	RetOk(c, ret)
}

// =========================
// JP FUND
// =========================

func ApiClubJpfund(c *gin.Context) {
	var err error
	var ok bool

	var arg struct {
		XMLName xml.Name `json:"-" xml:"arg"`
		CID     uint64   `json:"cid" xml:"cid,attr" binding:"required"`
	}

	var ret struct {
		XMLName xml.Name `json:"-" xml:"ret"`
		JpFund  float64  `json:"jpfund"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	club, ok := Clubs.Get(arg.CID)
	if !ok || club == nil {
		Ret404(c, ErrNoClub)
		return
	}

	ret.JpFund = club.Fund()

	RetOk(c, ret)
}

// =========================
// RENAME
// =========================

func ApiClubRename(c *gin.Context) {
	var err error
	var ok bool

	var arg struct {
		XMLName xml.Name `json:"-" xml:"arg"`
		CID     uint64   `json:"cid" xml:"cid,attr" binding:"required"`
		Name    string   `json:"name" xml:"name" binding:"required"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	arg.Name = normalizeName(arg.Name)
	if err = validateName(arg.Name); err != nil {
		Ret400(c, err)
		return
	}

	club, ok := Clubs.Get(arg.CID)
	if !ok || club == nil {
		Ret404(c, ErrNoClub)
		return
	}

	_, al := MustAdmin(c, arg.CID)
	if al&ALmaster == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	_, err = cfg.XormStorage.ID(club.CID()).
		Cols("name").
		Update(&ClubData{Name: arg.Name})

	if err != nil {
		Ret500(c, err)
		return
	}

	club.SetName(arg.Name)

	Ret204(c)
}

// =========================
// CASHIN (HARDENED)
// =========================

func ApiClubCashin(c *gin.Context) {
	var err error
	var ok bool

	var arg struct {
		XMLName xml.Name `json:"-" xml:"arg"`
		CID     uint64   `json:"cid" binding:"required"`
		BankSum float64  `json:"banksum"`
		FundSum float64  `json:"fundsum"`
		LockSum float64  `json:"locksum"`
	}

	if err = c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	if arg.CID == 0 {
		Ret400(c, ErrEmptyCID)
		return
	}

	if arg.BankSum == 0 && arg.FundSum == 0 && arg.LockSum == 0 {
		Ret400(c, ErrNoChanges)
		return
	}

	club, ok := Clubs.Get(arg.CID)
	if !ok || club == nil {
		Ret404(c, ErrNoClub)
		return
	}

	_, al := MustAdmin(c, arg.CID)
	if al&ALmaster == 0 {
		Ret403(c, ErrNoAccess)
		return
	}

	bank, fund, lock := club.GetCash()

	bank += arg.BankSum
	fund += arg.FundSum
	lock += arg.LockSum

	if bank < 0 || fund < 0 || lock < 0 {
		Ret403(c, ErrBankOut)
		return
	}

	rec := Banklog{
		Bank:    bank,
		Fund:    fund,
		Lock:    lock,
		BankSum: arg.BankSum,
		FundSum: arg.FundSum,
		LockSum: arg.LockSum,
	}

	err = SafeTransaction(cfg.XormStorage, func(session *Session) error {
		if _, err := session.Exec(sqlclub, arg.BankSum, arg.FundSum, arg.LockSum, club.CID()); err != nil {
			return err
		}
		if _, err := session.Insert(&rec); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		Ret500(c, err)
		return
	}

	club.AddCash(arg.BankSum, arg.FundSum, arg.LockSum)

	RetOk(c, gin.H{
		"bid":  rec.ID,
		"bank": bank,
		"fund": fund,
		"lock": lock,
	})
}
