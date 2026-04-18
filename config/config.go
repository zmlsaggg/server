package cfg

import (
	"errors"
	"strings"
	"time"
)

var (
	BuildVers string
	BuildTime string
)

const DefMRTP = 95.0

// =========================
// ERRORS
// =========================
var (
	ErrBadConfig = errors.New("invalid configuration value")
	ErrEmptyKey  = errors.New("crypto key cannot be empty")
	ErrBadTTL    = errors.New("invalid ttl configuration")
)

// =========================
// JWT AUTH
// =========================
type CfgJwtAuth struct {
	AccessTTL    time.Duration `json:"access-ttl" yaml:"access-ttl" mapstructure:"access-ttl"`
	RefreshTTL   time.Duration `json:"refresh-ttl" yaml:"refresh-ttl" mapstructure:"refresh-ttl"`
	AccessKey    string        `json:"access-key" yaml:"access-key" mapstructure:"access-key"`
	RefreshKey   string        `json:"refresh-key" yaml:"refresh-key" mapstructure:"refresh-key"`
	NonceTimeout time.Duration `json:"nonce-timeout" yaml:"nonce-timeout" mapstructure:"nonce-timeout"`
}

func (c *CfgJwtAuth) Validate() error {
	if c.AccessTTL <= 0 || c.RefreshTTL <= 0 {
		return ErrBadTTL
	}
	if len(c.AccessKey) < 32 || len(c.RefreshKey) < 32 {
		return ErrEmptyKey
	}
	if c.NonceTimeout <= 0 {
		return ErrBadTTL
	}
	return nil
}

// =========================
// SEND CODE
// =========================
type CfgSendCode struct {
	UseActivation      bool          `json:"use-activation" yaml:"use-activation" mapstructure:"use-activation"`
	BrevoApiKey        string        `json:"brevo-api-key" yaml:"brevo-api-key" mapstructure:"brevo-api-key"`
	BrevoEmailEndpoint string        `json:"brevo-email-endpoint" yaml:"brevo-email-endpoint" mapstructure:"brevo-email-endpoint"`
	SenderName         string        `json:"sender-name" yaml:"sender-name" mapstructure:"sender-name"`
	SenderEmail        string        `json:"sender-email" yaml:"sender-email" mapstructure:"sender-email"`
	ReplytoEmail       string        `json:"replyto-email" yaml:"replyto-email" mapstructure:"replyto-email"`
	EmailSubject       string        `json:"email-subject" yaml:"email-subject" mapstructure:"email-subject"`
	EmailHtmlContent   string        `json:"email-html-content" yaml:"email-html-content" mapstructure:"email-html-content"`
	CodeTimeout        time.Duration `json:"code-timeout" yaml:"code-timeout" mapstructure:"code-timeout"`
}

func (c *CfgSendCode) Validate() error {
	if c.CodeTimeout < time.Minute {
		return ErrBadConfig
	}
	if c.SenderEmail == "" || !strings.Contains(c.SenderEmail, "@") {
		return ErrBadConfig
	}
	return nil
}

// =========================
// WEB SERVER
// =========================
type CfgWebServ struct {
	TrustedProxies    []string      `json:"trusted-proxies" yaml:"trusted-proxies" mapstructure:"trusted-proxies"`
	PortHTTP          []string      `json:"port-http" yaml:"port-http" mapstructure:"port-http"`
	ReadTimeout       time.Duration `json:"read-timeout" yaml:"read-timeout" mapstructure:"read-timeout"`
	ReadHeaderTimeout time.Duration `json:"read-header-timeout" yaml:"read-header-timeout" mapstructure:"read-header-timeout"`
	WriteTimeout      time.Duration `json:"write-timeout" yaml:"write-timeout" mapstructure:"write-timeout"`
	IdleTimeout       time.Duration `json:"idle-timeout" yaml:"idle-timeout" mapstructure:"idle-timeout"`
	MaxHeaderBytes    int           `json:"max-header-bytes" yaml:"max-header-bytes" mapstructure:"max-header-bytes"`
	ShutdownTimeout   time.Duration `json:"shutdown-timeout" yaml:"shutdown-timeout" mapstructure:"shutdown-timeout"`
}

