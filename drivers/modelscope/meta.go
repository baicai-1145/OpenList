package modelscope

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

const (
	DriverName = "modelscope"
)

var config = driver.Config{
	Name:      "ModelScope",
	OnlyProxy: false,
}

type Addition struct {
	driver.RootPath
	ModelID      string `json:"model_id" type:"string" required:"true"`
	ResourceType string `json:"resource_type" type:"select" options:"model,dataset" default:"model"`
	Revision     string `json:"revision" type:"string" required:"true" default:"master"`
	DefaultRoot  string `json:"default_root" type:"string"`
}

func (a *Addition) GetRootPath() string {
	return a.ModelID
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &ModelScope{}
	})
}
