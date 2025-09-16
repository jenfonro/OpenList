package _123

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	Username string `json:"username" required:"true"`
	Password string `json:"password" required:"true"`
	driver.RootID
	//OrderBy        string `json:"order_by" type:"select" options:"file_id,file_name,size,update_at" default:"file_name"`
	//OrderDirection string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	AccessToken  string
	UploadThread int `json:"UploadThread" type:"number" default:"3" help:"the threads of upload"`
}

var config = driver.Config{
	Name:        "123Pan",
	DefaultRoot: "0",
	LocalSort:   true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		// 新增默认选项 要在RegisterDriver初始化设�?才会对正在使用的用户生效
		return &Pan123{
			Addition: Addition{
				UploadThread: 3,
			},
		}
	})
}
