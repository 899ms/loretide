// Package ipprofile holds the brand's content accounts and the expression
// settings attached to them.
//
// An Account here is the handle that publishes on a platform. It is not a login
// (upstream "user") and not an IM bot installation (upstream
// channel_installation, which is keyed by agent and holds app credentials).
// Account and ChannelAccount are the same entity: one table, not two layers.
//
// There is deliberately no separate persona table and no binding table. The
// expression settings hang off the account, which is what W-02 means by "no
// standalone persona model"; TestThisFeatureAddsNoPersonaOrBindingTable holds
// that boundary.
//
// Contract: specs/015-lt011-account-platform-config/contracts/account-api.md
package ipprofile

import (
	"errors"
	"strings"
)

// Platform is the content platform an account publishes on.
type Platform string

const (
	PlatformXiaohongshu Platform = "xiaohongshu"
	PlatformDouyin      Platform = "douyin"
	PlatformWechatMP    Platform = "wechat_mp"
	PlatformBilibili    Platform = "bilibili"
	PlatformZhihu       Platform = "zhihu"
	PlatformWeibo       Platform = "weibo"
	PlatformKuaishou    Platform = "kuaishou"
	PlatformShipinhao   Platform = "shipinhao"
)

// Platforms is the controlled set. It is the authority: a value outside it is
// answered with 400 and a diagnostic error object, and never reaches storage.
//
// The CHECK constraint in migration 477 repeats these values as a backstop for
// writes that bypass the API. The two must stay identical, which is why a test
// reads the migration and compares rather than restating the list.
//
// Adding a platform therefore needs a migration as well as an entry here. That
// cost buys a grouping key later modules can trust.
var Platforms = []Platform{
	PlatformXiaohongshu, PlatformDouyin, PlatformWechatMP, PlatformBilibili,
	PlatformZhihu, PlatformWeibo, PlatformKuaishou, PlatformShipinhao,
}

var (
	ErrPlatform    = errors.New("unsupported platform")
	ErrDisplayName = errors.New("display name is required")
)

// ValidatePlatform accepts only an exact match. No trimming and no case folding:
// "Xiaohongshu " and "xiaohongshu" would become two grouping keys for one
// platform if either were tolerated here, and the CHECK would reject the first
// anyway - so the API would disagree with storage.
func ValidatePlatform(value string) error {
	for _, platform := range Platforms {
		if string(platform) == value {
			return nil
		}
	}
	return ErrPlatform
}

// ValidateDisplayName requires something visible. Blank is refused rather than
// stored: the display name is how a person tells two accounts apart, and an
// empty one renders as a row with nothing in it.
//
// Duplicates are allowed. One brand running two accounts under the same name on
// different platforms is ordinary; identity is the id, not the name.
func ValidateDisplayName(value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrDisplayName
	}
	return nil
}

// Account is one brand content account.
type Account struct {
	AccountID   string         `json:"account_id"`
	WorkspaceID string         `json:"workspace_id"`
	Platform    string         `json:"platform"`
	DisplayName string         `json:"display_name"`
	Settings    map[string]any `json:"settings"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}
