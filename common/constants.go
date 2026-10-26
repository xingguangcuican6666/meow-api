package common

import (
	"crypto/tls"
	//"os"
	//"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

var StartTime = time.Now().Unix() // unit: second
var Version = "v0.0.0"            // this hard coding will be replaced automatically when building, no need to manually change
var SystemName = "New API"
var Footer = ""
var Logo = ""
var TopUpLink = ""

// var ChatLink = ""
// var ChatLink2 = ""
var QuotaPerUnit = 500 * 1000.0 // $0.002 / 1K tokens
// 保留旧变量以兼容历史逻辑，实际展示由 general_setting.quota_display_type 控制
var DisplayInCurrencyEnabled = true
var DisplayTokenStatEnabled = true
var DrawingEnabled = true
var TaskEnabled = true
var DataExportEnabled = true
var DataExportInterval = 5         // unit: minute
var DataExportDefaultTime = "hour" // unit: minute
var DefaultCollapseSidebar = false // default value of collapse sidebar

// Any options with "Secret", "Token" in its key won't be return by GetOptions

var SessionSecret = uuid.New().String()
var CryptoSecret = uuid.New().String()
var SessionCookieSecure = false
var SessionCookieTrustedURLs []string

const (
	DefaultUserSessionActiveLimit           = 50
	DefaultUserSessionIssuanceLimit         = 100
	DefaultUserSessionIssuanceWindowSeconds = 24 * 60 * 60
	DefaultUserSessionRevokedRetentionDays  = 7
	DefaultUserSessionHourlyAlertThreshold  = 5000
)

var (
	UserSessionActiveLimit           = DefaultUserSessionActiveLimit
	UserSessionIssuanceLimit         = DefaultUserSessionIssuanceLimit
	UserSessionIssuanceWindowSeconds = int64(DefaultUserSessionIssuanceWindowSeconds)
	UserSessionRevokedRetentionDays  = DefaultUserSessionRevokedRetentionDays
	UserSessionHourlyAlertThreshold  = DefaultUserSessionHourlyAlertThreshold
)

var OptionMap map[string]string
var OptionMapRWMutex sync.RWMutex

var ItemsPerPage = 10
var MaxRecentItems = 1000

var PasswordLoginEnabled = true
var PasswordLoginEncryptionEnabled = false
var PasswordRegisterEnabled = true
var InviteCodeRegisterEnabled = true // 密码注册是否强制要求有效邀请码
var EmailVerificationEnabled = false
var GitHubOAuthEnabled = false
var LinuxDOOAuthEnabled = false
var WeChatAuthEnabled = false
var TelegramOAuthEnabled = false
var TurnstileCheckEnabled = false
var RegisterEnabled = true

var EmailDomainRestrictionEnabled = false // 是否启用邮箱域名限制
var EmailAliasRestrictionEnabled = false  // 是否启用邮箱别名限制
var EmailFormatRegex = ""                 // 自定义邮箱格式正则；空 = 仅内置校验
var EmailDomainWhitelist = []string{
	"gmail.com",
	"163.com",
	"126.com",
	"qq.com",
	"outlook.com",
	"hotmail.com",
	"icloud.com",
	"yahoo.com",
	"foxmail.com",
}
var EmailLoginAuthServerList = []string{
	"smtp.sendcloud.net",
	"smtp.azurecomm.net",
}

var DebugEnabled bool
var MemoryCacheEnabled bool

var LogConsumeEnabled = true

var TLSInsecureSkipVerify bool
var InsecureTLSConfig = &tls.Config{InsecureSkipVerify: true}

var SMTPServer = ""
var SMTPPort = 587
var SMTPSSLEnabled = false
var SMTPStartTLSEnabled = false
var SMTPInsecureSkipVerify = false
var SMTPForceAuthLogin = false
var SMTPAccount = ""
var SMTPFrom = ""
var SMTPToken = ""

var GitHubClientId = ""
var GitHubClientSecret = ""
var LinuxDOClientId = ""
var LinuxDOClientSecret = ""
var LinuxDOMinimumTrustLevel = 0

var WeChatServerAddress = ""
var WeChatServerToken = ""
var WeChatAccountQRCodeImageURL = ""

var TurnstileSiteKey = ""
var TurnstileSecretKey = ""

var TelegramBotToken = ""
var TelegramBotName = ""

var QuotaForNewUser = 0
var QuotaForInviter = 0
var QuotaForInvitee = 0
var ChannelDisableThreshold = 5.0
var AutomaticDisableChannelEnabled = false
var RuntimeAutomaticDisableChannelEnabled = false
var AutomaticEnableChannelEnabled = false
var EmptyResponseRetryEnabled = true
var EmptyResponseRetryInPlaceEnabled = true
var QuotaRemindThreshold = 1000

// PreConsumedQuota is retained for old option clients; token reservations now
// use quota_setting.pre_consume_multiplier and the estimated input cost.
var PreConsumedQuota = 500

var RetryTimes = 3

// Per-(channel, model) cooldown breaker tuning (service/channel_cooldown.go).
var ChannelCooldownFailureThreshold = 5       // consecutive bad outcomes before the skip arms
var ChannelCooldownBaseSeconds = 30           // initial skip duration
var ChannelCooldownMaxSeconds = 1800          // escalation cap
var ChannelSlowFirstByteSeconds = 90          // streaming first-byte limit (1min30s); 0 disables the slow skip
var ChannelModelMissingCooldownSeconds = 3600 // model-missing (404) skip duration; 0 disables the missing skip
var ChannelLatencyWeightingEnabled = true     // demote slow channels within a priority tier by recent TTFT

//var RootUserEmail = ""

var IsMasterNode bool

const (
	NodeNameSourceManual   = "manual"
	NodeNameSourceHostname = "hostname"
)

// NodeName 节点名称，优先从 NODE_NAME 环境变量读取，未配置时回退主机名。
// 用于审计日志和后台任务中标识节点身份；多实例部署时建议显式配置稳定 NODE_NAME。
var NodeName = ""

// NodeNameSource records how NodeName was chosen so future instance-management
// reporting can distinguish operator-configured names from automatic fallback.
var NodeNameSource = NodeNameSourceHostname

var NodeNameManuallyConfigured bool

var requestInterval int
var RequestInterval time.Duration

var SyncFrequency int // unit is second

var BatchUpdateEnabled = false
var BatchUpdateInterval int

var RelayTimeout int // unit is second

var RelayIdleConnTimeout int // unit is second

// RelayResponseHeaderTimeout limits how long the relay transport waits for the
// upstream response headers after the request has been fully written.
// 0 disables it (previous behaviour: wait forever).
//
// Note this is NOT the same as RelayTimeout (http.Client.Timeout), which covers
// the whole response read and therefore breaks legitimate long streaming calls.
// ResponseHeaderTimeout only bounds the wait for the response headers; once the
// headers arrive, streaming is unaffected.
var RelayResponseHeaderTimeout int // unit is second
var RelayMaxIdleConns int
var RelayMaxIdleConnsPerHost int

var GeminiSafetySetting string

// https://docs.cohere.com/docs/safety-modes Type; NONE/CONTEXTUAL/STRICT
var CohereSafetySetting string

const (
	RequestIdKey         = "X-Oneapi-Request-Id"
	UpstreamRequestIdKey = "X-Upstream-Request-Id"
)

const (
	RoleGuestUser  = 0
	RoleCommonUser = 1
	RoleAdminUser  = 10
	RoleRootUser   = 100
)

func IsValidateRole(role int) bool {
	return role == RoleGuestUser || role == RoleCommonUser || role == RoleAdminUser || role == RoleRootUser
}

var (
	FileUploadPermission    = RoleGuestUser
	FileDownloadPermission  = RoleGuestUser
	ImageUploadPermission   = RoleGuestUser
	ImageDownloadPermission = RoleGuestUser
)

// All duration's unit is seconds
// Shouldn't larger then RateLimitKeyExpirationDuration
var (
	GlobalApiRateLimitEnable   bool
	GlobalApiRateLimitNum      int
	GlobalApiRateLimitDuration int64

	GlobalWebRateLimitEnable   bool
	GlobalWebRateLimitNum      int
	GlobalWebRateLimitDuration int64

	CriticalRateLimitEnable   bool
	CriticalRateLimitNum            = 20
	CriticalRateLimitDuration int64 = 20 * 60

	UploadRateLimitNum            = 10
	UploadRateLimitDuration int64 = 60

	DownloadRateLimitNum            = 10
	DownloadRateLimitDuration int64 = 60

	// Per-user search rate limit (applies after authentication, keyed by user ID)
	SearchRateLimitEnable         = true
	SearchRateLimitNum            = 10
	SearchRateLimitDuration int64 = 60

	// NginxMode declares that this instance sits behind a reverse proxy —
	// typically nginx in a Docker deployment — where every client arrives from
	// the same container network address. Counting requests per IP then charges
	// one shared address for everyone's traffic, so the first legitimate user
	// to exhaust the budget locks out the whole deployment, and an actual
	// attacker is indistinguishable from that user.
	//
	// While it is on, every IP-keyed limiter is a no-op: global web/API,
	// critical, email verification, upload/download and task-artifact access.
	// Rate limiting by address becomes the proxy's responsibility, not this
	// service's. Limiters keyed by authenticated user ID stay active, because
	// the proxy cannot produce that identity.
	//
	// Set it with the NGINX_MODE environment variable or the root setting of
	// the same name. DISABLE_IP_RATE_LIMIT is an accepted alias for operators
	// who configured the earlier name.
	NginxMode = false

	// DisableRateLimit is the master switch that turns request-rate limiting
	// off entirely. While it is on, every rate limiter is a no-op: the IP-keyed
	// ones that NginxMode already neutralizes (global web/API, critical, email
	// verification, upload/download, task-artifact invalid-token) and — unlike
	// NginxMode — the ones keyed by authenticated user ID as well (model
	// request, user-critical, search). It exists for deployments behind two or
	// three proxy layers, where NginxMode alone still leaves the per-user
	// limiters biting because every client arrives as the same address.
	//
	// It deliberately does NOT touch concurrency admission caps or account
	// brute-force lockouts (2FA, email binding, session issuance): a shared
	// proxy address does not weaken those per-account controls, and disabling
	// them would drop protection the proxy cannot provide. Set it with the
	// DISABLE_RATE_LIMIT environment variable or the root setting of the same
	// name; a saved root setting overrides the bootstrap default once options
	// load from the database.
	DisableRateLimit = false
)

var RateLimitKeyExpirationDuration = 20 * time.Minute

const (
	UserStatusEnabled  = 1 // don't use 0, 0 is the default value!
	UserStatusDisabled = 2 // also don't use 0
)

// AuthErrorCodeUserDisabled is the stable machine code every ban response
// carries. The dashboard routes to the account-state page on this code, so
// the value must stay in sync with the frontend constant of the same name.
const AuthErrorCodeUserDisabled = "AUTH_USER_DISABLED"

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

const (
	RedemptionCodeStatusEnabled  = 1 // don't use 0, 0 is the default value!
	RedemptionCodeStatusDisabled = 2 // also don't use 0
	RedemptionCodeStatusUsed     = 3 // also don't use 0
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // don't use 0, 0 is the default value!
	ChannelStatusManuallyDisabled = 2 // also don't use 0
	ChannelStatusAutoDisabled     = 3
)

const (
	TopUpStatusPending = "pending"
	TopUpStatusSuccess = "success"
	TopUpStatusFailed  = "failed"
	TopUpStatusExpired = "expired"
)
