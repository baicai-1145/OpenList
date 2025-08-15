package modelscope

import (
	"time"
	stdpath "path"
	"github.com/OpenListTeam/OpenList/internal/model"
)

// do others that not defined in Driver interface

func fileToObj(file File) model.Obj {
	isDir := file.Type == "tree"
	return &model.Object{
		ID:       file.Path,
		Name:     file.Name,
		Size:     file.Size,
		Modified: time.Time{}, // API doesn't provide this
		IsFolder: isDir,
		Path:     file.Path,
	}
}

func filesToObjs(files []File) []model.Obj {
	objs := make([]model.Obj, 0, len(files))
	for _, file := range files {
		obj := &model.Object{
			Name:     file.Name,
			Path:     file.Path,
			Size:     file.Size,
			Modified: time.Unix(file.CommittedDate, 0),
			IsFolder: file.Type == "tree",
		}
		objs = append(objs, obj)
	}
	return objs
}

// Helper to get relative path
func getRelativePath(obj model.Obj) string {
	if obj == nil || obj.GetPath() == "/" {
		return ""
	}
	return stdpath.Clean(obj.GetPath())
}
