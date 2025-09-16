package thunderx

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

// 高级设置
type ExpertAddition struct {
	driver.RootID

	LoginType string `json:"login_type" type:"select" options:"user,refresh_token" default:"user"`
	SignType  string `json:"sign_type" type:"select" options:"algorithms,captcha_sign" default:"algorithms"`

	// 登录方式1
	Username string `json:"username" required:"true" help:"login type is user,this is required"`
	Password string `json:"password" required:"true" help:"login type is user,this is required"`
	// 登录方式2
	RefreshToken string `json:"refresh_token" required:"true" help:"login type is refresh_token,this is required"`

	// 签名方法1
	Algorithms string `json:"algorithms" required:"true" help:"sign type is algorithms,this is required" default:"kVy0WbPhiE4v6oxXZ88DvoA3Q,lON/AUoZKj8/nBtcE85mVbkOaVdVa,rLGffQrfBKH0BgwQ33yZofvO3Or,FO6HWqw,GbgvyA2,L1NU9QvIQIH7DTRt,y7llk4Y8WfYflt6,iuDp1WPbV3HRZudZtoXChxH4HNVBX5ZALe,8C28RTXmVcco0,X5Xh,7xe25YUgfGgD0xW3ezFS,,CKCR,8EmDjBo6h3eLaK7U6vU2Qys0NsMx,t2TeZBXKqbdP09Arh9C3"`
	// 签名方法2
	CaptchaSign string `json:"captcha_sign" required:"true" help:"sign type is captcha_sign,this is required"`
	Timestamp   string `json:"timestamp" required:"true" help:"sign type is captcha_sign,this is required"`

	// 验证�?
	CaptchaToken string `json:"captcha_token"`

	// 必要且影响登�?由签名决�?
	DeviceID      string `json:"device_id"  required:"false" default:""`
	ClientID      string `json:"client_id"  required:"true" default:"ZQL_zwA4qhHcoe_2"`
	ClientSecret  string `json:"client_secret"  required:"true" default:"Og9Vr1L8Ee6bh0olFxFDRg"`
	ClientVersion string `json:"client_version"  required:"true" default:"1.06.0.2132"`
	PackageName   string `json:"package_name"  required:"true" default:"com.thunder.downloader"`

	////不影响登�?影响下载速度
	UserAgent         string `json:"user_agent"  required:"false" default:""`
	DownloadUserAgent string `json:"download_user_agent"  required:"false" default:""`

	//优先使用视频链接代替下载链接
	UseVideoUrl bool `json:"use_video_url"`
}

// 登录特征,用于判断是否重新登录
func (i *ExpertAddition) GetIdentity() string {
	hash := md5.New()
	if i.LoginType == "refresh_token" {
		hash.Write([]byte(i.RefreshToken))
	} else {
		hash.Write([]byte(i.Username + i.Password))
	}

	if i.SignType == "captcha_sign" {
		hash.Write([]byte(i.CaptchaSign + i.Timestamp))
	} else {
		hash.Write([]byte(i.Algorithms))
	}

	hash.Write([]byte(i.DeviceID))
	hash.Write([]byte(i.ClientID))
	hash.Write([]byte(i.ClientSecret))
	hash.Write([]byte(i.ClientVersion))
	hash.Write([]byte(i.PackageName))
	return hex.EncodeToString(hash.Sum(nil))
}

type Addition struct {
	driver.RootID
	Username     string `json:"username" required:"true"`
	Password     string `json:"password" required:"true"`
	CaptchaToken string `json:"captcha_token"`
	UseVideoUrl  bool   `json:"use_video_url" default:"true"`
}

// 登录特征,用于判断是否重新登录
func (i *Addition) GetIdentity() string {
	return utils.GetMD5EncodeStr(i.Username + i.Password)
}

var config = driver.Config{
	Name:      "ThunderX",
	LocalSort: true,
}

var configExpert = driver.Config{
	Name:      "ThunderXExpert",
	LocalSort: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &ThunderX{}
	})
	op.RegisterDriver(func() driver.Driver {
		return &ThunderXExpert{}
	})
}
