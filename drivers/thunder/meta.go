package thunder

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
	Algorithms string `json:"algorithms" required:"true" help:"sign type is algorithms,this is required" default:"9uJNVj/wLmdwKrJaVj/omlQ,Oz64Lp0GigmChHMf/6TNfxx7O9PyopcczMsnf,Eb+L7Ce+Ej48u,jKY0,ASr0zCl6v8W4aidjPK5KHd1Lq3t+vBFf41dqv5+fnOd,wQlozdg6r1qxh0eRmt3QgNXOvSZO6q/GXK,gmirk+ciAvIgA/cxUUCema47jr/YToixTT+Q6O,5IiCoM9B1/788ntB,P07JH0h6qoM6TSUAK2aL9T5s2QBVeY9JWvalf,+oK0AN"`
	// 签名方法2
	CaptchaSign string `json:"captcha_sign" required:"true" help:"sign type is captcha_sign,this is required"`
	Timestamp   string `json:"timestamp" required:"true" help:"sign type is captcha_sign,this is required"`

	// 验证�?
	CaptchaToken string `json:"captcha_token"`
	// 信任密钥
	CreditKey string `json:"credit_key" help:"credit key,used for login"`

	// 必要且影响登�?由签名决�?
	DeviceID      string `json:"device_id" default:""`
	ClientID      string `json:"client_id"  required:"true" default:"Xp6vsxz_7IYVw2BB"`
	ClientSecret  string `json:"client_secret"  required:"true" default:"Xp6vsy4tN9toTVdMSpomVdXpRmES"`
	ClientVersion string `json:"client_version"  required:"true" default:"8.31.0.9726"`
	PackageName   string `json:"package_name"  required:"true" default:"com.xunlei.downloadprovider"`

	//不影响登�?影响下载速度
	UserAgent         string `json:"user_agent"  required:"true" default:"ANDROID-com.xunlei.downloadprovider/8.31.0.9726 netWorkType/5G appid/40 deviceName/Xiaomi_M2004j7ac deviceModel/M2004J7AC OSVersion/12 protocolVersion/301 platformVersion/10 sdkVersion/512000 Oauth2Client/0.9 (Linux 4_14_186-perf-gddfs8vbb238b) (JAVA 0)"`
	DownloadUserAgent string `json:"download_user_agent"  required:"true" default:"Dalvik/2.1.0 (Linux; U; Android 12; M2004J7AC Build/SP1A.210812.016)"`

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
	// 信任密钥
	CreditKey string `json:"credit_key" help:"credit key,used for login"`
	// 登录设备ID
	DeviceID string `json:"device_id" default:""`
}

// 登录特征,用于判断是否重新登录
func (i *Addition) GetIdentity() string {
	return utils.GetMD5EncodeStr(i.Username + i.Password)
}

var config = driver.Config{
	Name:      "Thunder",
	LocalSort: true,
	OnlyProxy: true,
}

var configExpert = driver.Config{
	Name:      "ThunderExpert",
	LocalSort: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Thunder{}
	})
	op.RegisterDriver(func() driver.Driver {
		return &ThunderExpert{}
	})
}