func (c *CfgWebServ) Validate() error {
	if len(c.PortHTTP) == 0 {
		return ErrBadConfig
	}
	if c.MaxHeaderBytes < 1024 {
		return ErrBadConfig
	}
	return nil
}

// =========================
// DB CONFIG
// =========================
type CfgXormDrv struct {
	DriverName       string        `json:"driver-name" yaml:"driver-name" mapstructure:"driver-name"`
	UseSpinLog       bool          `json:"use-spin-log" yaml:"use-spin-log" mapstructure:"use-spin-log"`
	ClubSourceName   string        `json:"club-source-name" yaml:"club-source-name" mapstructure:"club-source-name"`
	SpinSourceName   string        `json:"spin-source-name" yaml:"spin-source-name" mapstructure:"spin-source-name"`
	SqlFlushTick     time.Duration `json:"sql-flush-tick" yaml:"sql-flush-tick" mapstructure:"sql-flush-tick"`
	ClubUpdateBuffer int           `json:"club-update-buffer" yaml:"club-update-buffer" mapstructure:"club-update-buffer"`
	ClubInsertBuffer int           `json:"club-insert-buffer" yaml:"club-insert-buffer" mapstructure:"club-insert-buffer"`
	SpinInsertBuffer int           `json:"spin-insert-buffer" yaml:"spin-insert-buffer" mapstructure:"spin-insert-buffer"`
}

func (c *CfgXormDrv) Validate() error {
	if c.DriverName == "" {
		return ErrBadConfig
	}
	if c.ClubSourceName == "" || c.SpinSourceName == "" {
		return ErrBadConfig
	}
	if c.SqlFlushTick < 500*time.Millisecond {
		return ErrBadConfig
	}
	return nil
}

// =========================
// GAMEPLAY
// =========================
type CfgGameplay struct {
	AdjunctLimit   float64 `json:"adjunct-limit" yaml:"adjunct-limit" mapstructure:"adjunct-limit"`
	MinJackpot     float64 `json:"min-jackpot" yaml:"min-jackpot" mapstructure:"min-jackpot"`
	MaxSpinAttempts int     `json:"max-spin-attempts" yaml:"max-spin-attempts" mapstructure:"max-spin-attempts"`
}

func (c *CfgGameplay) Validate() error {
	if c.AdjunctLimit <= 0 {
		return ErrBadConfig
	}
	if c.MaxSpinAttempts <= 0 || c.MaxSpinAttempts > 5000 {
		return ErrBadConfig
	}
	return nil
}

// =========================
// MAIN CONFIG
// =========================
type Config struct {
	CfgJwtAuth
	CfgSendCode
	CfgWebServ
	CfgXormDrv
	CfgGameplay
}

var Cfg = &Config{
	CfgJwtAuth: CfgJwtAuth{
		AccessTTL:    24 * time.Hour,
		RefreshTTL:   72 * time.Hour,
		AccessKey:    "CHANGE_ME_ACCESS_KEY_32+_CHARS_MINIMUM!!!!!!!!",
		RefreshKey:   "CHANGE_ME_REFRESH_KEY_32+_CHARS_MINIMUM!!!!!!",
		NonceTimeout: 150 * time.Second,
	},
	CfgSendCode: CfgSendCode{
		CodeTimeout: 15 * time.Minute,
	},
	CfgWebServ: CfgWebServ{
		TrustedProxies:    []string{"127.0.0.0/8"},
		PortHTTP:          []string{":8080"},
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ShutdownTimeout:   15 * time.Second,
	},
	CfgXormDrv: CfgXormDrv{
		DriverName:       "sqlite3",
		UseSpinLog:       true,
		ClubSourceName:   ":memory:",
		SpinSourceName:   ":memory:",
		SqlFlushTick:     2 * time.Second,
		ClubUpdateBuffer: 1,
		ClubInsertBuffer: 1,
		SpinInsertBuffer: 1,
	},
	CfgGameplay: CfgGameplay{
		AdjunctLimit:    100000,
		MinJackpot:      10000,
		MaxSpinAttempts: 300,
	},
}
