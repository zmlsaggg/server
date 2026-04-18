package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	cfg "github.com/slotopol/server/config"
	"github.com/slotopol/server/util"
)

const (
	jwtIssuer = "slotopol"
	userKey   = "user"
)

var (
	ErrNoAuth   = errors.New("no authorization")
	ErrNoScheme = errors.New("bad auth scheme")
	ErrNoCred   = errors.New("invalid credentials")

	ErrNoJwtID  = errors.New("jwt missing uid")
	ErrBadJwtID = errors.New("user not found")

	ErrNotPass  = errors.New("wrong password")
	ErrSigTime  = errors.New("bad signature time")
	ErrSigOut   = errors.New("signature expired")
	ErrBadHash  = errors.New("bad hmac hash")
	ErrSmallKey = errors.New("password too small")
)

type Claims struct {
	jwt.RegisteredClaims
	UID uint64 `json:"uid"`
}

type AuthResp struct {
	UID     uint64 `json:"uid"`
	Email   string `json:"email"`
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Expire  string `json:"expire"`
	Living  string `json:"living"`
}

func (r *AuthResp) Setup(user *User) {
	now := time.Now()

	accessExp := now.Add(cfg.Cfg.AccessTTL)
	refreshExp := now.Add(cfg.Cfg.RefreshTTL)

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExp),
		},
		UID: user.UID,
	})

	r.Access, _ = accessToken.SignedString([]byte(cfg.Cfg.AccessKey))
	r.Expire = accessExp.Format(time.RFC3339)

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExp),
		},
		UID: user.UID,
	})

	r.Refresh, _ = refreshToken.SignedString([]byte(cfg.Cfg.RefreshKey))

	r.Living = refreshExp.Format(time.RFC3339)
	r.UID = user.UID
	r.Email = user.Email
}

func GetBasicAuth(credentials string) (*User, error) {
	data, err := base64.StdEncoding.DecodeString(credentials)
	if err != nil {
		return nil, ErrNoCred
	}

	parts := strings.Split(string(data), ":")
	if len(parts) != 2 {
		return nil, ErrNoCred
	}

	email := util.ToLower(parts[0])
	pass := parts[1]

	for _, u := range Users.Items() {
		if u.Email == email && u.Secret == pass {
			return u, nil
		}
	}

	return nil, ErrNoCred
}

func GetBearerAuth(tokenStr string) (*User, error) {
	claims := Claims{}

	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		return []byte(cfg.Cfg.AccessKey), nil
	}, jwt.WithIssuer(jwtIssuer))

	if err != nil || !token.Valid {
		return nil, ErrNoAuth
	}

	user, ok := Users.Get(claims.UID)
	if !ok {
		return nil, ErrBadJwtID
	}

	return user, nil
}

func Auth(required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := extractAuth(c)

		if user != nil {
			c.Set(userKey, user)
			c.Next()
			return
		}

		if required {
			c.Abort()
			Ret401(c, ErrNoAuth)
			return
		}

		c.Next()
	}
}

func extractAuth(c *gin.Context) (*User, error) {
	h := c.GetHeader("Authorization")

	if strings.HasPrefix(h, "Basic ") {
		return GetBasicAuth(h[6:])
	}

	if strings.HasPrefix(h, "Bearer ") {
		return GetBearerAuth(h[7:])
	}

	return nil, ErrNoAuth
}

func ApiSignin(c *gin.Context) {
	var arg struct {
		UID     uint64 `form:"uid"`
		Email   string `form:"email"`
		Secret  string `form:"secret"`
		SigTime string `form:"sigtime"`
		HS256   string `form:"hs256"`
		Code    uint32 `form:"code"`
	}

	if err := c.ShouldBind(&arg); err != nil {
		Ret400(c, err)
		return
	}

	email := util.ToLower(arg.Email)

	var user *User
	if arg.UID != 0 {
		user, _ = Users.Get(arg.UID)
	} else {
		for _, u := range Users.Items() {
			if u.Email == email {
				user = u
				break
			}
		}
	}

	if user == nil {
		Ret403(c, ErrNoCred)
		return
	}

	if arg.Secret != "" {
		if user.Secret != arg.Secret {
			Ret403(c, ErrNotPass)
			return
		}
	} else {
		t, err := time.Parse(time.RFC3339, arg.SigTime)
		if err != nil {
			Ret403(c, ErrSigTime)
			return
		}
		if time.Since(t) > cfg.Cfg.NonceTimeout {
			Ret403(c, ErrSigOut)
			return
		}

		hash, err := hex.DecodeString(arg.HS256)
		if err != nil {
			Ret400(c, ErrBadHash)
			return
		}

		mac := hmac.New(sha256.New, []byte(arg.SigTime))
		mac.Write([]byte(user.Secret))

		if !hmac.Equal(mac.Sum(nil), hash) {
			Ret403(c, ErrNotPass)
			return
		}
	}

	var resp AuthResp
	resp.Setup(user)

	RetOk(c, resp)
}

func ApiRefresh(c *gin.Context) {
	userAny, ok := c.Get(userKey)
	if !ok {
		Ret401(c, ErrNoAuth)
		return
	}

	user := userAny.(*User)

	var resp AuthResp
	resp.Setup(user)

	RetOk(c, resp)
}
