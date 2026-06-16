package modelscope

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

const (
	DriverName = "modelscope"
)

var config = driver.Config{
	Name:      "ModelScope",
	OnlyProxy: false,
	LocalSort: false,
	NoUpload:  true,
	NoCache:   false,
}

type Addition struct {
	driver.RootPath
	ModelID      string `json:"model_id" type:"string" required:"true" help:"ModelScope 仓库路径，如 baicai1145/BanG_Dream_Dataset"`
	ResourceType string `json:"resource_type" type:"select" options:"model,dataset" default:"model" help:"资源类型：model=模型仓库，dataset=数据集仓库"`
	Revision     string `json:"revision" type:"string" required:"true" default:"master" help:"分支/版本名，默认 master"`
	DefaultRoot  string `json:"default_root" type:"string" help:"默认进入的子目录，留空则从仓库根目录开始"`
}

func (a *Addition) GetRootPath() string {
	return a.ModelID
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &ModelScope{}
	})
}
